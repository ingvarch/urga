package ui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// logOf is what the open log reads its stream through.
func logOf(m Model) logState {
	p, _ := m.screen.page.(logsPage)

	return p.read
}

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
	m = drain(m, logOf(m).waitForLog())
	m = drain(m, logOf(m).waitForLog())

	out := plain(m.render())
	r.Contains(out, "Logs (Task: server, Allocation: af1f37df) [stdout]")
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
	m = drain(m, logOf(m).waitForLog())

	r.Contains(plain(m.render()), "Autoscroll:On")

	// Turned off, autoscroll leaves the window where it is; turned on, it
	// jumps back to the end.
	m, _ = m.update(key('s'))
	r.Contains(plain(m.render()), "Autoscroll:Off")

	m, _ = m.update(key('s'))
	r.Contains(plain(m.render()), "Autoscroll:On")
}

func TestLogs_TheLogStartsRightUnderTheToggles(t *testing.T) {
	r := require.New(t)

	written := make(chan string, 1)
	written <- "first line\n"

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), logs: &nomad.LogStream{Lines: written}}
	m := openTasks(t, client)

	m, cmd := m.update(enter())
	m = drain(m, cmd)
	m = drain(m, logOf(m).waitForLog())

	// A log starts with what the task wrote, not with an empty line.
	rows := lines(m.render())
	for i, row := range rows {
		if strings.Contains(row, "Autoscroll:") {
			r.Contains(rows[i+1], "first line")

			return
		}
	}

	t.Fatal("no line of toggles")
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
		m = drain(m, logOf(m).waitForLog())
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
	m = drain(m, logOf(m).waitForLog())
	r.Contains(plain(m.render()), "Autoscroll:On")

	// Reading back through the output must not be pulled to the end by the
	// next line.
	m, _ = m.update(key('k'))

	r.Contains(plain(m.render()), "Autoscroll:Off")
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
	m = drain(m, logOf(m).waitForLog())
	r.Nil(logOf(m).waitForLog())

	m, _ = m.update(escape())

	r.Equal(screenTasks, m.screen.kind)
	r.False(client.logsClosed)
}

func TestLogs_OneKeyTogglesAutoscroll(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openLog(t, client, &nomad.LogStream{Lines: make(chan string)})

	// One key turns following the end on and off, and the line under the
	// title says which it is: two keys for one switch say it twice.
	out := plain(m.render())
	r.Contains(out, "Toggle Autoscroll")
	r.Contains(out, "Autoscroll:On")
	r.NotContains(out, "Stop following")
	r.NotContains(out, "Resume")

	m, _ = m.update(key('s'))
	r.Contains(plain(m.render()), "Autoscroll:Off")

	m, _ = m.update(key('s'))
	r.Contains(plain(m.render()), "Autoscroll:On")
}

func TestLogs_SayWhatIsOnAndWhatIsOff(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openLog(t, client, &nomad.LogStream{Lines: make(chan string)})

	out := plain(m.render())
	r.Contains(out, "Timestamps:Off")
	r.Contains(out, "Wrap:Off")

	m, _ = m.update(key('w'))
	m, _ = m.update(key('t'))

	out = plain(m.render())
	r.Contains(out, "Timestamps:On")
	r.Contains(out, "Wrap:On")

	// On is what the eye looks for, off steps back.
	raw := m.render()
	r.Contains(raw, styleTitle.Render("On"))

	m, _ = m.update(key('s'))
	r.Contains(m.render(), styleMuted.Render("Off"))
}

func TestLogs_TheLineOfTogglesTakesNoLineOfTheLog(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openLog(t, client, &nomad.LogStream{Lines: make(chan string)})

	for i := range 100 {
		m, _ = m.update(lineOf(m, fmt.Sprintf("line-%03d\n", i)))
	}

	// Followed, the last line of the log is the last line in the box,
	// under the line of toggles, not pushed out by it.
	out := plain(m.render())
	r.Contains(out, "line-099")
	r.Contains(out, "Autoscroll:On")
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
	m = drain(m, logOf(m).waitForLog())
	m = drain(m, logOf(m).waitForLog())

	// What a stopped task wrote is all there is: nothing to follow, and
	// the title says why nothing more arrives.
	out := plain(m.render())
	r.Contains(out, "last words")
	r.Contains(out, "Logs (Task: server, Allocation: af1f37df) [stdout, task finished]")
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
	r.Contains(out, "Logs (Task: server, Allocation: af1f37df) [stderr]")
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
	r.Contains(out, "Logs (Task: server, Allocation: af1f37df) [stderr]")
	r.Contains(out, "Stdout")

	// It is the same screen read another way: wrapped as it was, and
	// escape goes back to the tasks, not to stdout.
	r.Contains(out, "Wrap:On")

	m, _ = m.update(escape())
	r.Equal(screenTasks, m.screen.kind)
}

func TestLogs_TheOtherSourceIsFollowed(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openLog(t, client, &nomad.LogStream{Lines: make(chan string)})

	m, _ = m.update(key('s'))
	r.Contains(plain(m.render()), "Autoscroll:Off")

	// Read from its end again: what stood still was the other one.
	client.logs = &nomad.LogStream{Lines: make(chan string)}
	m, cmd := m.update(ctrlKey('e'))
	m = drain(m, cmd)

	r.Contains(plain(m.render()), "Autoscroll:On")
}

func TestLogs_ALineOfTheStreamLeftIsDropped(t *testing.T) {
	r := require.New(t)

	stdout := make(chan string, 1)
	stdout <- "from stdout\n"

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openLog(t, client, &nomad.LogStream{Lines: stdout})
	left := logOf(m).stream

	// A read of stdout is on its way when the screen switches.
	late := logOf(m).waitForLog()

	stderr := &nomad.LogStream{Lines: make(chan string)}
	client.logs = stderr

	m, cmd := m.update(ctrlKey('e'))
	m = drain(m, cmd)

	m, _ = m.update(late())
	r.NotContains(plain(m.render()), "from stdout")

	// Nor does the end of the stream left end the one that is open.
	close(stdout)

	m, _ = m.update(logState{stream: left}.waitForLog()())
	r.Same(stderr, logOf(m).stream)
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
	r.Same(stderr, logOf(m).stream)
}

func TestLogs_AStreamOfALogLeftIsLetGo(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openTasks(t, client)

	// The same log is asked for twice before either answer arrives: the
	// first time for a screen that was left before it did.
	m, first := m.update(enter())
	m, _ = m.update(escape())
	m, second := m.update(enter())

	client.logs = &nomad.LogStream{Lines: make(chan string)}
	m = drain(m, first)
	r.True(client.logsClosed, "the stream of a log that was left is still open")

	kept := &nomad.LogStream{Lines: make(chan string)}
	client.logs = kept
	client.logsClosed = false
	m = drain(m, second)

	r.False(client.logsClosed)
	r.Same(kept, logOf(m).stream)
}

func TestLogs_TheSourceLeftIsNotRead(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openTasks(t, client)

	// Stdout is asked for, the log switches to stderr, and stdout answers
	// first.
	m, openStdout := m.update(enter())
	m, openStderr := m.update(ctrlKey('e'))

	client.logs = &nomad.LogStream{Lines: make(chan string)}
	m = drain(m, openStdout)
	r.True(client.logsClosed, "the stream of stdout is still open")

	stderr := &nomad.LogStream{Lines: make(chan string)}
	client.logs = stderr
	m = drain(m, openStderr)

	r.Same(stderr, logOf(m).stream)
}

func TestLogs_ALineLeavesTheErrorUp(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openLog(t, client, &nomad.LogStream{Lines: make(chan string)})

	m, _ = m.update(errMsg{err: errors.New("disk full")})

	// What the task writes is no answer of the cluster: what went wrong
	// stays on the status line.
	m, _ = m.update(lineOf(m, "ready\n"))

	r.Contains(plain(m.render()), "disk full")
}

func TestLogs_OneStreamPerLog(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openLog(t, client, &nomad.LogStream{Lines: make(chan string)})

	// Over to stderr, back and over again before any answer arrives: two
	// answers are stderr, and one of them is read.
	m, toStderr := m.update(ctrlKey('e'))
	m, toStdout := m.update(ctrlKey('e'))
	m, again := m.update(ctrlKey('e'))

	kept := &nomad.LogStream{Lines: make(chan string)}
	client.logs = kept
	m = drain(m, toStderr)

	client.logs = &nomad.LogStream{Lines: make(chan string)}
	client.logsClosed = false
	m = drain(m, toStdout)
	r.True(client.logsClosed, "the stream of stdout is left open")

	client.logs = &nomad.LogStream{Lines: make(chan string)}
	client.logsClosed = false
	m = drain(m, again)
	r.True(client.logsClosed, "a second stream of the same log is left open")

	r.Same(kept, logOf(m).stream)
}

// replaced is an allocation of twoAllocs placed again: the one it replaced
// is named on its stream.
const replaced = "0ld0a11c-0000-0000-0000-000000000000"

func TestLogs_OpenTheAllocationBefore(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openLog(t, client, &nomad.LogStream{Lines: make(chan string), Previous: replaced})

	// A task that crashed was placed again, and its new log starts empty:
	// why it crashed is in the allocation it replaced.
	r.Contains(plain(m.render()), "Previous Alloc")

	client.logs = &nomad.LogStream{Lines: make(chan string)}

	m, cmd := m.update(key('p'))
	r.True(client.logsClosed, "the stream of the newer allocation is still open")

	m = drain(m, cmd)

	r.Equal(replaced, client.askedID)
	r.Equal("server", client.askedTask)
	r.Equal(nomad.LogStdout, client.askedSource)
	r.Contains(plain(m.render()), "Logs (Task: server, Allocation: 0ld0a11c) [stdout]")

	// Escape comes back to the newer log, read again: the text on the
	// screen is not the log of the older one.
	client.logs = &nomad.LogStream{Lines: make(chan string)}

	m, cmd = m.update(escape())
	m = drain(m, cmd)

	r.Equal(screenLogs, m.screen.kind)
	r.Equal(twoAllocs()[0].ID, client.askedID)
	r.Same(client.logs, logOf(m).stream)
	r.Contains(plain(m.render()), "Logs (Task: server, Allocation: af1f37df) [stdout]")
}

func TestLogs_TheFirstAllocationHasNoneBefore(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openLog(t, client, &nomad.LogStream{Lines: make(chan string)})

	r.NotContains(plain(m.render()), "Previous Alloc")
}

func TestLogs_BackOnALogReadsItAgain(t *testing.T) {
	r := require.New(t)

	old := make(chan string, 1)
	old <- "why it crashed\n"

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openLog(t, client, &nomad.LogStream{Lines: make(chan string), Previous: replaced})

	client.logs = &nomad.LogStream{Lines: old}

	m, cmd := m.update(key('p'))
	m = drain(m, cmd)

	// The line is read the way the program reads it, and a stream that
	// holds none is not waited on for good.
	if line := within(logOf(m).waitForLog(), time.Second); line != nil {
		m, _ = m.update(line)
	}

	r.Contains(plain(m.render()), "why it crashed")

	client.logs = &nomad.LogStream{Lines: make(chan string)}

	m, cmd = m.update(escape())
	m = drain(m, cmd)

	r.NotContains(plain(m.render()), "why it crashed")
}

func TestLogs_TheHeaderSaysWhatALogCanDo(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openLog(t, client, &nomad.LogStream{Lines: make(chan string)})

	r.Equal([]hint{
		{Key: "<s>", Description: "Toggle Autoscroll"},
		{Key: "<ctrl-e>", Description: "Stderr"},
		{Key: "<w>", Description: "Toggle Wrap"},
		{Key: "<t>", Description: "Toggle Timestamps"},
		{Key: "<ctrl-s>", Description: "Save"},
	}, m.hints())

	// The allocation before is offered where there is one.
	m = openLog(t, client, &nomad.LogStream{Lines: make(chan string), Previous: replaced})

	r.Equal([]hint{
		{Key: "<s>", Description: "Toggle Autoscroll"},
		{Key: "<ctrl-e>", Description: "Stderr"},
		{Key: "<p>", Description: "Previous Alloc"},
		{Key: "<w>", Description: "Toggle Wrap"},
		{Key: "<t>", Description: "Toggle Timestamps"},
		{Key: "<ctrl-s>", Description: "Save"},
	}, m.hints())
}

func TestLogs_DoNotPoll(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openLog(t, client, &nomad.LogStream{Lines: make(chan string)})

	// What a task writes arrives on its own: a poll would open it again.
	_, cmd := m.update(pollMsg{})
	r.Nil(cmd)
}

func TestLogs_SavedUnderTheTaskAndWhatItWrites(t *testing.T) {
	r := require.New(t)

	dir := t.TempDir()
	t.Chdir(dir)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openLog(t, client, &nomad.LogStream{Lines: make(chan string)})

	client.logs = &nomad.LogStream{Lines: make(chan string)}
	m, cmd := m.update(ctrlKey('e'))
	m = drain(m, cmd)

	m, cmd = m.update(ctrlKey('s'))
	drain(m, cmd)

	files, err := filepath.Glob(filepath.Join(dir, "server-stderr-*.log"))
	r.NoError(err)
	r.Len(files, 1)
}

func TestLogs_TheTogglesStandInTheMiddle(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openLog(t, client, &nomad.LogStream{Lines: make(chan string)})

	for _, row := range lines(m.render()) {
		if !strings.Contains(row, "Autoscroll:") {
			continue
		}

		// Inside the box, as much room on the left of the toggles as on the
		// right: they are read as one line under the title, which is
		// centered too.
		inside := strings.TrimSuffix(strings.TrimSpace(row), "│")
		inside = strings.TrimPrefix(inside, "│")

		left := len(inside) - len(strings.TrimLeft(inside, " "))
		right := len(inside) - len(strings.TrimRight(inside, " "))
		r.InDelta(left, right, 1, "%q", inside)

		return
	}

	t.Fatal("no line of toggles")
}

func TestLogs_ASwitchOfTheSessionLeavesTheLogAsItIs(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), datacenters: []string{"dc1", "dc2"}}
	stream := &nomad.LogStream{Lines: make(chan string)}
	m := openLog(t, client, stream)
	m.namespaceOrder = []string{"production", "staging"}
	m, _ = m.update(datacentersMsg{names: client.datacenters})
	m, _ = m.update(key('w'))
	r.True(m.text.wrap)

	// The log belongs to its allocation: a namespace or a datacenter for
	// the lists changes nothing about it, and it is not read again.
	// Played out with a deadline per command: a log opened again waits on
	// its stream for good.
	m, cmd := m.update(key('2'))
	m = playOut(m, cmd)
	m, cmd = runLine(m, "dc dc2")
	m = playOut(m, cmd)

	r.Equal("staging", m.namespace)
	r.Equal("dc2", m.datacenter)
	r.True(m.text.wrap)
	r.False(client.logsClosed, "the log was closed to be read again")
}
