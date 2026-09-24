package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// Messages of a log stream. Each carries the stream it is about: a screen
// that switched or was left lets its stream go, and what that stream still
// says must not reach the one that is open.
type (
	// logStreamMsg also carries the log it was opened for: one that arrives
	// after the screen moved on belongs to no screen.
	logStreamMsg struct {
		stream *nomad.LogStream

		allocID string
		task    string
		source  string
	}

	logLineMsg struct {
		stream *nomad.LogStream
		text   string
	}

	logEndMsg struct{ stream *nomad.LogStream }
)

// logState is the output of a task that the log screen reads.
type logState struct {
	// stream is the task output the log screen follows.
	stream    *nomad.LogStream
	following bool

	// finished says the task has stopped: the stream holds all it wrote,
	// and there is nothing to follow.
	finished bool

	// previous is the allocation this one replaced, empty for the first.
	previous string
}

// openLogs follows what the task under the cursor writes.
func openLogs(m Model, source string) (Model, tea.Cmd) {
	task, ok := selectedOf(m, screenTasks, m.tasks())
	if !ok {
		return m, nil
	}

	next := m.screen
	next.kind = screenLogs
	next.task = task.Name
	next.source = source

	m = m.stackText(next, "")
	m.logs = logState{following: true}

	return m, m.startLogs()
}

// startLogs opens the stream. The lines arrive as messages, one command at a
// time, so nothing writes to the model from a goroutine.
func (m Model) startLogs() tea.Cmd {
	client, screen := m.client, m.screen

	return func() tea.Msg {
		stream, err := client.Logs(context.Background(), screen.namespace, screen.allocID, screen.task, screen.source)
		if err != nil {
			return errMsg{err: err}
		}

		return logStreamMsg{stream: stream, allocID: screen.allocID, task: screen.task, source: screen.source}
	}
}

// openedLog keeps a stream that was opened for the log on the screen. One
// that arrives after the screen moved on, or next to a stream the screen
// already reads, is let go of: nothing else would ever close it.
func (m Model) openedLog(msg logStreamMsg) (Model, tea.Cmd) {
	s := m.screen

	ours := s.kind == screenLogs && s.allocID == msg.allocID && s.task == msg.task && s.source == msg.source
	if !ours || m.logs.stream != nil {
		msg.stream.Close()

		return m, nil
	}

	var cmd tea.Cmd
	m.logs, cmd = m.logs.opened(msg.stream)

	return m, cmd
}

// fromOpenStream says a message came from the stream the screen reads.
func (m Model) fromOpenStream(stream *nomad.LogStream) bool {
	return stream == m.logs.stream
}

// waitForLog waits for the next thing the task writes.
func (l logState) waitForLog() tea.Cmd {
	stream := l.stream
	if stream == nil {
		return nil
	}

	return func() tea.Msg {
		select {
		case line, ok := <-stream.Lines:
			if !ok {
				return logEndMsg{stream: stream}
			}

			return logLineMsg{stream: stream, text: line}

		case err := <-stream.Err:
			if err != nil {
				return errMsg{err: err}
			}

			return logEndMsg{stream: stream}
		}
	}
}

// opened keeps the stream that was opened and waits for its first line.
func (l logState) opened(stream *nomad.LogStream) (logState, tea.Cmd) {
	l.stream = stream
	l.finished = stream.Finished
	l.previous = stream.Previous

	return l, l.waitForLog()
}

// appendLog puts what arrived at the end and follows it, unless following was
// stopped. When each line arrived is written down: a task writes no time of
// its own, and the time urga read it is the only one there is.
func (m Model) appendLog(chunk string) (Model, tea.Cmd) {
	if m.screen.kind != screenLogs {
		return m, nil
	}

	if m.text.stamps == nil {
		m.text.stamps = map[int]time.Time{}
	}

	now := time.Now()

	for _, line := range strings.Split(strings.TrimSuffix(chunk, "\n"), "\n") {
		m.text.stamps[len(m.text.lines)] = now
		m.text.lines = append(m.text.lines, line)
	}

	if m.logs.following {
		m.text.toEnd()
	}

	return m, m.logs.waitForLog()
}

// ended lets go of a stream that has ended on its own.
func (l logState) ended() logState {
	l.stream = nil

	return l
}

// stop closes the stream, which stops the request behind it.
func (l *logState) stop() {
	if l.stream != nil {
		l.stream.Close()
		l.stream = nil
	}
}

// logsTitle says whose output this is, in which allocation, which of the two
// it is, and whether the task has stopped writing it.
func logsTitle(s screen, finished bool) string {
	source := s.source
	if finished {
		source += ", task finished"
	}

	return fmt.Sprintf("Logs (Task: %s, Allocation: %s) [%s]", s.task, shortID(s.allocID), source)
}

// logBindings are the keys of the log screen: the ones of any text, and
// the ones of a stream that is still being followed.
var logBindings = []binding{
	{press: "s", label: "Stop following", do: stopFollowing, offered: following},
	{press: "r", label: "Resume", do: resumeFollowing, offered: notFollowing},
	// The key that opens stderr from the tasks switches to the other of
	// the two here, and says which one it goes to.
	{press: "ctrl+e", label: "Stderr", do: switchSource, offered: onSource(nomad.LogStdout)},
	{press: "ctrl+e", label: "Stdout", do: switchSource, offered: onSource(nomad.LogStderr)},
	{press: "p", label: "Allocation before", do: openPrevious, offered: hasPrevious},
	{press: "w", label: "Wrap lines", do: wrapLines},
	{press: "t", label: "When urga read it", do: showTimes},
	{press: "ctrl+s", label: "Save", do: saveScreen},
}

var taskBindings = []binding{
	{press: "enter", label: "Logs", do: openStdout},
	// The same key opens the events of a task here and edits elsewhere,
	// because a screen never offers both.
	{press: "e", label: "Events", do: openTaskEvents},
	{press: "ctrl+e", label: "Logs (stderr)", do: openStderr},
	// The same key opens a shell here and scales a task group elsewhere.
	{press: "s", label: "Shell", do: shell, writes: true},
}

// onSource says the log screen reads the source.
func onSource(source string) func(m Model) bool {
	return func(m Model) bool { return m.screen.source == source }
}

// switchSource reads the other of the two a task writes to, on the same
// screen and the same way: escape still goes back to the tasks.
func switchSource(m Model) (Model, tea.Cmd) {
	m.logs.stop()
	m.logs = logState{following: true}

	m.screen.source = otherSource(m.screen.source)
	m.text = m.text.emptied()
	m.layout()

	return m, m.startLogs()
}

// hasPrevious says the allocation of the log replaced another one.
func hasPrevious(m Model) bool { return m.logs.previous != "" }

// openPrevious reads the same task in the allocation this one replaced, on
// top of this log: escape comes back to it. A task that crashed and was
// placed again starts an empty log, and why it crashed is in the one before.
func openPrevious(m Model) (Model, tea.Cmd) {
	next := m.screen
	next.allocID = m.logs.previous

	m.logs.stop()
	m = m.stackText(next, "")
	m.logs = logState{following: true}

	return m, m.startLogs()
}

// readAgain opens the stream of a log screen that is come back to. What the
// text held was the log of the screen that was on top of it.
func (m Model) readAgain() (Model, tea.Cmd) {
	m.text = m.text.emptied()
	m.logs = logState{following: true}

	return m, m.startLogs()
}

// otherSource is the other of the two a task writes to.
func otherSource(source string) string {
	if source == nomad.LogStdout {
		return nomad.LogStderr
	}

	return nomad.LogStdout
}

// following and notFollowing say which of stop and resume would do
// something: only one of them ever does, and neither for a task that has
// stopped.
func following(m Model) bool { return m.logs.following && !m.logs.finished }

func notFollowing(m Model) bool { return !m.logs.following && !m.logs.finished }

func stopFollowing(m Model) (Model, tea.Cmd) {
	m.logs.following = false

	return m, nil
}

func resumeFollowing(m Model) (Model, tea.Cmd) {
	m.logs.following = true
	m.text.toEnd()

	return m, nil
}

// showTimes puts when each line was read in front of it. Only a log has
// times to show: they are when urga read a line, and nothing else on a text
// screen has any.
func showTimes(m Model) (Model, tea.Cmd) {
	m.text.times = !m.text.times

	return m, nil
}

// wrapLines folds the long lines of anything that reads as text: the logs of
// a task, a description, a job file. Again lets them run on.
func wrapLines(m Model) (Model, tea.Cmd) {
	m.text.wrap = !m.text.wrap
	m.text.follow()

	return m, nil
}

// saveScreen writes what is on a text screen to a file.
func saveScreen(m Model) (Model, tea.Cmd) {
	return m, m.saveText()
}

// openStdout follows what the task under the cursor writes to stdout.
func openStdout(m Model) (Model, tea.Cmd) {
	return openLogs(m, nomad.LogStdout)
}

// openStderr follows what the task under the cursor writes to stderr.
func openStderr(m Model) (Model, tea.Cmd) {
	return openLogs(m, nomad.LogStderr)
}
