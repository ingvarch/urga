package ui

import (
	"context"
	"fmt"
	"image/color"
	"time"

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

	// checks are the last results of the checks of the allocation.
	checks []nomad.Check

	// due says a read of the checks, or its timer, is pending.
	// The allocation is read again on every change the cluster reports, and
	// none of those may start a second chain of readings next to the first.
	due bool
}

// listedTasks are the tasks of an allocation a list holds: they show before
// the allocation is read again.
func listedTasks(alloc nomad.Alloc) tasksPage {
	return tasksPage{namespace: alloc.Namespace, jobID: alloc.JobID, allocID: alloc.ID, alloc: alloc}
}

func (p tasksPage) title(_ env, count int) string {
	return sprintf("Tasks (Allocation: %s) [%d]", shortID(p.allocID), count)
}

func (tasksPage) titles() []string { return taskTitles }

// topics: a change to an allocation is a change to its tasks.
func (tasksPage) topics() []string   { return []string{nomad.TopicAllocation} }
func (p tasksPage) where(env) string { return p.namespace }

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

// restart forgets the read of the checks the page is waiting for: it was
// for a request that is over.
func (p tasksPage) restart() page {
	p.due = false

	return p
}

func (p tasksPage) rows(e env) []tableRow { return taskRows(p.alloc.Tasks, e.usage) }

func (p tasksPage) ids(env) []string { return names(p.alloc.Tasks, taskMark) }

// readings are the tasks on the screen that run: a task that ended or has
// not started has nothing to report.
func (p tasksPage) readings(e env) []rowRef {
	refs := make([]rowRef, 0, len(e.index))
	for _, at := range e.index {
		if at < len(p.alloc.Tasks) && p.alloc.Tasks[at].State == statusRunning {
			refs = append(refs, rowRef{namespace: p.namespace, id: p.alloc.Tasks[at].Name})
		}
	}

	return refs
}

// reading reads what one task takes, in the allocation the page is open on.
func (p tasksPage) reading(ctx context.Context, client Client, ref rowRef) (nomad.ResourceUse, error) {
	return client.TaskUsage(ctx, ref.namespace, p.allocID, ref.id)
}

// picked is the task under the cursor.
func (p tasksPage) picked(e env) (nomad.Task, bool) { return pickedFrom(e, p.alloc.Tasks) }

func (p tasksPage) pickedTask(e env) (allocTask, bool) {
	task, ok := p.picked(e)
	if !ok {
		return allocTask{}, false
	}

	return allocTask{taskRef: taskRef{namespace: p.namespace, allocID: p.allocID, name: task.Name}, task: task, alloc: p.alloc}, true
}

func (p tasksPage) pickedAlloc(env) (namespace, allocID string, ok bool) {
	return p.namespace, p.allocID, true
}

// tasksKeys are the keys of a task, and the keys of the allocation after them.
var tasksKeys = append(taskKeys[tasksPage](), tasksAllocKeys...)

var tasksAllocKeys = []pageKey[tasksPage]{
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
var taskTitles = []string{"Name", "State", "Failed", "Restarts", "CPU", "MEM", "Started", "Finished", "Uptime"}

func taskRows(tasks []nomad.Task, usage map[string]nomad.ResourceUse) []tableRow {
	rows := make([]tableRow, 0, len(tasks))

	for _, task := range tasks {
		use, known := usage[task.Name]

		row := tableRow{color: taskColor(task)}
		row.add(
			task.Name,
			task.State,
			fmt.Sprintf("%t", task.Failed),
			fmt.Sprintf("%d", task.Restarts),
			cpuCell(use, known),
			memoryCell(use, known),
		)
		addTaskTimes(&row, task)

		rows = append(rows, row)
	}

	return rows
}

// addTaskTimes puts when a task started and ended, and how long it ran, at
// the end of a row.
func addTaskTimes(row *tableRow, task nomad.Task) {
	ended := finishedAt(task)

	row.addAge(task.Started)
	row.addAge(ended)

	// Only a running task counts to now; one that ended counts to its end.
	switch {
	case task.State == statusRunning:
		row.addSpan(task.Started, time.Time{})
	case !ended.IsZero():
		row.addSpan(task.Started, ended)
	default:
		row.addSpan(time.Time{}, time.Time{})
	}
}

// finishedAt is when a task ended in its last run, or zero while it runs or
// waits. Nomad keeps the end of an earlier run after a restart, so an end
// before the start is not this run's.
func finishedAt(task nomad.Task) time.Time {
	if task.State != statusDead || task.Finished.Before(task.Started) {
		return time.Time{}
	}

	return task.Finished
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
func openTaskEvents[P taskLister](p P, e env) (P, outcome) {
	task, ok := p.pickedTask(e)
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg{taskEventsPage{namespace: task.namespace, allocID: task.allocID, task: task.name, alloc: task.alloc}})
}

// openTaskAllocEvents opens the events of the allocation under the cursor,
// in the namespace of that allocation.
func openTaskAllocEvents[P taskLister](p P, e env) (P, outcome) {
	namespace, allocID, ok := p.pickedAlloc(e)
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg{subjectEvents(namespace, nomad.Subject{AllocID: allocID})})
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
func (taskEventsPage) topics() []string   { return []string{nomad.TopicAllocation} }
func (p taskEventsPage) where(env) string { return p.namespace }

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
		var row tableRow
		row.addAge(event.Time)
		row.add(event.Type, event.Message)

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
