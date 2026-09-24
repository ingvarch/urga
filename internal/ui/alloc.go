package ui

import (
	"fmt"
	"image/color"

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
		row := tableRow{cells: []string{ageOf(event.Time), event.Type, event.Message}}
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
func (m Model) openTaskEvents() (Model, tea.Cmd) {
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

// openAllocation drills into the allocation under the cursor: the tasks it
// runs.
func openAllocation(m Model) (Model, tea.Cmd) {
	alloc, ok := selectedOf(m, screenAllocations, m.visibleAllocs())
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
