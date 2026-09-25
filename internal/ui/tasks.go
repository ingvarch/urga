package ui

import (
	"context"
	"fmt"
	"image/color"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// tasksPage is the tasks of one allocation, under what the allocation is to
// its job: where it runs and listens, what came before and after it, and
// what its checks say.
type tasksPage struct {
	namespace, jobID, allocID string

	// alloc is the allocation as the page last read it, or as the list it
	// was opened from held it until then; read says the page has read it.
	// The tasks do not wait for a reading, the panel above them does.
	alloc nomad.Alloc
	read  bool

	// checks are what the checks of the allocation last said.
	checks []nomad.Check

	// due says the next reading of the checks or its timer is on its way.
	// The allocation is read again on every change the cluster reports, and
	// none of those may start a second chain of readings next to the first.
	due bool
}

// listedTasks are the tasks of an allocation a list holds: they show before
// the allocation is read again.
func listedTasks(alloc nomad.Alloc) tasksPage {
	return tasksPage{namespace: alloc.Namespace, jobID: alloc.JobID, allocID: alloc.ID, alloc: alloc}
}

// tasksScreen opens the tasks of an allocation where it lives, which is
// where its stream watches.
func tasksScreen(p tasksPage) screen {
	return screen{kind: screenTasks, namespace: p.namespace, page: p}
}

func (p tasksPage) title(_ env, count int) string {
	return sprintf("Tasks (Allocation: %s) [%d]", shortID(p.allocID), count)
}

func (tasksPage) titles() []string { return taskTitles }

// topics: a change to an allocation is a change to its tasks.
func (tasksPage) topics() []string { return []string{nomad.TopicAllocation} }

func (p tasksPage) fetch(e env) tea.Cmd {
	return fetchAllocation(e.client, p.namespace, p.allocID)
}

func (p tasksPage) take(msg tea.Msg, e env) (page, outcome, bool) {
	switch msg := msg.(type) {
	case allocMsg:
		if msg.ID != p.allocID {
			return p, outcome{}, false
		}

		p.alloc, p.read = nomad.Alloc(msg), true

		return p.checksOnce(e)

	case checksMsg:
		if msg.allocID != p.allocID {
			return p, outcome{}, false
		}

		return p.keepChecks(msg)

	case pollChecksMsg:
		return p.pollChecks(e)
	}

	return p, outcome{}, false
}

// restart lets go of the reading of the checks the page thinks is on its
// way: it belonged to an ask that is over.
func (p tasksPage) restart() page {
	p.due = false

	return p
}

func (p tasksPage) rows(env) []tableRow { return taskRows(p.alloc.Tasks) }

// picked is the task under the cursor.
func (p tasksPage) picked(e env) (nomad.Task, bool) { return pickedFrom(e, p.alloc.Tasks) }

var tasksKeys = []pageKey[tasksPage]{
	{press: "enter", label: "Logs", do: followStdout},
	// The same key opens the events of a task here and edits elsewhere,
	// because a screen never offers both.
	{press: "e", label: "Events", do: openTaskEvents},
	{press: "ctrl+e", label: "Stderr", do: followStderr},
	// The same key opens a shell here and scales a task group elsewhere.
	{press: "s", label: "Shell", do: shell, writes: true},
	{press: "r", label: "Restart", do: restartTask, writes: true, offered: taskRuns},
	{press: "x", label: "Signal", do: askSignal, writes: true, offered: taskRuns},
	{press: "b", label: "Browse", do: browse},
	{press: "c", label: "Client", do: openAllocNode, offered: allocHas(func(a nomad.Alloc) string { return a.NodeID })},
	{press: "p", label: "Previous", do: openReplaced, offered: allocHas(func(a nomad.Alloc) string { return a.Previous })},
	{press: "n", label: "Next", do: openReplacement, offered: allocHas(func(a nomad.Alloc) string { return a.Next })},
	{press: "f", label: "Follow-up", do: openFollowUp, offered: allocHas(func(a nomad.Alloc) string { return a.FollowUp })},
}

func (p tasksPage) keys(e env) []keyHint { return hintsOf(p, e, tasksKeys) }

func (p tasksPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, tasksKeys, k)
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

// openTaskEvents opens what happened to the task under the cursor.
func openTaskEvents(p tasksPage, e env) (tasksPage, outcome) {
	task, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg(screen{
		kind:      screenTaskEvents,
		namespace: p.namespace,
		page:      taskEventsPage{namespace: p.namespace, allocID: p.allocID, task: task.Name, alloc: p.alloc},
	}))
}

// taskEventsPage is what happened to one task of an allocation.
type taskEventsPage struct {
	noKeys

	namespace, allocID, task string

	// alloc is the allocation as it was last read, or as the tasks it was
	// opened from held it until then.
	alloc nomad.Alloc
}

func (p taskEventsPage) title(_ env, count int) string {
	return sprintf("Events (Task: %s) [%d]", p.task, count)
}

func (taskEventsPage) titles() []string { return taskEventTitles }

// topics: a change to an allocation is a change to what its tasks went
// through.
func (taskEventsPage) topics() []string { return []string{nomad.TopicAllocation} }

func (p taskEventsPage) fetch(e env) tea.Cmd {
	return fetchAllocation(e.client, p.namespace, p.allocID)
}

func (p taskEventsPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	alloc, ok := msg.(allocMsg)
	if !ok || alloc.ID != p.allocID {
		return p, outcome{}, false
	}

	p.alloc = nomad.Alloc(alloc)

	return p, outcome{}, true
}

func (p taskEventsPage) rows(env) []tableRow { return taskEventRows(eventsOf(p.alloc, p.task)) }

// eventsOf are what happened to one task of an allocation.
func eventsOf(alloc nomad.Alloc, task string) []nomad.TaskEvent {
	for _, t := range alloc.Tasks {
		if t.Name == task {
			return t.Events
		}
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

// fetchAllocation reads an allocation on its own. The list it was opened
// from is not asked again while it is not on the screen, and it need not be
// in that list at all: the one it replaced may be on another client.
func fetchAllocation(client allocsClient, namespace, allocID string) tea.Cmd {
	return request(func(ctx context.Context) (nomad.Alloc, error) {
		return client.Allocation(ctx, namespace, allocID)
	}, func(alloc nomad.Alloc) tea.Msg { return allocMsg(alloc) })
}
