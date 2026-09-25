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

// taskGroupsPage is the groups of one job.
type taskGroupsPage struct {
	namespace, jobID string

	groups []nomad.TaskGroup
}

func (p taskGroupsPage) title(_ env, count int) string {
	return sprintf("Task Groups (Job: %s) [%d]", p.jobID, count)
}

func (taskGroupsPage) titles() []string { return taskGroupTitles }
func (taskGroupsPage) topics() []string { return nil }

// fetch reads the groups of the job the page is open on.
func (p taskGroupsPage) fetch(e env) tea.Cmd {
	client, namespace, jobID := e.client, p.namespace, p.jobID

	return fetchList(func(ctx context.Context) ([]nomad.TaskGroup, error) {
		return client.TaskGroups(ctx, namespace, jobID)
	}, func(items []nomad.TaskGroup) tea.Msg { return taskGroupsMsg(items) })
}

func (p taskGroupsPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	groups, ok := msg.(taskGroupsMsg)
	if !ok {
		return p, outcome{}, false
	}

	p.groups = groups

	return p, outcome{}, true
}

func (p taskGroupsPage) rows(env) []tableRow { return taskGroupRows(p.groups) }

// picked is the group under the cursor.
func (p taskGroupsPage) picked(e env) (nomad.TaskGroup, bool) { return pickedFrom(e, p.groups) }

var taskGroupKeys = []pageKey[taskGroupsPage]{
	{press: "enter", label: "Allocations", do: openGroupAllocations},
	{press: "s", label: "Scale", do: scaleGroup, writes: true},
	{press: "p", label: "Placement", do: groupPlacement, offered: groupWaits},
	{press: "l", label: "Logs", do: groupLogs},
}

func (p taskGroupsPage) keys(e env) []keyHint { return hintsOf(p, e, taskGroupKeys) }

func (p taskGroupsPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, taskGroupKeys, k)
}

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

// openJobGroups lists the groups of the job under the cursor. The screen
// keeps the namespace of the job, where a scale is sent.
func openJobGroups(p jobsPage, e env) (jobsPage, outcome) {
	job, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg(screen{
		kind:      screenTaskGroups,
		namespace: job.Namespace,
		page:      taskGroupsPage{namespace: job.Namespace, jobID: job.ID},
	}))
}

// scaleGroup asks how many allocations the group under the cursor should run.
func scaleGroup(p taskGroupsPage, e env) (taskGroupsPage, outcome) {
	group, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	return p, then(scaleMsg(group))
}

// askScale puts up the line that asks for the count of a group, with the
// count it runs now.
func (m Model) askScale(group nomad.TaskGroup) (Model, tea.Cmd) {
	m.overlay = overlayScale
	m.prompt = promptModel{
		prefix: fmt.Sprintf("scale %s to: ", group.Name),
		text:   strconv.Itoa(group.Count),
		group:  group,
	}
	m.layout()

	return m, nil
}

// scaleTo sets the count of a group, when the answer is a count and the
// question that follows is answered yes.
func (m Model) scaleTo(group nomad.TaskGroup, input string) (Model, tea.Cmd) {
	count, err := strconv.Atoi(input)
	if err != nil || count < 0 {
		return m.fail(fmt.Errorf("%q is not a count", input)), nil
	}

	client, namespace := m.client, m.screen.namespace

	return m.ask(
		fmt.Sprintf("Really scale %s of %s from %d to %d?", group.Name, group.JobID, group.Count, count),
		act(fmt.Sprintf("Task group %s scaled to %d.", group.Name, count), func(ctx context.Context) error {
			return client.ScaleJob(ctx, namespace, group.JobID, group.Name, count)
		}),
	)
}

// openGroupAllocations drills into the task group under the cursor: the
// allocations of the job that belong to it.
func openGroupAllocations(p taskGroupsPage, e env) (taskGroupsPage, outcome) {
	group, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg(screen{
		kind:      screenAllocations,
		namespace: p.namespace,
		page:      allocationsPage{namespace: p.namespace, jobID: group.JobID, group: group.Name},
	}))
}
