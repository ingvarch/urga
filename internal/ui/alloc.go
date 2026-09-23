package ui

import (
	"fmt"
	"image/color"

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

const (
	statusComplete = "complete"
	statusLost     = "lost"

	desiredStop = "stop"
)
