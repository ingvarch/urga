package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

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
	m = drain(m, m.logs.waitForLog())

	r.True(m.logs.following)

	// Turned off, autoscroll leaves the window where it is; turned on, it
	// jumps back to the end.
	m, _ = m.update(key('s'))
	r.False(m.logs.following)

	m, _ = m.update(key('s'))
	r.True(m.logs.following)
}

func TestLogs_TheLogStartsRightUnderTheToggles(t *testing.T) {
	r := require.New(t)

	written := make(chan string, 1)
	written <- "first line\n"

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), logs: &nomad.LogStream{Lines: written}}
	m := openTasks(t, client)

	m, cmd := m.update(enter())
	m = drain(m, cmd)
	m = drain(m, m.logs.waitForLog())

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
	r.False(m.logs.following)
	r.Contains(plain(m.render()), "Autoscroll:Off")

	m, _ = m.update(key('s'))
	r.True(m.logs.following)
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
	m = drain(m, m.logs.waitForLog())
	m = drain(m, m.logs.waitForLog())

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
	r.Same(client.logs, m.logs.stream)
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
	if line := within(m.logs.waitForLog(), time.Second); line != nil {
		m, _ = m.update(line)
	}

	r.Contains(plain(m.render()), "why it crashed")

	client.logs = &nomad.LogStream{Lines: make(chan string)}

	m, cmd = m.update(escape())
	m = drain(m, cmd)

	r.NotContains(plain(m.render()), "why it crashed")
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
