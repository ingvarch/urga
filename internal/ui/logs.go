package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ingvarch/urga/internal/nomad"
)

// Messages of a log stream. Each carries the stream it is about: a screen
// that switched or was left lets its stream go, and what that stream still
// says must not reach the one that is open.
type (
	// logStreamMsg also carries the source it was opened for: the log may
	// have switched to the other one while it was on its way.
	logStreamMsg struct {
		stream *nomad.LogStream
		source string
	}

	logLineMsg struct {
		stream *nomad.LogStream
		text   string
	}

	logEndMsg struct{ stream *nomad.LogStream }
)

// logState is what a stream is read through: the stream, what it said so
// far, and what it said of itself when it was opened.
type logState struct {
	stream  *nomad.LogStream
	content textContent

	// finished says the task has stopped: the stream holds all it wrote,
	// and there is nothing to follow.
	finished bool

	// previous is the allocation this one replaced, empty for the first.
	previous string

	// size is how big a file was when it was opened, and from where in it
	// reading began.
	size int64
	from int64
}

// logsPage is what one task of an allocation writes to stdout or stderr,
// followed as it is written.
type logsPage struct {
	namespace, allocID, task, source string

	read logState
}

// logsScreen opens a log where its allocation lives.
func logsScreen(p logsPage) screen {
	return screen{kind: screenLogs, page: p}
}

// followLogs follows what the task under the cursor writes to a source.
func followLogs(p tasksPage, e env, source string) (tasksPage, outcome) {
	task, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg(logsScreen(logsPage{namespace: p.namespace, allocID: p.allocID, task: task.Name, source: source})))
}

// title says whose output this is, in which allocation, which of the two
// it is, and whether the task has stopped writing it.
func (p logsPage) title(env, int) string {
	source := p.source
	if p.read.finished {
		source += ", task finished"
	}

	return fmt.Sprintf("Logs (Task: %s, Allocation: %s) [%s]", p.task, shortID(p.allocID), source)
}

func (logsPage) titles() []string           { return nil }
func (logsPage) topics() []string           { return nil }
func (logsPage) fetch(env) tea.Cmd          { return nil }
func (logsPage) rows(env) []tableRow        { return nil }
func (logsPage) follows() bool              { return true }
func (p logsPage) text(env) textContent     { return p.read.content }
func (p logsPage) saveAs() (string, string) { return fmt.Sprintf("%s-%s", p.task, p.source), "log" }

// open reads the log from the start of what the cluster still holds of it:
// what the page read before is in there again.
func (p logsPage) open(e env) (page, tea.Cmd) {
	p.read.stop()
	p.read = logState{}

	return p, p.start(e.client)
}

func (p logsPage) close() page {
	p.read.stop()

	return p
}

// start opens the stream. The lines arrive as messages, one command at a
// time, so nothing writes to the model from a goroutine.
func (p logsPage) start(client filesClient) tea.Cmd {
	namespace, allocID, task, source := p.namespace, p.allocID, p.task, p.source

	return func() tea.Msg {
		stream, err := client.Logs(context.Background(), namespace, allocID, task, source)
		if err != nil {
			return errMsg{err: err}
		}

		return logStreamMsg{stream: stream, source: source}
	}
}

func (p logsPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	var (
		out  outcome
		took bool
	)

	// A stream opened for a log that is no longer up never gets here: it
	// belongs to an ask that is over.
	if opened, ok := msg.(logStreamMsg); ok {
		p.read, out, took = p.read.keep(opened.stream, opened.source == p.source)
	} else {
		p.read, out, took = p.read.take(msg)
	}

	return p, out, took
}

// logsKeys are the keys of a log: the ones of any text, and the ones of a
// stream that is still being followed.
var logsKeys = []pageKey[logsPage]{
	followKey[logsPage](),
	// The key that opens stderr from the tasks switches to the other of
	// the two here, and says which one it goes to.
	{press: "ctrl+e", label: "Stderr", do: switchSource, offered: readsSource(nomad.LogStdout)},
	{press: "ctrl+e", label: "Stdout", do: switchSource, offered: readsSource(nomad.LogStderr)},
	{press: "p", label: "Previous Alloc", do: openPrevious, offered: hasPrevious},
	wrapKey[logsPage](),
	timesKey[logsPage](),
	saveKey[logsPage](),
}

func (p logsPage) keys(e env) []keyHint { return hintsOf(p, e, logsKeys) }

func (p logsPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, logsKeys, k)
}

// readsSource says the log reads the source.
func readsSource(source string) func(p logsPage, _ env) bool {
	return func(p logsPage, _ env) bool { return p.source == source }
}

// switchSource reads the other of the two a task writes to, on the same
// screen and the same way: escape still goes back to the tasks.
func switchSource(p logsPage, _ env) (logsPage, outcome) {
	p.source = otherSource(p.source)

	return p, then(reopenMsg{})
}

// hasPrevious says the allocation of the log replaced another one.
func hasPrevious(p logsPage, _ env) bool { return p.read.previous != "" }

// openPrevious reads the same task in the allocation this one replaced, on
// top of this log: escape comes back to it. A task that crashed and was
// placed again starts an empty log, and why it crashed is in the one before.
func openPrevious(p logsPage, _ env) (logsPage, outcome) {
	previous := logsPage{namespace: p.namespace, allocID: p.read.previous, task: p.task, source: p.source}

	return p, then(openMsg(logsScreen(previous)))
}

// waitForLog waits for the next thing the task writes.
func (l logState) waitForLog() tea.Cmd {
	return waitForStream(l.stream)
}

// waitForStream waits for the next thing a stream says.
func waitForStream(stream *nomad.LogStream) tea.Cmd {
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
	l.size, l.from = stream.Size, stream.From

	return l, l.waitForLog()
}

// keep reads a stream that was opened for the page, when ours says it was.
// One opened for something else, or next to a stream the page already
// reads, is not the page's to keep: the session lets go of it.
func (l logState) keep(stream *nomad.LogStream, ours bool) (logState, outcome, bool) {
	if !ours || l.stream != nil {
		return l, outcome{}, false
	}

	l, cmd := l.opened(stream)

	return l, outcome{cmd: cmd, reading: true}, true
}

// take keeps what the stream says: a line, after which it waits for the
// next, or its end. None of it is an answer to what the page asked.
func (l logState) take(msg tea.Msg) (logState, outcome, bool) {
	switch msg := msg.(type) {
	case logLineMsg:
		if msg.stream != l.stream {
			return l, outcome{}, false
		}

		// When each line arrived is written down: a task writes no time of
		// its own, and the time urga read it is the only one there is.
		l.content.add(linesOf(msg.text), time.Now(), nil, nil)

		return l, outcome{cmd: l.waitForLog(), reading: true}, true

	case logEndMsg:
		if msg.stream != l.stream {
			return l, outcome{}, false
		}

		return l.ended(), outcome{reading: true}, true
	}

	return l, outcome{}, false
}

// linesOf are the lines of a chunk a stream sent: the newline it ends with
// ends its last line and starts none.
func linesOf(chunk string) []string {
	return strings.Split(strings.TrimSuffix(chunk, "\n"), "\n")
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

// otherSource is the other of the two a task writes to.
func otherSource(source string) string {
	if source == nomad.LogStdout {
		return nomad.LogStderr
	}

	return nomad.LogStdout
}

// toggleAutoscroll follows the end of the log, or stops following it where
// it stands. Turned on, it goes to the end at once.
func toggleAutoscroll(m Model) (Model, tea.Cmd) {
	m.text.following = !m.text.following

	if m.text.following {
		m.text.toEnd()
	}

	return m, nil
}

// logToggleGap is the room between two toggles on the line under the title.
const logToggleGap = 6

// logToggles is the line under the title of a log: what each toggle is set
// to, named the way the keys that set it name it.
func (m Model) logToggles(width int) string {
	toggles := []struct {
		press, name string
		on          bool
	}{
		{"s", "Autoscroll", m.text.following},
		{"t", "Timestamps", m.text.times},
		{"w", "Wrap", m.text.wrap},
	}

	cells := make([]string, 0, len(toggles))
	for _, toggle := range toggles {
		// A toggle the screen has no key for is nothing to it.
		if _, ok := m.binding(toggle.press); !ok {
			continue
		}

		state := styleMuted.Render("Off")
		if toggle.on {
			state = styleTitle.Render("On")
		}

		cells = append(cells, styleLabel.Render(toggle.name+":")+state)
	}

	// Centered under the title, which is centered too.
	line := strings.Join(cells, strings.Repeat(" ", logToggleGap))
	left := max((width-ansi.StringWidth(line))/2, 0)

	return truncate(strings.Repeat(" ", left)+line, width)
}

// toggleRows is how many rows of the box the line of toggles takes.
func (m Model) toggleRows() int {
	if m.readsAStream() {
		return 1
	}

	return 0
}

// readsAStream says the screen reads what something writes: the log of a
// task, the logs of a task in every allocation, or a file of an allocation.
func (m Model) readsAStream() bool {
	_, ok := m.screen.page.(streamer)

	return ok
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

// followStdout follows what the task under the cursor writes to stdout.
func followStdout(p tasksPage, e env) (tasksPage, outcome) {
	return followLogs(p, e, nomad.LogStdout)
}

// followStderr follows what the task under the cursor writes to stderr.
func followStderr(p tasksPage, e env) (tasksPage, outcome) {
	return followLogs(p, e, nomad.LogStderr)
}
