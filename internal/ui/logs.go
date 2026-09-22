package ui

import (
	"context"
	"fmt"
	"strings"

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
	row, ok := m.selectedIndex()
	if !ok || m.screen.kind != screenTasks {
		return m, nil
	}

	tasks := m.tasks()
	if row >= len(tasks) {
		return m, nil
	}

	next := m.screen
	next.kind = screenLogs
	next.task = tasks[row].Name
	next.source = source

	m.history = append(m.history, m.screen)
	m.screen = next
	m.err = nil
	m.following = true
	m.text = newTextModel("")
	m.layout()

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
// stopped.
func (m Model) appendLog(chunk string) (Model, tea.Cmd) {
	m.text.lines = append(m.text.lines, strings.Split(strings.TrimSuffix(chunk, "\n"), "\n")...)

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
	default:
		return m, nil, false
	}

	return m, nil, true
}

var logHints = []hint{
	{Key: "<s>", Description: "Stop following"},
	{Key: "<r>", Description: "Resume"},
}

var taskHints = []hint{
	{Key: "<enter>", Description: "Logs"},
	{Key: "<ctrl-e>", Description: "Logs (stderr)"},
}
