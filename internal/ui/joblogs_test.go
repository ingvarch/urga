package ui

import (
	"fmt"
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

// writing is a log that has said what it holds and goes on.
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
	// newest first, each line saying where it came from.
	r.Equal(screenJobLogs, m.screen.kind)
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
	r.Equal(screenLogTasks, m.screen.kind)
	r.Contains(plain(m.render()), "Logs of which task? (Job: web)")
	r.Contains(fileRow(t, m, 0), "backend/db")
	r.Contains(fileRow(t, m, 0), "1 running")
	r.Contains(fileRow(t, m, 1), "frontend/server")
	r.Contains(fileRow(t, m, 1), "2 running")
	r.Contains(fileRow(t, m, 2), "frontend/sidecar")
	r.Empty(client.logsOpened)

	m, _ = m.update(key('j'))
	m, _ = m.update(key('j'))
	m, cmd := m.update(enter())
	m = playOut(m, cmd)

	r.Equal(screenJobLogs, m.screen.kind)
	r.Equal("sidecar", client.askedTask)
	r.Equal([]string{newer, older}, client.logsOpened)

	// Escape goes back to the question, and from there to the jobs.
	m, _ = m.update(escape())
	r.Equal(screenLogTasks, m.screen.kind)
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
	r.Equal(screenLogTasks, m.screen.kind)
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

	r.Equal(screenJobs, m.screen.kind)
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

func TestJobLogs_ALineOfAStreamNotReadHereIsDropped(t *testing.T) {
	r := require.New(t)

	m, _ := oneTask(t)

	// A stream of a reading that ended, or of another screen.
	m, _ = m.update(logLineMsg{stream: writing(), text: "from nowhere\n"})

	r.NotContains(plain(m.render()), "from nowhere")
}

func TestJobLogs_EscapeClosesEveryStream(t *testing.T) {
	r := require.New(t)

	m, client := oneTask(t)

	closed := 0
	for _, stream := range client.logsByAlloc {
		stream.OnClose = func() { closed++ }
	}

	m, _ = m.update(escape())

	r.Equal(screenJobs, m.screen.kind)
	r.Equal(2, closed)
}

func TestJobLogs_AStreamThatEnds(t *testing.T) {
	r := require.New(t)

	m, client := oneTask(t)

	m, _ = m.update(logEndMsg{stream: client.logsByAlloc[older]})

	// The others go on; that one says it stopped.
	r.Contains(plain(m.render()), "bbbb2222 │ stopped")
}

func TestJobLogs_AStreamOpenedAfterLeavingIsLetGo(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: webAllocs("server")[:2]}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, cmd := m.update(key('l'))
	m = drain(m, cmd)
	reading := m.jobLogs.reading

	// Gone back before the log answered: nothing would ever close it.
	m, _ = m.update(escape())

	closed := false
	late := &nomad.LogStream{Lines: make(chan string), OnClose: func() { closed = true }}
	m, _ = m.update(jobLogOpenedMsg{reading: reading, allocID: newer, stream: late})

	r.True(closed)
	r.Equal(screenJobs, m.screen.kind)
}

func TestJobLogs_ReadAgain(t *testing.T) {
	r := require.New(t)

	m, client := oneTask(t)

	// A deployment placed another allocation since the logs were opened.
	placed := webAllocs("server")[0]
	placed.ID, placed.Created = "eeee5555-0000-0000-0000-000000000000", time.Now()
	client.allocs = append(client.allocs, placed)
	client.logsByAlloc[placed.ID] = writing("from the new one\n")
	client.logsOpened = nil

	m, cmd := m.update(key('r'))
	m = playOut(m, cmd)

	r.Equal([]string{placed.ID, newer, older}, client.logsOpened)
	r.Contains(plain(m.render()), "eeee5555 │ from the new one")
	r.Contains(plain(m.render()), "[stdout, 3 allocations]")
}

func TestJobLogs_Stderr(t *testing.T) {
	r := require.New(t)

	m, client := oneTask(t)
	r.True(offersLabel(m, "ctrl-e", "Stderr"))

	client.logsOpened = nil
	m, cmd := m.update(ctrl('e'))
	m = playOut(m, cmd)

	// The other of the two, from the same allocations.
	r.Equal(nomad.LogStderr, client.askedSource)
	r.Equal([]string{newer, older}, client.logsOpened)
	r.Contains(plain(m.render()), "[stderr, 2 allocations]")
	r.True(offersLabel(m, "ctrl-e", "Stdout"))
}

func TestJobLogs_ComingBackReadsAgain(t *testing.T) {
	r := require.New(t)

	m, client := oneTask(t)

	m, _ = m.show(screenDeployments)
	client.logsOpened = nil

	m, cmd := m.update(escape())
	m = playOut(m, cmd)

	r.Equal(screenJobLogs, m.screen.kind)
	r.Equal([]string{newer, older}, client.logsOpened)
}

func TestJobLogs_ALineWhileAwayStaysOut(t *testing.T) {
	r := require.New(t)

	m, client := oneTask(t)
	client.describe = "Job web, as the cluster describes it"

	// The logs stay behind, open, while a description is read on top.
	m, _ = m.show(screenJobs)
	m, _ = m.update(jobsMsg(twoJobs()))
	m, cmd := m.update(key('d'))
	m = drain(m, cmd)
	r.Equal(screenDescribe, m.screen.kind)

	m, _ = m.update(logLineMsg{stream: client.logsByAlloc[newer], text: "written meanwhile\n"})

	r.NotContains(plain(m.render()), "written meanwhile")
}
