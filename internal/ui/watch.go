package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

const (
	// settle is how long a burst of changes is let run before the screen
	// asks the cluster. A deploy fires events by the dozen, and one answer
	// is enough for all of them.
	settle = 300 * time.Millisecond

	// slowPoll is how often a watched screen asks on its own. The stream
	// says when something changed; this is only there in case it does not.
	slowPoll = 30 * time.Second
)

// Messages of the stream.
type (
	// watchingMsg is the cluster agreeing to say when things change.
	watchingMsg struct{ changes *nomad.Changes }

	// watchEndedMsg is the stream stopping, however it stopped.
	watchEndedMsg struct{ err error }

	// changeMsg is one thing that changed.
	changeMsg struct{ change nomad.Change }

	// settleMsg is the end of a burst of changes.
	settleMsg struct{}
)

// watch asks the cluster to say when what the screen shows changes. A screen
// that watches nothing, or a cluster that will not stream, is polled the way
// it always was.
func (m Model) watch() tea.Cmd {
	topics := m.screen.of().topics
	if len(topics) == 0 {
		return nil
	}

	client, namespace := m.client, m.screen.namespace

	return func() tea.Msg {
		changes, err := client.Events(context.Background(), namespace, topics)
		if err != nil {
			return watchEndedMsg{err: err}
		}

		return watchingMsg{changes: changes}
	}
}

// waitForChange takes the next thing the cluster says.
func (m Model) waitForChange() tea.Cmd {
	changes := m.changes
	if changes == nil {
		return nil
	}

	return func() tea.Msg {
		select {
		case change, ok := <-changes.C:
			if !ok {
				return watchEndedMsg{}
			}

			return changeMsg{change: change}

		case err := <-changes.Err:
			return watchEndedMsg{err: err}
		}
	}
}

// startWatching keeps the stream and starts reading it.
func (m Model) startWatching(msg watchingMsg) (Model, tea.Cmd) {
	m.stopWatching()

	m.changes = msg.changes
	m.watching = true

	return m, m.waitForChange()
}

// keepChange writes down that something changed and lets the burst settle
// before asking the cluster: the stream says when, one request says what.
func (m Model) keepChange() (Model, tea.Cmd) {
	next := m.waitForChange()

	if m.settling {
		return m, next
	}

	m.settling = true

	return m, tea.Batch(next, tea.Tick(settle, func(time.Time) tea.Msg { return settleMsg{} }))
}

// endWatch lets the stream go. What it was watching is polled again, and a
// cluster that will not stream is not an error of the screen.
func (m Model) endWatch() Model {
	m.stopWatching()
	m.watching = false

	return m
}

// stopWatching closes the stream, which stops the request behind it.
func (m *Model) stopWatching() {
	if m.changes != nil {
		m.changes.Close()
		m.changes = nil
	}
}

// pollEvery is how long the screen waits before asking again: rarely while
// the cluster says when things change, often when it does not.
func (m Model) pollEvery() time.Duration {
	if m.watching {
		return slowPoll
	}

	return m.opts.PollEvery
}
