package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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

	// Only what runs has checks worth reading.
	var checks []nomad.Check
	if m.checks.allocID == alloc.ID && alloc.Status == statusRunning {
		checks = m.checks.list
	}

	return allocPanel(alloc, checks, width, m.rowsForPanel())
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
// to its job and its deployment, where it listens, what came before and
// after it, and its checks. A line with nothing to say is left out, and the
// panel takes no more than room rows, the line of air before the tasks
// included.
func allocPanel(alloc nomad.Alloc, checks []nomad.Check, width, room int) []string {
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

	return fitPanel(rows, checkBlock(checks, width), room)
}

// panelBlock is a part of a panel that lists things under a title: the
// checks of an allocation, the groups of a deployment. An entry is one
// thing, on one row or more.
type panelBlock struct {
	title   []string
	entries [][]string

	// more is the row that counts the entries that do not fit.
	more func(count int) string
}

// fitPanel lays the head of a panel out with a block under it, in no more
// than room rows, the line of air before the table included. What does not
// fit of the head is cut. The block keeps whole entries and counts the rest,
// and with room for none of them it is left out.
func fitPanel(head []string, block panelBlock, room int) []string {
	if len(head) >= room {
		if room < 2 {
			return nil
		}

		return append(head[:room-1:room-1], "")
	}

	rows := append(head, blockRows(block, room-len(head)-1)...)

	return append(rows, "")
}

// blockRows are the rows of a block in no more than room rows: a line apart
// from what is above it, its title, and the entries that fit.
func blockRows(block panelBlock, room int) []string {
	if len(block.entries) == 0 {
		return nil
	}

	rows := append([]string{""}, block.title...)

	used := 0
	for _, entry := range block.entries {
		used += len(entry)
	}

	if len(rows)+used <= room {
		for _, entry := range block.entries {
			rows = append(rows, entry...)
		}

		return rows
	}

	// One row goes to the count of the rest.
	left, shown := room-len(rows)-1, 0

	for _, entry := range block.entries {
		if len(entry) > left {
			break
		}

		rows = append(rows, entry...)
		left -= len(entry)
		shown++
	}

	if shown == 0 {
		return nil
	}

	return append(rows, styleMuted.Render(block.more(len(block.entries)-shown)))
}

// checkBlock is the checks of an allocation. One that fails says why under
// it.
func checkBlock(checks []nomad.Check, width int) panelBlock {

	named := 0
	for _, check := range checks {
		named = max(named, ansi.StringWidth(check.Service+"/"+check.Name))
	}

	entries := make([][]string, 0, len(checks))

	for _, check := range checks {
		style := checkStyle(check.Status)

		line := "  " + style.Render("●") + " " + styleText.Render(pad(check.Service+"/"+check.Name, named)) +
			"  " + style.Render(check.Status)
		if !check.Time.IsZero() {
			line += "  " + styleMuted.Render(ageOf(check.Time)+" ago")
		}

		entry := []string{truncate(line, width)}

		if check.Status == checkFailure && check.Output != "" {
			why := strings.Join(strings.Fields(check.Output), " ")
			entry = append(entry, truncate("    "+styleText.Render(why), width))
		}

		entries = append(entries, entry)
	}

	return panelBlock{
		title:   []string{" " + styleLabel.Render("Checks")},
		entries: entries,
		more:    func(count int) string { return fmt.Sprintf("  + %d more checks", count) },
	}
}

// checkFailure is the status of a check that did not pass.
const checkFailure = "failure"

// checkStyle is the colour of a check in the state it is in.
func checkStyle(status string) lipgloss.Style {
	switch status {
	case "success":
		return styleValue
	case checkFailure:
		return styleError
	}

	return stylePending
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
