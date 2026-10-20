package ui

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// onNamespaceTasks is the list the command `:tasks` opens, read from the client.
func onNamespaceTasks(t *testing.T, client *fakeClient, namespace string) Model {
	t.Helper()

	m := New(client, Options{
		Namespace: namespace, Version: "v-test", PollEvery: time.Millisecond,
		Shell: &fakeShell{},
	})
	m, _ = m.update(sizeMsg())
	m = typeCommand(m, "tasks")

	require.IsType(t, namespaceTasksPage{}, m.screen.page)

	return m
}

func TestNamespaceTasks_OneRowPerTaskOfEveryAllocation(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{allocs: twoAllocs()}
	m := onNamespaceTasks(t, client, "production")

	r.Contains(plain(m.render()), "Tasks (production) [2]")

	line := taskLine(t, m, "server")
	for _, cell := range []string{"web", "frontend", "server", "af1f37df", "node-01", "running"} {
		r.Contains(line, cell)
	}

	r.Contains(taskLine(t, m, "sidecar"), "dead")

	// The tasks of every allocation of the namespace, with no job.
	r.Equal("production/", client.lists["allocations"][0])
}

func TestNamespaceTasks_AskAgainInTheNamespaceTheSessionMovesTo(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{allocs: twoAllocs()}
	m := onNamespaceTasks(t, client, "production")
	m = elsewhere(m)
	m = playOut(m, m.fetch())

	r.Equal("staging/", client.lists["allocations"][len(client.lists["allocations"])-1])
	r.Contains(plain(m.render()), "Tasks (staging)")
}

func TestNamespaceTasks_KeepTheOrderOfTheClusterAndTheNamespaceOfTheirAllocation(t *testing.T) {
	r := require.New(t)

	page := namespaceTasksPage{allocs: []nomad.Alloc{
		{ID: "a1", Namespace: "production", Tasks: []nomad.Task{{Name: "a"}, {Name: "b"}}},
		{ID: "a0", Namespace: "staging", Tasks: []nomad.Task{{Name: "c"}}},
	}}

	var got []string
	for _, task := range page.tasks() {
		got = append(got, task.namespace+"/"+task.allocID+"/"+task.name)
	}

	// The allocations stay in the order the cluster answered, each task in
	// the namespace of its own allocation.
	r.Equal([]string{"production/a1/a", "production/a1/b", "staging/a0/c"}, got)
	r.Equal([]string{"a1/a", "a1/b", "a0/c"}, page.ids(env{}))
}

// Two jobs of the same name in two namespaces read apart by the namespace
// of their allocation, next to the job.
func TestNamespaceTasks_ARowShowsTheNamespaceOfItsAllocation(t *testing.T) {
	r := require.New(t)

	column := slices.Index(namespaceTaskTitles, "Namespace")
	r.Equal(slices.Index(namespaceTaskTitles, "Job")+1, column)

	rows := namespaceTaskRows([]allocTask{{alloc: nomad.Alloc{JobID: "web", Namespace: "staging"}}})

	r.Equal("staging", rows[0].cells[column])
}

func TestNamespaceTasks_RowsHaveTheColorsOfTheTasks(t *testing.T) {
	r := require.New(t)

	rows := namespaceTaskRows([]allocTask{
		{task: nomad.Task{State: "running"}},
		{task: nomad.Task{State: "dead", Failed: true}},
		{task: nomad.Task{State: "pending"}},
		{task: nomad.Task{State: "dead"}},
	})

	r.Nil(rows[0].color)
	r.Equal(colorDead, rows[1].color)
	r.Equal(colorPending, rows[2].color)
	r.Equal(colorSpent, rows[3].color)
}

func TestNamespaceTasks_ShowWhenTheyStartedAndEnded(t *testing.T) {
	r := require.New(t)

	now := time.Now()
	rows := namespaceTaskRows([]allocTask{
		{task: nomad.Task{State: "dead", Started: now.Add(-3 * time.Hour), Finished: now.Add(-1 * time.Hour)}},
	})

	started, finished, uptime := slices.Index(namespaceTaskTitles, "Started"), slices.Index(namespaceTaskTitles, "Finished"), slices.Index(namespaceTaskTitles, "Uptime")

	r.Equal([]string{"3h", "1h", "2h"}, []string{rows[0].cells[started], rows[0].cells[finished], rows[0].cells[uptime]})
}

func TestNamespaceTasks_TheHeaderShowsTheKeysOfATask(t *testing.T) {
	r := require.New(t)

	m := onNamespaceTasks(t, &fakeClient{allocs: twoAllocs()}, "production")

	r.Equal([]hint{
		{Key: "<enter>", Description: "Logs"},
		{Key: "<e>", Description: "Events"},
		{Key: "<o>", Description: "Alloc Events"},
		{Key: "<ctrl-e>", Description: "Stderr"},
		{Key: "<s>", Description: "Shell"},
		{Key: "<r>", Description: "Restart"},
		{Key: "<x>", Description: "Signal"},
		{Key: "<ctrl-x>", Description: "Run Command"},
		{Key: "<b>", Description: "Browse"},
	}, m.hints())
}

// staged is twoAllocs with the first allocation in staging, which the
// session lists with every namespace.
func staged() []nomad.Alloc {
	allocs := twoAllocs()
	allocs[0].Namespace = "staging"

	return allocs
}

func TestNamespaceTasks_LogsOfTheTaskInItsOwnAllocation(t *testing.T) {
	r := require.New(t)

	for press, source := range map[string]string{"<enter>": nomad.LogStdout, "<ctrl-e>": nomad.LogStderr} {
		m := onNamespaceTasks(t, &fakeClient{allocs: staged()}, nomad.AllNamespaces)

		next, _ := m.update(keyOf(press))

		logs, ok := next.screen.page.(logsPage)
		r.True(ok, press)
		r.Equal(logsPage{namespace: "staging", allocID: frontendAlloc, task: "server", source: source}, logs, press)
	}
}

func TestNamespaceTasks_EventsAndFilesOfTheTaskInItsOwnAllocation(t *testing.T) {
	r := require.New(t)

	m := onNamespaceTasks(t, &fakeClient{allocs: staged()}, nomad.AllNamespaces)

	events, _ := m.update(key('e'))
	page, ok := events.screen.page.(taskEventsPage)
	r.True(ok)
	r.Equal("staging", page.namespace)
	r.Equal(frontendAlloc, page.allocID)
	r.Equal("server", page.task)
	r.Equal(staged()[0].ID, page.alloc.ID)

	files, _ := m.update(key('b'))
	browsed, ok := files.screen.page.(filesPage)
	r.True(ok)
	r.Equal(filesPage{namespace: "staging", allocID: frontendAlloc, path: "/server"}, browsed)
}

func TestNamespaceTasks_EventsOfTheAllocationOfTheRowInItsOwnNamespace(t *testing.T) {
	r := require.New(t)

	m := onNamespaceTasks(t, &fakeClient{allocs: staged()}, nomad.AllNamespaces)

	next, _ := m.update(key('o'))
	events, ok := next.screen.page.(feedPage)
	r.True(ok)
	r.Equal("staging", events.namespace)
	r.Equal(nomad.Subject{AllocID: frontendAlloc}, events.subject)
}

func TestNamespaceTasks_ShellInTheTaskInItsOwnAllocation(t *testing.T) {
	r := require.New(t)

	shell := &fakeShell{}
	m := onNamespaceTasks(t, &fakeClient{allocs: staged()}, nomad.AllNamespaces)
	m.opts.Shell = shell

	m, cmd := m.update(key('s'))
	m = follow(m, cmd, 3)

	r.Equal("server", shell.opened.Task)
	r.Equal(frontendAlloc, shell.opened.AllocID)
	r.Equal("staging", shell.opened.Namespace)
}

func TestNamespaceTasks_RestartAndSignalReachTheTaskInItsOwnAllocation(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{allocs: staged()}
	m := onNamespaceTasks(t, client, nomad.AllNamespaces)

	m, _ = m.update(key('r'))
	r.Contains(plain(m.render()), "Really restart task server of the allocation af1f37df?")

	m = confirmed(t, m)

	r.Equal("server", client.restartedTask)
	r.Equal(frontendAlloc, client.askedID)
	r.Equal("staging", client.askedNamespace)

	client.askedID, client.askedNamespace = "", ""

	m, _ = m.update(key('x'))
	m, _ = m.update(enter())
	_ = confirmed(t, m)

	r.Equal("server", client.signalledTask)
	r.Equal(frontendAlloc, client.askedID)
	r.Equal("staging", client.askedNamespace)
}

func TestNamespaceTasks_ARestartAndASignalAreOfferedOnARunningTaskOnly(t *testing.T) {
	r := require.New(t)

	m := onNamespaceTasks(t, &fakeClient{allocs: twoAllocs()}, "production")
	r.True(offers(m, "r"))
	r.True(offers(m, "x"))

	// The sidecar is dead.
	m, _ = m.update(key('j'))

	r.False(offers(m, "r"))
	r.False(offers(m, "x"))
}

func TestNamespaceTasks_NoTasksNoKeysToAct(t *testing.T) {
	r := require.New(t)

	m := onNamespaceTasks(t, &fakeClient{}, "production")

	next, cmd := m.update(enter())
	r.Nil(cmd)
	r.IsType(namespaceTasksPage{}, next.screen.page)
}
