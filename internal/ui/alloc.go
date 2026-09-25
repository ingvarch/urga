package ui

import (
	"context"
	"fmt"
	"image/color"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// allocationsPage is the allocations of a job, or of one group of it. Opened
// by name it has no job, and lists those of the namespace the session looks
// at.
type allocationsPage struct {
	namespace, jobID, group string

	allocs []nomad.Alloc
}

func (p allocationsPage) title(e env, count int) string {
	if p.group != "" {
		return sprintf("Allocations (Group: %s) [%d]", p.group, count)
	}

	// The command line opens the allocations of the namespace, with no job
	// to name.
	if p.jobID == "" {
		return sprintf("Allocations (%s) [%d]", namespaceLabel(e.namespace), count)
	}

	return sprintf("Allocations (Job: %s) [%d]", p.jobID, count)
}

// followsSession: the allocations the command line opens are those of the
// namespace the session looks at; those of a job stay where the job lives.
func (p allocationsPage) followsSession() bool { return p.jobID == "" }

func (allocationsPage) titles() []string { return allocTitles }
func (allocationsPage) topics() []string { return []string{nomad.TopicAllocation} }

// where is the namespace the allocations are asked in: the one of the job,
// or the session's for those of no job.
func (p allocationsPage) where(e env) string {
	if p.jobID == "" {
		return e.namespace
	}

	return p.namespace
}

func (p allocationsPage) fetch(e env) tea.Cmd {
	client, namespace, jobID := e.client, p.where(e), p.jobID

	return fetchList(func(ctx context.Context) ([]nomad.Alloc, error) {
		return client.Allocations(ctx, namespace, jobID)
	}, func(items []nomad.Alloc) tea.Msg { return allocsMsg(items) })
}

func (p allocationsPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	allocs, ok := msg.(allocsMsg)
	if !ok {
		return p, outcome{}, false
	}

	p.allocs = allocs

	return p, outcome{}, true
}

// visible are the allocations the page was opened for: all it read, or only
// those of one task group.
func (p allocationsPage) visible(env) []nomad.Alloc {
	if p.group == "" {
		return p.allocs
	}

	return keep(p.allocs, func(alloc nomad.Alloc) bool { return alloc.TaskGroup == p.group })
}

func (p allocationsPage) rows(e env) []tableRow { return allocRows(p.visible(e), e.usage) }

func (p allocationsPage) ids(e env) []string { return names(p.visible(e), allocMark) }

func (p allocationsPage) readings(e env) []rowRef { return runningRefs(e.index, p.visible(e)) }

func (allocationsPage) reading(ctx context.Context, client Client, ref rowRef) (nomad.ResourceUse, error) {
	return allocReading(ctx, client, ref)
}

// logScope is what the logs of the page are read of: the allocations of the
// job, of the group, or of the namespace.
func (p allocationsPage) logScope(e env) screen {
	return screen{kind: screenAllocations, namespace: p.where(e), jobID: p.jobID, taskGroup: p.group}
}

var allocationsKeys = allocKeys[allocationsPage]()

func (p allocationsPage) keys(e env) []keyHint { return hintsOf(p, e, allocationsKeys) }

func (p allocationsPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, allocationsKeys, k)
}

// allocLister is a page that lists allocations: of a job, of a client, of a
// deployment. The keys of an allocation answer on every one of them.
type allocLister interface {
	page

	// visible are the allocations of the rows, in their order.
	visible(e env) []nomad.Alloc

	// logScope is what the logs of the list are read of.
	logScope(e env) screen
}

// allocKeys are the keys of an allocation, for a page that lists them. A
// page with keys of its own puts them first.
func allocKeys[P allocLister]() []pageKey[P] {
	return []pageKey[P]{
		{press: "enter", label: "Tasks", do: openAllocTasks[P]},
		{press: "d", label: "Describe", do: describeAlloc[P]},
		{press: "r", label: "Restart", do: restartAllocs[P], writes: true},
		{press: "ctrl+k", label: "Stop", do: stopAllocs[P], writes: true},
		{press: "l", label: "Logs", do: allocLogs[P]},
		markKey[P](),
		markAllKey[P](),
	}
}

// openAllocTasks drills into the allocation under the cursor: the tasks it
// runs.
func openAllocTasks[P allocLister](p P, e env) (P, outcome) {
	alloc, ok := pickedFrom(e, p.visible(e))
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg(tasksScreen(listedTasks(alloc))))
}

// describeAlloc asks for the allocation under the cursor, in the words of
// the cluster.
func describeAlloc[P allocLister](p P, e env) (P, outcome) {
	alloc, ok := pickedFrom(e, p.visible(e))
	if !ok {
		return p, outcome{}
	}

	return p, outcome{cmd: allocDescription(e.client, alloc)}
}

// allocDescription is an allocation in the words of the cluster.
func allocDescription(client allocsClient, alloc nomad.Alloc) tea.Cmd {
	return describe(fmt.Sprintf("Allocation: %s", shortID(alloc.ID)), func(ctx context.Context) (string, error) {
		return client.DescribeAllocation(ctx, alloc.Namespace, alloc.ID)
	})
}

// restartAllocs restarts every task of the allocations the key takes.
func restartAllocs[P allocLister](p P, e env) (P, outcome) {
	return p, eachAllocAsked(e, p.visible(e), restarting(e.client))
}

// stopAllocs stops the allocations the key takes. The scheduler places new
// ones where the job still asks for them.
func stopAllocs[P allocLister](p P, e env) (P, outcome) {
	return p, eachAllocAsked(e, p.visible(e), stopping(e.client))
}

// eachAllocAsked asks about the allocations the key takes: the marked ones,
// or the one under the cursor.
func eachAllocAsked(e env, allocs []nomad.Alloc, action allocAction) outcome {
	taken := markedFrom(e, allocs, allocMark)
	if len(taken) == 0 {
		return outcome{}
	}

	return then(action.asked(taken))
}

// allocLogs reads the logs of the allocations of the list: which task, then
// that task in every allocation that runs it.
func allocLogs[P allocLister](p P, e env) (P, outcome) {
	return p, then(jobLogsMsg(p.logScope(e)))
}

// runningRefs are the allocations whose usage the rows on the screen show:
// index says which of them are on it. Only what runs has anything to report.
func runningRefs(index []int, allocs []nomad.Alloc) []rowRef {
	refs := make([]rowRef, 0, len(index))
	for _, at := range index {
		if at < len(allocs) && allocs[at].Status == statusRunning {
			refs = append(refs, rowRef{namespace: allocs[at].Namespace, id: allocs[at].ID})
		}
	}

	return refs
}

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

const (
	statusComplete = "complete"
	statusLost     = "lost"

	desiredStop = "stop"
)

// allocReading reads what one allocation takes.
func allocReading(ctx context.Context, client Client, ref rowRef) (nomad.ResourceUse, error) {
	return client.AllocationUsage(ctx, ref.namespace, ref.id)
}
