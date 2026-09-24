package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// shownAlloc is the allocation the tasks screen read, once it has.
func (m Model) shownAlloc() (nomad.Alloc, bool) {
	return m.alloc, m.alloc.ID != "" && m.alloc.ID == m.screen.allocID
}

// tasksPanel is the panel of the allocation the tasks screen read, as much
// of it as leaves the tasks their rows. Until it has read one, there is none.
func (m Model) tasksPanel(width int) []string {
	alloc, ok := m.shownAlloc()
	if !ok {
		return nil
	}

	lines := allocPanel(alloc, width)

	// The line of air before the tasks stays with what is kept.
	if room := m.rowsForPanel(); len(lines) > room {
		if room < 2 {
			return nil
		}

		lines = append(lines[:room-1:room-1], "")
	}

	return lines
}

// allocHas offers a key when the allocation on the screen has somewhere for
// it to go.
func allocHas(where func(nomad.Alloc) string) func(m Model) bool {
	return func(m Model) bool {
		alloc, ok := m.shownAlloc()

		return ok && where(alloc) != ""
	}
}

// openAllocNode opens the client the allocation runs on.
func openAllocNode(m Model) (Model, tea.Cmd) {
	alloc := m.alloc

	return m.openNode(nomad.Node{ID: alloc.NodeID, Name: alloc.NodeName})
}

// openReplaced and openReplacement open the tasks of the allocation this one
// replaced, and of the one that replaced it. Escape comes back here.
func openReplaced(m Model) (Model, tea.Cmd) { return m.openAllocTasks(m.alloc.Previous) }

func openReplacement(m Model) (Model, tea.Cmd) { return m.openAllocTasks(m.alloc.Next) }

func (m Model) openAllocTasks(allocID string) (Model, tea.Cmd) {
	alloc := m.alloc

	return m.push(screen{kind: screenTasks, namespace: alloc.Namespace, jobID: alloc.JobID, allocID: allocID})
}

// openFollowUp reads the evaluation that will place the allocation again.
func openFollowUp(m Model) (Model, tea.Cmd) {
	return m, describeEvaluation(m.client, m.alloc.Namespace, m.alloc.FollowUp)
}

// field is a label and its value on a line of a panel.
type field struct{ label, value string }

// allocPanel is what the tasks of an allocation show above them: what it is
// to its job and its deployment, where it listens, and what came before and
// after it. A line with nothing to say is left out.
func allocPanel(alloc nomad.Alloc, width int) []string {
	deployment := alloc.Health
	if deployment != "" && alloc.Canary {
		deployment += ", canary"
	}

	node := alloc.NodeName
	if node == "" {
		node = shortID(alloc.NodeID)
	}

	ports := make([]string, 0, len(alloc.Ports))
	for _, port := range alloc.Ports {
		mapped := port.Label + " " + port.Address
		if port.To > 0 {
			mapped += fmt.Sprintf("->%d", port.To)
		}

		ports = append(ports, mapped)
	}

	reschedules := ""
	if alloc.Reschedules > 0 {
		reschedules = fmt.Sprint(alloc.Reschedules)
	}

	lines := [][]field{
		{
			{"Status", alloc.Status},
			{"Desired", alloc.DesiredStatus},
			{"Client", node},
			{"Version", fmt.Sprint(alloc.JobVersion)},
			{"Deployment", deployment},
		},
		{{"Ports", strings.Join(ports, ", ")}},
		{
			{"Reschedules", reschedules},
			{"Previous", shortID(alloc.Previous)},
			{"Next", shortID(alloc.Next)},
			{"Follow-up", shortID(alloc.FollowUp)},
		},
	}

	rows := []string{}

	for _, line := range lines {
		if row := fieldLine(line, width-1); row != "" {
			// The panel starts where the columns of the table do.
			rows = append(rows, " "+row)
		}
	}

	return append(rows, "")
}

// fieldLine lays out the fields that have a value, or nothing when none has.
func fieldLine(fields []field, width int) string {
	cells := make([]string, 0, len(fields))

	for _, f := range fields {
		if f.value == "" {
			continue
		}

		cells = append(cells, styleLabel.Render(f.label)+" "+styleValue.Render(f.value))
	}

	return truncate(strings.Join(cells, strings.Repeat(" ", columnGap+1)), width)
}
