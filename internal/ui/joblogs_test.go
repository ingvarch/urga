package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// Allocations of web: two of frontend run, one finished; one of backend runs.
const (
	newer    = "aaaa1111-0000-0000-0000-000000000000"
	older    = "bbbb2222-0000-0000-0000-000000000000"
	finished = "cccc3333-0000-0000-0000-000000000000"
	backend  = "dddd4444-0000-0000-0000-000000000000"
)

// webAllocs are the allocations of web, each running the tasks named.
func webAllocs(tasks ...string) []nomad.Alloc {
	running := make([]nomad.Task, 0, len(tasks))
	for _, name := range tasks {
		running = append(running, nomad.Task{Name: name, State: "running"})
	}

	now := time.Now()

	return []nomad.Alloc{
		{ID: older, Namespace: "production", JobID: "web", TaskGroup: "frontend", Status: "running", Tasks: running, Created: now.Add(-2 * time.Hour)},
		{ID: newer, Namespace: "production", JobID: "web", TaskGroup: "frontend", Status: "running", Tasks: running, Created: now.Add(-time.Hour)},
		{ID: finished, Namespace: "production", JobID: "web", TaskGroup: "frontend", Status: "complete", Tasks: running, Created: now.Add(-3 * time.Hour)},
		{ID: backend, Namespace: "production", JobID: "web", TaskGroup: "backend", Status: "running", Tasks: []nomad.Task{{Name: "db", State: "running"}}, Created: now},
	}
}

// writing is a log that has sent the given lines and stays open.
func writing(lines ...string) *nomad.LogStream {
	out := make(chan string, len(lines))
	for _, line := range lines {
		out <- line
	}

	return &nomad.LogStream{Lines: out, Err: make(chan error)}
}

// fromJobs presses l on web in the list of jobs, and runs what it starts.
func fromJobs(t *testing.T, client *fakeClient) Model {
	t.Helper()

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))
	m, cmd := m.update(key('l'))

	return playOut(m, cmd)
}

func TestJobLogs_OfATaskGroupWithOneTask(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		jobs:   twoJobs(),
		groups: twoGroups(),
		allocs: webAllocs("server"),
		logsByAlloc: map[string]*nomad.LogStream{
			newer: writing("hello from the newer\n"),
			older: writing("hello from the older\n"),
		},
	}

	m := openTaskGroups(t, client)
	m, cmd := m.update(key('l'))
	m = playOut(m, cmd)

	// One task in the group: its log in every allocation that runs, the
	// newest first, each line tagged with the allocation it came from.
	r.IsType(jobLogsPage{}, m.screen.page)
	r.Equal([]string{newer, older}, client.logsOpened)
	r.Equal("server", client.askedTask)
	r.Equal(nomad.LogStdout, client.askedSource)
	r.Equal("production", client.askedNamespace)

	out := plain(m.render())
	r.Contains(out, "Logs (Job: web, Task: server) [stdout, 2 allocations]")
	r.Contains(out, "aaaa1111 │ hello from the newer")
	r.Contains(out, "bbbb2222 │ hello from the older")
	r.Contains(out, "Autoscroll:On")

	// Each allocation in a colour of its own.
	r.NotEqual(opening(m.text.tags[0].style), opening(m.text.tags[1].style))
}

func TestJobLogs_AskWhichTask(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		jobs:        twoJobs(),
		allocs:      webAllocs("server", "sidecar"),
		logsByAlloc: map[string]*nomad.LogStream{newer: writing(), older: writing(), backend: writing()},
	}

	m := fromJobs(t, client)

	// Every task of the job that runs somewhere, and where.
	r.IsType(logTasksPage{}, m.screen.page)
	r.Contains(plain(m.render()), "Logs of which task? (Job: web)")
	r.Contains(fileRow(t, m, 0), "backend/db")
	r.Contains(fileRow(t, m, 0), "1 running")
	r.Contains(fileRow(t, m, 1), "frontend/server")
	r.Contains(fileRow(t, m, 1), "2 running")
	r.Contains(fileRow(t, m, 2), "frontend/sidecar")
	r.Empty(client.logsOpened)

	m, _ = m.update(key('j'))
	m, _ = m.update(key('j'))

	asked := client.allocCalls
	m, cmd := m.update(enter())
	m = playOut(m, cmd)

	// The logs are read from the allocations the question already fetched.
	r.IsType(jobLogsPage{}, m.screen.page)
	r.Equal("sidecar", client.askedTask)
	r.Equal([]string{newer, older}, client.logsOpened)
	r.Equal(asked, client.allocCalls)

	// Escape goes back to the question, and from there to the jobs.
	m, _ = m.update(escape())
	r.IsType(logTasksPage{}, m.screen.page)
}

func TestJobLogs_OfTheAllocationsOnTheScreen(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		jobs:        twoJobs(),
		allocs:      webAllocs("server"),
		logsByAlloc: map[string]*nomad.LogStream{newer: writing(), older: writing(), backend: writing()},
	}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(enter())
	m, _ = m.update(allocsMsg(webAllocs("server")))

	m, cmd := m.update(key('l'))
	m = playOut(m, cmd)

	// The allocations of the job, and which task of theirs.
	r.IsType(logTasksPage{}, m.screen.page)
	r.Contains(fileRow(t, m, 1), "frontend/server")
}

func TestJobLogs_NothingRuns(t *testing.T) {
	r := require.New(t)

	stopped := webAllocs("server")
	for i := range stopped {
		stopped[i].Status = "complete"
	}

	client := &fakeClient{jobs: twoJobs(), allocs: stopped, logsByAlloc: map[string]*nomad.LogStream{}}
	m := fromJobs(t, client)

	r.IsType(jobsPage{}, m.screen.page)
	r.Contains(plain(m.render()), "web has no allocation running")
}

func TestJobLogs_NoMoreThanTwentyStreams(t *testing.T) {
	r := require.New(t)

	allocs := []nomad.Alloc{}
	logs := map[string]*nomad.LogStream{}

	for i := range 25 {
		id := fmt.Sprintf("%08d-0000-0000-0000-000000000000", i)
		allocs = append(allocs, nomad.Alloc{
			ID: id, Namespace: "production", JobID: "web", TaskGroup: "frontend", Status: "running",
			Tasks: []nomad.Task{{Name: "server", State: "running"}}, Created: time.Now().Add(time.Duration(i) * time.Minute),
		})
		logs[id] = writing()
	}

	client := &fakeClient{jobs: twoJobs(), allocs: allocs, logsByAlloc: logs}
	m := fromJobs(t, client)

	// The newest twenty, and how many were left out.
	r.Len(client.logsOpened, 20)
	r.Equal("00000024-0000-0000-0000-000000000000", client.logsOpened[0])
	r.Contains(plain(m.render()), "[stdout, 20 of 25 allocations]")
}

// oneTask is the log screen of the server of web, read from two
// allocations that go on writing.
func oneTask(t *testing.T) (Model, *fakeClient) {
	t.Helper()

	allocs := webAllocs("server")[:2]

	client := &fakeClient{
		jobs:        twoJobs(),
		allocs:      allocs,
		logsByAlloc: map[string]*nomad.LogStream{newer: writing(), older: writing()},
	}

	return fromJobs(t, client), client
}

func TestJobLogs_EveryLineOfAChunkIsTagged(t *testing.T) {
	r := require.New(t)

	m, client := oneTask(t)

	m, _ = m.update(logLineMsg{stream: client.logsByAlloc[newer], text: "ready\nserving on 8080\n"})

	// A chunk is as many lines as it holds, each after the tag of the
	// allocation, and the newline it ends with starts no line of its own.
	out := plain(m.render())
	r.Contains(out, "aaaa1111 │ ready")
	r.Contains(out, "aaaa1111 │ serving on 8080")
	r.Equal(2, rowsWith(m, "aaaa1111 │"))
}

func TestJobLogs_SayWhenALineArrived(t *testing.T) {
	r := require.New(t)

	m, client := oneTask(t)

	m, _ = m.update(logLineMsg{stream: client.logsByAlloc[newer], text: "ready\n"})
	m, _ = m.update(logEndMsg{stream: client.logsByAlloc[older]})
	m, _ = m.update(key('t'))

	// The time comes before the tag, on what a task wrote and on the notes
	// urga writes about the allocation alike.
	out := plain(m.render())
	r.Regexp(`\d\d:\d\d:\d\d  aaaa1111 │ ready`, out)
	r.Regexp(`\d\d:\d\d:\d\d  bbbb2222 │ stopped`, out)
}

func TestJobLogs_FollowTheEndUntilStopped(t *testing.T) {
	r := require.New(t)

	m, client := oneTask(t)

	m, _ = m.update(logLineMsg{stream: client.logsByAlloc[newer], text: strings.Join(longLines(40), "\n") + "\n"})
	m, _ = m.update(logEndMsg{stream: client.logsByAlloc[older]})

	out := plain(m.render())
	r.Contains(out, "bbbb2222 │ stopped")
	r.NotContains(out, "line-000")

	// With autoscroll off, the window stays where it is as lines arrive.
	m, _ = m.update(key('s'))
	m, _ = m.update(logLineMsg{stream: client.logsByAlloc[newer], text: "written later\n"})

	out = plain(m.render())
	r.Contains(out, "bbbb2222 │ stopped")
	r.NotContains(out, "written later")
}

func TestJobLogs_WhatUrgaSaysIsPaintedApart(t *testing.T) {
	r := require.New(t)

	// The older allocation has no log to open.
	client := &fakeClient{
		jobs:        twoJobs(),
		allocs:      webAllocs("server")[:2],
		logsByAlloc: map[string]*nomad.LogStream{newer: writing("ready\n")},
	}

	m := fromJobs(t, client)
	m, _ = m.update(logEndMsg{stream: client.logsByAlloc[newer]})

	out := m.render()
	r.Contains(out, styleText.Render("ready"))
	r.Contains(out, styleMuted.Render("stopped"))
	r.Contains(out, styleError.Render("could not read: no log for "+older))
}

func TestJobLogs_ALineOfAStreamNotReadHereIsDropped(t *testing.T) {
	r := require.New(t)

	m, _ := oneTask(t)

	// A stream of a reading that ended, or of another screen.
	m, _ = m.update(logLineMsg{stream: writing(), text: "from nowhere\n"})

	r.NotContains(plain(m.render()), "from nowhere")
}

// countClosed counts the streams closed from now on.
func countClosed(streams map[string]*nomad.LogStream) *int {
	closed := 0
	for _, stream := range streams {
		stream.OnClose = func() { closed++ }
	}

	return &closed
}

func TestJobLogs_CoveredClosesEveryStream(t *testing.T) {
	r := require.New(t)

	m, client := oneTask(t)
	closed := countClosed(client.logsByAlloc)

	// Each one is a request held open to a client: nothing reads it while
	// another screen is on top, and coming back reads them again.
	m, _ = m.show(deploymentsView)

	r.IsType(deploymentsPage{}, m.screen.page)
	r.Equal(2, *closed)
}

func TestJobLogs_EscapeClosesEveryStream(t *testing.T) {
	r := require.New(t)

	m, client := oneTask(t)
	closed := countClosed(client.logsByAlloc)

	m, _ = m.update(escape())

	r.IsType(jobsPage{}, m.screen.page)
	r.Equal(2, *closed)
}

func TestJobLogs_AStreamThatEnds(t *testing.T) {
	r := require.New(t)

	m, client := oneTask(t)

	m, _ = m.update(logEndMsg{stream: client.logsByAlloc[older]})

	// The others keep going; that one ends with a "stopped" line.
	r.Contains(plain(m.render()), "bbbb2222 │ stopped")
}

func TestJobLogs_AStreamOpenedAfterLeavingIsLetGo(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: webAllocs("server")[:2]}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, cmd := m.update(key('l'))
	m = drain(m, cmd)
	reading := m.screen.page.(jobLogsPage).logs.reading

	// Gone back before the log answered: nothing else would close it.
	m, _ = m.update(escape())

	closed := false
	late := &nomad.LogStream{Lines: make(chan string), OnClose: func() { closed = true }}
	m, _ = m.update(jobLogOpenedMsg{reading: reading, allocID: newer, stream: late})

	r.True(closed)
	r.IsType(jobsPage{}, m.screen.page)
}

func TestJobLogs_AStreamOfLogsLeftIsNotReadByOthers(t *testing.T) {
	r := require.New(t)

	m, client := oneTask(t)

	// Stderr is requested, and the session leaves the logs before it arrives.
	m, stderr := m.update(ctrlKey('e'))
	m, _ = m.update(escape())

	// The same task is read again, and read again once more.
	m, cmd := m.update(key('l'))
	m = playOut(m, cmd)
	m, _ = m.update(key('r'))

	closed := false
	late := &nomad.LogStream{Lines: make(chan string), OnClose: func() { closed = true }}
	client.logsByAlloc = map[string]*nomad.LogStream{newer: late}

	m = drain(m, stderr)

	// A stream opened for the logs that were left is closed with them.
	r.True(closed)
	r.NotContains(plain(m.render()), "could not read")
}

func TestJobLogs_AStreamOfAnEarlierReadingIsLetGo(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: webAllocs("server")[:2]}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	// The logs open, and are read again before their streams arrive.
	m, cmd := m.update(key('l'))
	m, opening := m.update(cmd())
	m, _ = m.update(key('r'))

	closed := 0
	early := &nomad.LogStream{Lines: make(chan string), OnClose: func() { closed++ }}
	client.logsByAlloc = map[string]*nomad.LogStream{newer: early, older: early}

	m = drain(m, opening)

	r.IsType(jobLogsPage{}, m.screen.page)
	r.Equal(2, closed)
}

func TestJobLogs_ReadAgain(t *testing.T) {
	r := require.New(t)

	m, client := oneTask(t)
	m, _ = m.update(logLineMsg{stream: client.logsByAlloc[newer], text: "read the first time\n"})

	// A deployment placed another allocation since the logs were opened.
	placed := webAllocs("server")[0]
	placed.ID, placed.Created = "eeee5555-0000-0000-0000-000000000000", time.Now()
	client.allocs = append(client.allocs, placed)
	client.logsByAlloc[placed.ID] = writing("from the new one\n")
	client.logsOpened = nil

	m, cmd := m.update(key('r'))
	m = playOut(m, cmd)

	// Read from the start: the screen is cleared and refilled with what the
	// cluster still keeps of each log.
	r.Equal([]string{placed.ID, newer, older}, client.logsOpened)
	r.Contains(plain(m.render()), "eeee5555 │ from the new one")
	r.Contains(plain(m.render()), "[stdout, 3 allocations]")
	r.NotContains(plain(m.render()), "read the first time")
}

func TestJobLogs_Stderr(t *testing.T) {
	r := require.New(t)

	m, client := oneTask(t)
	r.True(offersLabel(m, "ctrl-e", "Stderr"))
	m, _ = m.update(logLineMsg{stream: client.logsByAlloc[newer], text: "on stdout\n"})

	client.logsOpened = nil
	m, cmd := m.update(ctrl('e'))
	m = playOut(m, cmd)

	// The other of the two, from the same allocations.
	r.Equal(nomad.LogStderr, client.askedSource)
	r.Equal([]string{newer, older}, client.logsOpened)
	r.Contains(plain(m.render()), "[stderr, 2 allocations]")
	r.NotContains(plain(m.render()), "on stdout")
	r.True(offersLabel(m, "ctrl-e", "Stdout"))
}

func TestJobLogs_ComingBackReadsAgain(t *testing.T) {
	r := require.New(t)

	m, client := oneTask(t)

	m, _ = m.show(deploymentsView)
	client.logsOpened = nil

	m, cmd := m.update(escape())
	m = playOut(m, cmd)

	r.IsType(jobLogsPage{}, m.screen.page)
	r.Equal([]string{newer, older}, client.logsOpened)
}

func TestJobLogs_ALineWhileAwayStaysOut(t *testing.T) {
	r := require.New(t)

	m, client := oneTask(t)
	client.describe = "Job web, as the cluster describes it"

	// The logs stay behind, their streams closed, while a description is open.
	m, _ = m.show(jobsView)
	m, _ = m.update(jobsMsg(twoJobs()))
	m, cmd := m.update(key('d'))
	m = drain(m, cmd)
	r.IsType(describePage{}, m.screen.page)

	m, _ = m.update(logLineMsg{stream: client.logsByAlloc[newer], text: "written meanwhile\n"})

	r.NotContains(plain(m.render()), "written meanwhile")
}

func TestJobLogs_TheQuestionOffersToRead(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: webAllocs("server", "sidecar"), logsByAlloc: map[string]*nomad.LogStream{}}
	m := fromJobs(t, client)

	r.IsType(logTasksPage{}, m.screen.page)
	r.Equal([]hint{{Key: "<enter>", Description: "Logs"}}, m.hints())

	// Escape goes back to the jobs, and nothing was read.
	m, _ = m.update(escape())
	r.IsType(jobsPage{}, m.screen.page)
	r.Empty(client.logsOpened)
}

func TestJobLogs_TheHeaderSaysWhatTheLogsCanDo(t *testing.T) {
	r := require.New(t)

	m, _ := oneTask(t)

	r.Equal([]hint{
		{Key: "<r>", Description: "Reload"},
		{Key: "<ctrl-e>", Description: "Stderr"},
		{Key: "<s>", Description: "Toggle Autoscroll"},
		{Key: "<w>", Description: "Toggle Wrap"},
		{Key: "<t>", Description: "Toggle Timestamps"},
		{Key: "<ctrl-s>", Description: "Save"},
	}, m.hints())
}

func TestJobLogs_SavedUnderTheJobTheTaskAndWhatItWrites(t *testing.T) {
	r := require.New(t)

	dir := t.TempDir()
	t.Chdir(dir)

	m, _ := oneTask(t)

	m, cmd := m.update(ctrlKey('s'))
	drain(m, cmd)

	files, err := filepath.Glob(filepath.Join(dir, "web-server-stdout-*.log"))
	r.NoError(err)
	r.Len(files, 1)
}

func TestJobLogs_AnAnswerAfterLeavingOpensNothing(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		jobs:        twoJobs(),
		allocs:      webAllocs("server")[:2],
		logsByAlloc: map[string]*nomad.LogStream{newer: writing(), older: writing()},
	}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	// The allocations are requested, and the session opens another screen
	// before they answer.
	m, asked := m.update(key('l'))
	m, _ = m.show(deploymentsView)
	m = playOut(m, asked)

	r.IsType(deploymentsPage{}, m.screen.page)
	r.Empty(client.logsOpened)
}

func TestJobLogs_TheOtherSourceAndAReloadKeepTheToggles(t *testing.T) {
	r := require.New(t)

	m, _ := oneTask(t)
	m, _ = m.update(key('s'))
	m, _ = m.update(key('w'))

	m, cmd := m.update(ctrlKey('e'))
	m = playOut(m, cmd)

	out := plain(m.render())
	r.Contains(out, "[stderr, 2 allocations]")
	r.Contains(out, "Autoscroll:Off")
	r.Contains(out, "Wrap:On")

	m, cmd = m.update(key('r'))
	m = playOut(m, cmd)

	out = plain(m.render())
	r.Contains(out, "Autoscroll:Off")
	r.Contains(out, "Wrap:On")
}

func TestJobLogs_ATaskThatRunsNowhereNow(t *testing.T) {
	r := require.New(t)

	m, client := oneTask(t)

	// Every allocation of it was stopped since the logs were opened.
	client.allocs, client.logsOpened = nil, nil

	m, cmd := m.update(key('r'))
	m = playOut(m, cmd)

	out := plain(m.render())
	r.Contains(out, "server runs in no allocation now")
	r.Contains(out, "[stdout, 0 allocations]")
	r.Empty(client.logsOpened)
}

func TestJobLogs_DoNotPoll(t *testing.T) {
	r := require.New(t)

	m, _ := oneTask(t)

	// What a task writes arrives over its stream: a poll would open it again.
	_, cmd := m.update(pollMsg{})
	r.Nil(cmd)
}

func TestJobLogs_ASwitchOfTheSessionLeavesTheLogsAsTheyAre(t *testing.T) {
	r := require.New(t)

	m, client := oneTask(t)
	client.datacenters = []string{"dc1", "dc2"}
	m.namespaceOrder = []string{"production", "staging"}
	m, _ = m.update(datacentersMsg{names: client.datacenters})
	m, _ = m.update(key('w'))

	closed := countClosed(client.logsByAlloc)

	// The logs belong to their task: a namespace or a datacenter for the
	// lists changes nothing about them, and they are not read again.
	m, cmd := m.update(key('2'))
	m = playOut(m, cmd)
	m, cmd = runLine(m, "dc dc2")
	m = playOut(m, cmd)

	r.Equal("staging", m.namespace)
	r.Equal("dc2", m.datacenter)
	r.True(m.text.wrap)
	r.Zero(*closed)
	r.Contains(plain(m.render()), "Logs (Job: web, Task: server) [stdout, 2 allocations]")
}

func TestJobLogs_AStreamOfTheReadingIsKeptWhateverTheOrder(t *testing.T) {
	r := require.New(t)

	// A page with no streams yet gets the stream of its reading: it keeps
	// the stream and reads from it, without a crash.
	stream := &nomad.LogStream{Lines: make(chan string)}
	p := jobLogsPage{}

	next, out, ok := p.take(jobLogOpenedMsg{reading: p.logs.reading, allocID: "af1f37df", stream: stream}, env{})
	r.True(ok)
	r.NotNil(out.cmd)
	r.Equal("af1f37df", next.(jobLogsPage).logs.streams[stream])
}
