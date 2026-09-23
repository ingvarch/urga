package ui

import (
	"context"
	"fmt"
	"image/color"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// taskGroupTitles are the columns of the task group list.
var taskGroupTitles = []string{"Name", "JobID", "Allocs", "Starting", "Queued", "Complete", "Failed", "Lost"}

func taskGroupRows(groups []nomad.TaskGroup) []tableRow {
	rows := make([]tableRow, 0, len(groups))

	for _, group := range groups {
		rows = append(rows, tableRow{
			cells: []string{
				group.Name,
				group.JobID,
				fmt.Sprintf("%d/%d", group.Running+group.Starting, group.Count),
				strconv.Itoa(group.Starting),
				strconv.Itoa(group.Queued),
				strconv.Itoa(group.Complete),
				strconv.Itoa(group.Failed),
				strconv.Itoa(group.Lost),
			},
			color: taskGroupColor(group),
		})
	}

	return rows
}

// taskGroupColor marks a group that is not running what it asks for.
func taskGroupColor(group nomad.TaskGroup) color.Color {
	switch {
	case group.Failed > 0, group.Lost > 0:
		return colorDead
	case group.Running+group.Starting < group.Count:
		return colorAttention
	}

	return nil
}

var taskGroupHints = []hint{
	{Key: "<enter>", Description: "Allocations"},
	{Key: "<s>", Description: "Scale"},
}

// openTaskGroups lists the groups of the job under the cursor.
func (m Model) openTaskGroups() (Model, tea.Cmd) {
	job, ok := selectedOf(m, screenJobs, m.jobs)
	if !ok {
		return m, nil
	}

	return m.push(screen{kind: screenTaskGroups, namespace: job.Namespace, jobID: job.ID})
}

// scaleGroup asks how many allocations the group under the cursor should run.
func (m Model) scaleGroup() (Model, tea.Cmd) {
	group, ok := selectedOf(m, screenTaskGroups, m.groups)
	if !ok {
		return m, nil
	}

	m.overlay = overlayScale
	m.prompt = promptModel{
		prefix: fmt.Sprintf("scale %s to: ", group.Name),
		text:   strconv.Itoa(group.Count),
		group:  group.Name,
	}
	m.layout()

	return m, nil
}

// scaleTo sets the count of a group, when the answer is a count.
func (m Model) scaleTo(group, input string) (Model, tea.Cmd) {
	count, err := strconv.Atoi(input)
	if err != nil || count < 0 {
		return m.fail(fmt.Errorf("%q is not a count", input)), nil
	}

	client, screen := m.client, m.screen

	return m, act(fmt.Sprintf("Task group %s scaled to %d.", group, count), func(ctx context.Context) error {
		return client.ScaleJob(ctx, screen.namespace, screen.jobID, group, count)
	})
}
