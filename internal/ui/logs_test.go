package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// openTasks walks down to the task list of the first allocation.
func openTasks(t *testing.T, client *fakeClient) Model {
	t.Helper()

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(enter())
	m, _ = m.update(allocsMsg(twoAllocs()))
	m, _ = m.update(enter())

	return m
}

func TestLogs_OpenOnATask(t *testing.T) {
	r := require.New(t)

	lines := make(chan string, 2)
	lines <- "first line\n"
	lines <- "second line\n"

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), logs: &nomad.LogStream{Lines: lines}}
	m := openTasks(t, client)

	m, cmd := m.update(enter())
	r.Equal(screenLogs, m.screen.kind)
	r.NotNil(cmd)

	// The stream is opened for the task the cursor was on, in the namespace
	// of its allocation.
	m = drain(m, cmd)
	r.Equal("server", client.askedTask)
	r.Equal("af1f37df-7b19-6b1c-da67-5e8f482b5a15", client.askedID)
	r.Equal("production", client.askedNamespace)

	// What the task writes lands on the screen as it comes.
	m = drain(m, m.logs.waitForLog())
	m = drain(m, m.logs.waitForLog())

	out := plain(m.render())
	r.Contains(out, "Logs (Task: server) [stdout]")
	r.Contains(out, "first line")
	r.Contains(out, "second line")
}

func TestLogs_LeavingClosesTheStream(t *testing.T) {
	r := require.New(t)

	stream := &nomad.LogStream{Lines: make(chan string)}
	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), logs: stream}

	m := openTasks(t, client)
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	m, _ = m.update(escape())

	// The request behind the stream is not left hanging.
	r.Equal(screenTasks, m.screen.kind)
	r.True(client.logsClosed)
}

func TestLogs_FollowCanBeStopped(t *testing.T) {
	r := require.New(t)

	lines := make(chan string, 1)
	lines <- "first line\n"

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), logs: &nomad.LogStream{Lines: lines}}
	m := openTasks(t, client)

	m, cmd := m.update(enter())
	m = drain(m, cmd)
	m = drain(m, m.logs.waitForLog())

	r.True(m.logs.following)

	// Stopping leaves the window where it is, resuming jumps back to the end.
	m, _ = m.update(key('s'))
	r.False(m.logs.following)

	m, _ = m.update(key('r'))
	r.True(m.logs.following)
}

func TestLogs_Stderr(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), logs: &nomad.LogStream{Lines: make(chan string)}}
	m := openTasks(t, client)

	m, cmd := m.update(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	m = drain(m, cmd)

	r.Equal(screenLogs, m.screen.kind)
	r.Equal(nomad.LogStderr, client.askedSource)
	r.Contains(plain(m.render()), "[stderr]")
}

func TestLogs_FilterNarrowsTheLines(t *testing.T) {
	r := require.New(t)

	lines := make(chan string, 3)
	lines <- "starting up\n"
	lines <- "error: cannot connect\n"
	lines <- "retrying\n"

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), logs: &nomad.LogStream{Lines: lines}}
	m := openTasks(t, client)

	m, cmd := m.update(enter())
	m = drain(m, cmd)

	for range 3 {
		m = drain(m, m.logs.waitForLog())
	}

	m, _ = m.update(key('/'))
	m = typeIn(m, "error")

	out := plain(m.render())
	r.Contains(out, "error: cannot connect")
	r.NotContains(out, "starting up")

	// Escape puts the rest of the output back.
	m, _ = m.update(escape())
	r.Contains(plain(m.render()), "starting up")
}

func TestLogs_ScrollingByHandStopsFollowing(t *testing.T) {
	r := require.New(t)

	lines := make(chan string, 1)
	lines <- "first line\n"

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), logs: &nomad.LogStream{Lines: lines}}
	m := openTasks(t, client)

	m, cmd := m.update(enter())
	m = drain(m, cmd)
	m = drain(m, m.logs.waitForLog())
	r.True(m.logs.following)

	// Reading back through the output must not be pulled to the end by the
	// next line.
	m, _ = m.update(key('k'))

	r.False(m.logs.following)
}

func TestLogs_AStreamThatEndedIsLetGoOf(t *testing.T) {
	r := require.New(t)

	lines := make(chan string)
	close(lines)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), logs: &nomad.LogStream{Lines: lines}}
	m := openTasks(t, client)

	m, cmd := m.update(enter())
	m = drain(m, cmd)

	// The task is done writing: there is nothing more to wait for, and
	// nothing to close on the way out.
	m = drain(m, m.logs.waitForLog())
	r.Nil(m.logs.waitForLog())

	m, _ = m.update(escape())

	r.Equal(screenTasks, m.screen.kind)
	r.False(client.logsClosed)
}

func TestLogs_OffersToStopOrToResumeFollowing(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), logs: &nomad.LogStream{Lines: make(chan string)}}
	m := openTasks(t, client)

	m, cmd := m.update(enter())
	m = drain(m, cmd)

	// A log that is followed can stop being followed, and only that: resume
	// would do nothing, and a key in the header is a promise.
	header := plain(m.render())
	r.Contains(header, "Stop following")
	r.NotContains(header, "Resume")

	m, _ = m.update(key('s'))

	header = plain(m.render())
	r.Contains(header, "Resume")
	r.NotContains(header, "Stop following")
}

// finishedLog is the output of a task that has stopped: what it wrote, and
// the end of it.
func finishedLog(lines ...string) *nomad.LogStream {
	out := make(chan string, len(lines))
	for _, line := range lines {
		out <- line
	}

	close(out)

	return &nomad.LogStream{Lines: out, Finished: true}
}

func TestLogs_AFinishedTaskSaysSo(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), logs: finishedLog("last words\n")}
	m := openTasks(t, client)

	m, cmd := m.update(enter())
	m = drain(m, cmd)
	m = drain(m, m.logs.waitForLog())
	m = drain(m, m.logs.waitForLog())

	// What a stopped task wrote is all there is: nothing to follow, and
	// the title says why nothing more arrives.
	out := plain(m.render())
	r.Contains(out, "last words")
	r.Contains(out, "Logs (Task: server) [stdout, task finished]")
	r.NotContains(out, "Stop following")
	r.NotContains(out, "Resume")
}

func TestLogs_TheNextLogStartsAfresh(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), logs: finishedLog()}
	m := openTasks(t, client)

	m, cmd := m.update(enter())
	m = drain(m, cmd)

	m, _ = m.update(escape())
	client.logs = &nomad.LogStream{Lines: make(chan string)}

	m, cmd = m.update(ctrlKey('e'))

	// What was said of one log is not said of the next, not even while its
	// stream is on its way.
	r.NotContains(plain(m.render()), "task finished")

	m = drain(m, cmd)

	out := plain(m.render())
	r.Contains(out, "Logs (Task: server) [stderr]")
	r.NotContains(out, "task finished")
}

// openLog opens the stdout of the first task on a stream of the test.
func openLog(t *testing.T, client *fakeClient, stream *nomad.LogStream) Model {
	t.Helper()

	client.logs = stream

	m := openTasks(t, client)
	m, cmd := m.update(enter())

	return drain(m, cmd)
}

func TestLogs_SwitchBetweenStdoutAndStderr(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openLog(t, client, &nomad.LogStream{Lines: make(chan string)})
	m, _ = m.update(key('w'))

	// The header says where the key goes: the other of the two.
	r.Contains(plain(m.render()), "Stderr")

	client.logs = &nomad.LogStream{Lines: make(chan string)}

	m, cmd := m.update(ctrlKey('e'))
	r.True(client.logsClosed, "the stream of stdout is still open")

	m = drain(m, cmd)

	r.Equal(nomad.LogStderr, client.askedSource)

	out := plain(m.render())
	r.Contains(out, "Logs (Task: server) [stderr]")
	r.Contains(out, "Stdout")

	// It is the same screen read another way: wrapped as it was, and
	// escape goes back to the tasks, not to stdout.
	r.True(m.text.wrap)

	m, _ = m.update(escape())
	r.Equal(screenTasks, m.screen.kind)
}

func TestLogs_ALineOfTheStreamLeftIsDropped(t *testing.T) {
	r := require.New(t)

	stdout := make(chan string, 1)
	stdout <- "from stdout\n"

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openLog(t, client, &nomad.LogStream{Lines: stdout})
	left := m.logs.stream

	// A read of stdout is on its way when the screen switches.
	late := m.logs.waitForLog()

	stderr := &nomad.LogStream{Lines: make(chan string)}
	client.logs = stderr

	m, cmd := m.update(ctrlKey('e'))
	m = drain(m, cmd)

	m, _ = m.update(late())
	r.NotContains(plain(m.render()), "from stdout")

	// Nor does the end of the stream left end the one that is open.
	close(stdout)

	m, _ = m.update(logState{stream: left}.waitForLog()())
	r.Same(stderr, m.logs.stream)
}

func TestLogs_AStreamOpenedForAnotherLogIsLetGo(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), logs: &nomad.LogStream{Lines: make(chan string)}}
	m := openTasks(t, client)

	// Stdout is asked for, and the screen switches before it arrives.
	m, openStdout := m.update(enter())

	stderr := &nomad.LogStream{Lines: make(chan string)}
	client.logs = stderr

	m, cmd := m.update(ctrlKey('e'))
	m = drain(m, cmd)

	client.logs = &nomad.LogStream{Lines: make(chan string)}
	client.logsClosed = false

	m = drain(m, openStdout)

	// The stream of stdout belongs to no screen: kept, it would feed stdout
	// into stderr, and nothing would ever close it.
	r.True(client.logsClosed)
	r.Same(stderr, m.logs.stream)
}

func TestLogs_OneStreamPerLog(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openTasks(t, client)

	// The same log is asked for twice before either answer arrives.
	m, first := m.update(enter())
	m, _ = m.update(escape())
	m, second := m.update(enter())

	kept := &nomad.LogStream{Lines: make(chan string)}
	client.logs = kept
	m = drain(m, first)

	client.logs = &nomad.LogStream{Lines: make(chan string)}
	client.logsClosed = false
	m = drain(m, second)

	r.True(client.logsClosed, "a second stream of the same log is left open")
	r.Same(kept, m.logs.stream)
}
