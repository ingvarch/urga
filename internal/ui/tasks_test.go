package ui

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// restarted is the first allocation of twoAllocs a while later: its task
// server restarted, and an event says why.
func restarted() nomad.Alloc {
	alloc := twoAllocs()[0]
	alloc.Tasks = []nomad.Task{
		{
			Name:     "server",
			State:    "running",
			Restarts: 7,
			Events: []nomad.TaskEvent{
				{Time: time.Now(), Type: "Restarting", Message: "Task restarting in 15s"},
			},
		},
		{Name: "sidecar", State: "dead", Failed: true},
	}

	return alloc
}

// taskLine is the line of the screen that shows the task.
func taskLine(t *testing.T, m Model, name string) string {
	t.Helper()

	for _, line := range lines(plain(m.render())) {
		if strings.Contains(line, name) {
			return line
		}
	}

	t.Fatalf("no row for %s", name)

	return ""
}

func TestTasks_AskForTheirAllocation(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), alloc: restarted()}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(enter())
	m, _ = m.update(allocsMsg(twoAllocs()))

	m, cmd := m.update(enter())
	m = drain(m, cmd)

	// The tasks are read from the cluster again, not taken from the list the
	// allocation was opened from: the restarts of a task show.
	r.Equal("af1f37df-7b19-6b1c-da67-5e8f482b5a15", client.askedID)
	r.Equal("production", client.askedNamespace)
	r.Contains(taskLine(t, m, "server"), "7")
}

func TestTasks_WatchAllocations(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), alloc: restarted(), changes: newChanges()}

	m := openTasks(t, client)
	m = elsewhere(m)
	fire(t, m.watchScreen())

	// A change to an allocation is a change to its tasks, in the namespace
	// the allocation lives in.
	r.Equal([]string{nomad.TopicAllocation}, client.watchedTopics)
	r.Equal("production", client.watchedNamespace)
}

func TestTaskEvents_ShowWhatHappensNext(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), alloc: restarted(), changes: newChanges()}

	m := openTasks(t, client)
	m, _ = m.update(key('e'))
	r.IsType(taskEventsPage{}, m.screen.page)

	m = drain(m, m.fetch())

	// The events of a task are read again like any list: the one that
	// happened after the screen was opened is on it.
	r.Contains(plain(m.render()), "Task restarting in 15s")
	r.Equal([]string{nomad.TopicAllocation}, m.screen.page.topics())
}

func TestTaskEvents_WatchTheAllocationWhereItLives(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), alloc: restarted(), changes: newChanges()}

	m := openTasks(t, client)
	m, _ = m.update(key('e'))
	m = elsewhere(m)
	fire(t, m.watchScreen())

	r.Equal("production", client.watchedNamespace)
}

// elsewhere moves the session from production, where the jobs of the tests
// live, to staging.
func elsewhere(m Model) Model {
	m.namespaceOrder = []string{"production", "staging"}
	m, _ = m.update(key('2'))

	return m
}

func TestTasks_AnswerForAnotherAllocationIsDropped(t *testing.T) {
	r := require.New(t)

	other := restarted()
	other.ID = "b2222222-0000-0000-0000-000000000000"

	m := openTasks(t, &fakeClient{jobs: twoJobs(), allocs: twoAllocs()})
	m, _ = m.update(allocMsg(other))

	// An answer for another allocation does not change this one.
	r.NotContains(taskLine(t, m, "server"), "7")
}

func TestTasks_TheKeysOfATask(t *testing.T) {
	r := require.New(t)

	// The list holds where the allocation runs and what it replaced.
	allocs := []nomad.Alloc{replacedCanary(), twoAllocs()[1]}
	client := &fakeClient{jobs: twoJobs(), allocs: allocs}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(enter())
	m, _ = m.update(allocsMsg(allocs))
	m, _ = m.update(enter())

	keys := []hint{
		{Key: "<enter>", Description: "Logs"},
		{Key: "<e>", Description: "Events"},
		{Key: "<o>", Description: "Alloc Events"},
		{Key: "<ctrl-e>", Description: "Stderr"},
		{Key: "<s>", Description: "Shell"},
		{Key: "<r>", Description: "Restart"},
		{Key: "<x>", Description: "Signal"},
		{Key: "<b>", Description: "Browse"},
	}
	r.Equal(keys, m.hints())

	// Where the allocation runs, what it replaced, what replaced it and
	// what places it again are offered once it is read.
	client.alloc = replacedCanary()
	m = drain(m, m.fetch())

	r.Equal(append(keys,
		hint{Key: "<c>", Description: "Client"},
		hint{Key: "<p>", Description: "Previous"},
		hint{Key: "<n>", Description: "Next"},
		hint{Key: "<f>", Description: "Follow-up"},
	), m.hints())
}

func TestTasks_AllocEventsWithNoTaskToPick(t *testing.T) {
	r := require.New(t)

	// Its client has not taken it yet: there is no task under the cursor.
	m := newTestModel(&fakeClient{})
	m.namespace = nomad.AllNamespaces
	m, _ = m.update(openMsg{listedTasks(nomad.Alloc{ID: frontendAlloc, Namespace: "ml", Status: "pending"})})

	next, _ := m.update(key('o'))

	events, ok := next.screen.page.(feedPage)
	r.True(ok)
	r.Equal("ml", events.namespace)
	r.Equal(nomad.Subject{AllocID: frontendAlloc}, events.subject)
}

func TestTasks_ThePanelWaitsForTheAllocationToBeRead(t *testing.T) {
	r := require.New(t)

	m := openTasks(t, &fakeClient{jobs: twoJobs(), allocs: twoAllocs()})

	// The tasks are those the list held; the panel about the allocation
	// waits until the allocation is read.
	out := plain(m.render())
	r.Contains(out, "sidecar")
	r.NotContains(out, "Status running")
}

func TestTasks_ShowWhenATaskEndedAndHowLongItRan(t *testing.T) {
	r := require.New(t)

	now := time.Now()
	tasks := []nomad.Task{
		{Name: "running", State: "running", Started: now.Add(-3 * time.Hour)},
		{Name: "ended", State: "dead", Started: now.Add(-3 * time.Hour), Finished: now.Add(-1 * time.Hour)},
		{Name: "waiting", State: "pending"},
		// Restarted since it ended: the old end is before the new start.
		{Name: "again", State: "dead", Started: now.Add(-1 * time.Hour), Finished: now.Add(-3 * time.Hour)},
		// Waiting to restart: the times are those of the earlier run.
		{Name: "restarting", State: "pending", Started: now.Add(-3 * time.Hour), Finished: now.Add(-2 * time.Hour)},
	}

	rows := taskRows(tasks, nil)
	finished, uptime := slices.Index(taskTitles, "Finished"), slices.Index(taskTitles, "Uptime")

	cells := func(i int) []string { return []string{rows[i].cells[finished], rows[i].cells[uptime]} }

	r.Equal([]string{"-", "3h"}, cells(0))
	r.Equal([]string{"1h", "2h"}, cells(1))
	r.Equal([]string{"-", "-"}, cells(2))
	r.Equal([]string{"-", "-"}, cells(3))
	r.Equal([]string{"-", "-"}, cells(4))
}
