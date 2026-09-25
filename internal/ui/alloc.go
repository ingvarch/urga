package ui

import (
	"context"
	"fmt"
	"image/color"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// allocTitles are the columns of the allocation list.
var allocTitles = []string{"ID", "TaskGroup", "JobID", "Namespace", "Node", "Status", "Desired", "CPU", "MEM", "Age"}

func allocRows(allocs []nomad.Alloc, usage map[string]nomad.ResourceUse) []tableRow {
	rows := make([]tableRow, 0, len(allocs))

	for _, alloc := range allocs {
		use, known := usage[alloc.ID]

		rows = append(rows, tableRow{
			cells: []string{
				shortID(alloc.ID),
				alloc.TaskGroup,
				alloc.JobID,
				alloc.Namespace,
				alloc.NodeName,
				alloc.Status,
				alloc.DesiredStatus,
				cpuCell(use, known),
				memoryCell(use, known),
				ageOf(alloc.Created),
			},
			ages:  moments{9: alloc.Created},
			color: allocColor(alloc),
		})
	}

	return rows
}

// allocColor says whether an allocation is doing its job.
func allocColor(alloc nomad.Alloc) color.Color {
	switch alloc.Status {
	case statusRunning:
		if alloc.DesiredStatus == desiredStop {
			return colorAttention
		}
	case statusPending:
		return colorPending
	case statusFailed, statusLost:
		return colorDead
	case statusComplete:
		return colorSpent
	}

	return nil
}

// taskTitles are the columns of the task list.

var taskTitles = []string{"Name", "State", "Failed", "Restarts", "Started"}

func taskRows(tasks []nomad.Task) []tableRow {
	rows := make([]tableRow, 0, len(tasks))

	for _, task := range tasks {
		rows = append(rows, tableRow{
			cells: []string{
				task.Name,
				task.State,
				fmt.Sprintf("%t", task.Failed),
				fmt.Sprintf("%d", task.Restarts),
				ageOf(task.Started),
			},
			ages:  moments{4: task.Started},
			color: taskColor(task),
		})
	}

	return rows
}

func taskColor(task nomad.Task) color.Color {
	switch {
	case task.Failed:
		return colorDead
	case task.State == statusPending:
		return colorPending
	case task.State == statusDead:
		return colorSpent
	}

	return nil
}

// taskEventTitles are the columns of what happened to a task.
var taskEventTitles = []string{"Age", "Type", "Message"}

func taskEventRows(events []nomad.TaskEvent) []tableRow {
	rows := make([]tableRow, 0, len(events))

	for _, event := range events {
		row := tableRow{cells: []string{ageOf(event.Time), event.Type, event.Message}, ages: moments{0: event.Time}}
		if event.Failed {
			row.color = colorDead
		}

		rows = append(rows, row)
	}

	return rows
}

// taskEvents are what happened to the task the screen was opened for.
func (m Model) taskEvents() []nomad.TaskEvent {
	for _, task := range m.tasks() {
		if task.Name == m.screen.task {
			return task.Events
		}
	}

	return nil
}

// openTaskEvents opens what happened to the task under the cursor.
func openTaskEvents(m Model) (Model, tea.Cmd) {
	task, ok := selectedOf(m, screenTasks, m.tasks())
	if !ok {
		return m, nil
	}

	next := m.screen
	next.kind = screenTaskEvents
	next.task = task.Name

	return m.push(next)
}

const (
	statusComplete = "complete"
	statusLost     = "lost"

	desiredStop = "stop"
)

// fetchAllocation reads again the allocation a screen of its tasks was opened
// for. The list it was opened from is not asked again while it is not on the
// screen.
func fetchAllocation(m Model) tea.Cmd {
	client, screen := m.client, m.screen

	return request(func(ctx context.Context) (nomad.Alloc, error) {
		return client.Allocation(ctx, screen.namespace, screen.allocID)
	}, func(alloc nomad.Alloc) tea.Msg { return allocMsg(alloc) })
}

// keepAllocation puts the allocation that was read again in place of the one
// the list held, which is where its tasks are read from.
func (m Model) keepAllocation(alloc nomad.Alloc) (Model, tea.Cmd) {
	kind := m.screen.kind
	ours := (kind == screenTasks || kind == screenTaskEvents) && m.screen.allocID == alloc.ID

	m, read := m.applyWhen(ours, func(m *Model) {
		m.alloc = alloc

		at := slices.IndexFunc(m.allocs, func(held nomad.Alloc) bool { return held.ID == alloc.ID })
		if at < 0 {
			return
		}

		// A copy: the list is shared with the model this one was made from.
		allocs := slices.Clone(m.allocs)
		allocs[at] = alloc
		m.allocs = allocs
	})

	return checksOnce(m, read)
}

// openAllocation drills into the allocation under the cursor: the tasks it
// runs.
func openAllocation(m Model) (Model, tea.Cmd) {
	alloc, ok := m.selectedAlloc()
	if !ok {
		return m, nil
	}

	return m.push(screen{
		kind:      screenTasks,
		namespace: alloc.Namespace,
		jobID:     alloc.JobID,
		allocID:   alloc.ID,
	})
}

// selectedAlloc is the allocation under the cursor of a list of them, of
// whatever the list was opened for.
func (m Model) selectedAlloc() (nomad.Alloc, bool) {
	if !m.screen.listsAllocs() {
		return nomad.Alloc{}, false
	}

	return selectedOf(m, m.screen.kind, m.visibleAllocs())
}

// fetchAllocs asks for the allocations of the list on the screen.
func fetchAllocs(m Model) tea.Cmd {
	return fetchList(allocsOf(m.client, m.screen), func(items []nomad.Alloc) tea.Msg { return allocsMsg(items) })
}

// allocListRows are the rows of a list of allocations that says nothing of
// its own about them.
func allocListRows(m Model) []tableRow { return allocRows(m.visibleAllocs(), m.usage.rows) }

// allocReadings are the allocations of the list whose usage the rows show.
func allocReadings(m Model) []rowRef {
	allocs := m.visibleAllocs()

	refs := make([]rowRef, 0, len(m.list.index))
	for _, at := range m.list.index {
		// Only what runs has anything to report.
		if at < len(allocs) && allocs[at].Status == statusRunning {
			refs = append(refs, rowRef{namespace: allocs[at].Namespace, id: allocs[at].ID})
		}
	}

	return refs
}

// allocReading reads what one allocation takes.
func allocReading(ctx context.Context, client Client, ref rowRef) (nomad.ResourceUse, error) {
	return client.AllocationUsage(ctx, ref.namespace, ref.id)
}
