package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// namespaceTasksPage is every task of every allocation of the namespace the
// session looks at, read from the allocation list.
type namespaceTasksPage struct {
	ofTheSession

	allocs []nomad.Alloc
}

// namespaceTaskTitles are the columns of the tasks of the namespace. There
// is no CPU and memory: they cost a request for each task.
var namespaceTaskTitles = []string{
	"Job", "Namespace", "Group", "Task", "Alloc", "Node", "State", "Restarts", "Started", "Finished", "Uptime",
}

func (namespaceTasksPage) title(e env, count int) string {
	return sprintf("Tasks (%s) [%d]", namespaceLabel(e.namespace), count)
}

func (namespaceTasksPage) titles() []string { return namespaceTaskTitles }
func (namespaceTasksPage) topics() []string { return []string{nomad.TopicAllocation} }

func (namespaceTasksPage) fetch(e env) tea.Cmd {
	client, namespace := e.client, e.namespace

	return fetchList(func(ctx context.Context) ([]nomad.Alloc, error) {
		return client.Allocations(ctx, namespace, "")
	}, func(items []nomad.Alloc) tea.Msg { return allocsMsg(items) })
}

func (p namespaceTasksPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	allocs, ok := msg.(allocsMsg)
	if !ok {
		return p, outcome{}, false
	}

	p.allocs = allocs

	return p, outcome{}, true
}

// tasks are the tasks of every allocation, the allocations in the order the
// cluster lists them and the tasks of each by name. Each task keeps the
// namespace of its allocation: the session may look at every namespace.
func (p namespaceTasksPage) tasks() []allocTask {
	var tasks []allocTask

	for _, alloc := range p.allocs {
		for _, task := range alloc.Tasks {
			tasks = append(tasks, allocTask{
				taskRef: taskRef{namespace: alloc.Namespace, allocID: alloc.ID, name: task.Name},
				task:    task,
				alloc:   alloc,
			})
		}
	}

	return tasks
}

func (p namespaceTasksPage) rows(env) []tableRow { return namespaceTaskRows(p.tasks()) }

func (p namespaceTasksPage) ids(env) []string { return names(p.tasks(), allocTaskMark) }

func namespaceTaskRows(tasks []allocTask) []tableRow {
	rows := make([]tableRow, 0, len(tasks))

	for _, t := range tasks {
		row := tableRow{color: taskColor(t.task)}
		row.add(
			t.alloc.JobID,
			t.alloc.Namespace,
			t.alloc.TaskGroup,
			t.name,
			shortID(t.allocID),
			t.alloc.NodeName,
			t.task.State,
			sprintf("%d", t.task.Restarts),
		)
		addTaskTimes(&row, t.task)

		rows = append(rows, row)
	}

	return rows
}

func (p namespaceTasksPage) pickedTask(e env) (allocTask, bool) { return pickedFrom(e, p.tasks()) }

func (p namespaceTasksPage) pickedAlloc(e env) (namespace, allocID string, ok bool) {
	task, ok := p.pickedTask(e)

	return task.namespace, task.allocID, ok
}

var namespaceTasksKeys = taskKeys[namespaceTasksPage]()

func (p namespaceTasksPage) keys(e env) []keyHint { return hintsOf(p, e, namespaceTasksKeys) }

func (p namespaceTasksPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, namespaceTasksKeys, k)
}
