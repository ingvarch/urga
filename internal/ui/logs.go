package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// Messages of a log stream.
type (
	logStreamMsg struct{ stream *nomad.LogStream }
	logLineMsg   string
	logEndMsg    struct{}
)

// openLogs follows what the task under the cursor writes.
func (m Model) openLogs(source string) (Model, tea.Cmd) {
	task, ok := selectedOf(m, screenTasks, m.tasks())
	if !ok {
		return m, nil
	}

	next := m.screen
	next.kind = screenLogs
	next.task = task.Name
	next.source = source

	m = m.stackText(next, "")
	m.following = true

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

		return logStreamMsg{stream: stream}
	}
}

// waitForLog waits for the next thing the task writes.
func (m Model) waitForLog() tea.Cmd {
	stream := m.stream
	if stream == nil {
		return nil
	}

	return func() tea.Msg {
		select {
		case line, ok := <-stream.Lines:
			if !ok {
				return logEndMsg{}
			}

			return logLineMsg(line)

		case err := <-stream.Err:
			if err != nil {
				return errMsg{err: err}
			}

			return logEndMsg{}
		}
	}
}

// appendLog puts what arrived at the end and follows it, unless following was
// stopped. When each line arrived is written down: a task writes no time of
// its own, and the time urga read it is the only one there is.
func (m Model) appendLog(chunk string) (Model, tea.Cmd) {
	if m.text.stamps == nil {
		m.text.stamps = map[int]time.Time{}
	}

	now := time.Now()

	for _, line := range strings.Split(strings.TrimSuffix(chunk, "\n"), "\n") {
		m.text.stamps[len(m.text.lines)] = now
		m.text.lines = append(m.text.lines, line)
	}

	if m.following {
		m.text.move(len(m.text.lines))
	}

	return m, m.waitForLog()
}

// closeLogs stops the stream, which stops the request behind it.
func (m *Model) closeLogs() {
	if m.stream != nil {
		m.stream.Close()
		m.stream = nil
	}
}

// logsTitle says whose output this is and which of the two it is.
func logsTitle(s screen) string {
	return fmt.Sprintf("Logs (Task: %s) [%s]", s.task, s.source)
}

// logsKey answers the keys of the log screen.
func (m Model) logsKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	switch msg.String() {
	case "s":
		m.following = false
	case "r":
		m.following = true
		m.text.move(len(m.text.lines))
	case "t":
		// Only a log has times to show: they are when urga read a line,
		// and nothing else on a text screen has any.
		m.text.times = !m.text.times
	default:
		return m, nil, false
	}

	return m, nil, true
}

// textKey answers the keys of anything that reads as text: the logs of a
// task, a description, a job file.
func (m Model) textKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	switch msg.String() {
	case "w":
		m.text.wrap = !m.text.wrap
		m.text.follow()

	case "ctrl+s":
		return m, m.saveText(), true

	default:
		return m, nil, false
	}

	return m, nil, true
}

var logHints = []hint{
	{Key: "<s>", Description: "Stop following"},
	{Key: "<r>", Description: "Resume"},
	{Key: "<w>", Description: "Wrap lines"},
	{Key: "<t>", Description: "When urga read it"},
	{Key: "<ctrl-s>", Description: "Save"},
}

var taskHints = []hint{
	{Key: "<enter>", Description: "Logs"},
	{Key: "<e>", Description: "Events"},
	{Key: "<ctrl-e>", Description: "Logs (stderr)"},
	{Key: "<s>", Description: "Shell"},
}
