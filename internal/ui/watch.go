package ui

import (
	"context"
	"fmt"
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

// Messages of the stream. Each carries the stream it came from: a screen
// that was left closes its stream, and the answers that were in flight when
// it did must not touch the one that is up.
type (
	// watchingMsg is the cluster agreeing to say when things change.
	watchingMsg struct {
		id      int
		changes *nomad.Changes
	}

	// watchEndedMsg is the stream stopping. A stream urga closed itself
	// carries no reason; one the cluster refused or dropped carries why,
	// because the screen is no longer live and nothing else would say so.
	watchEndedMsg struct {
		id  int
		err error
	}

	// changeMsg is the cluster saying that the screen is no longer what it
	// shows. What it now holds is read the usual way, so what exactly
	// changed is not carried here.
	changeMsg struct{ id int }

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

	client, namespace, id := m.client, m.screen.namespace, m.watchID

	return func() tea.Msg {
		changes, err := client.Events(context.Background(), namespace, topics)
		if err != nil {
			return watchEndedMsg{id: id, err: err}
		}

		return watchingMsg{id: id, changes: changes}
	}
}

// waitForChange takes the next thing the cluster says.
func (m Model) waitForChange() tea.Cmd {
	changes := m.changes
	if changes == nil {
		return nil
	}

	id := m.watchID

	return func() tea.Msg {
		select {
		case _, ok := <-changes.C:
			if !ok {
				return watchEndedMsg{id: id}
			}

			return changeMsg{id: id}

		case err := <-changes.Err:
			return watchEndedMsg{id: id, err: err}
		}
	}
}

// startWatching keeps the stream and starts reading it. A stream that comes
// up after the screen that asked for it is gone is closed instead.
func (m Model) startWatching(msg watchingMsg) (Model, tea.Cmd) {
	if msg.id != m.watchID {
		msg.changes.Close()

		return m, nil
	}

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

// noteWatchEnded says the screen is no longer live, when it is not urga that
// stopped it. The rows are still there and still asked for, so this is worth
// knowing rather than something gone wrong.
func (m Model) noteWatchEnded(err error) Model {
	if err == nil {
		return m
	}

	return m.warn(fmt.Sprintf("%s: not following the cluster, asking every %s instead",
		err, m.opts.PollEvery))
}

// endWatch lets the stream go. What it was watching is polled again, and a
// cluster that will not stream is not an error of the screen. The count goes
// up so that whatever was in flight for that stream is known to be stale.
func (m Model) endWatch() Model {
	m.stopWatching()
	m.watching = false
	m.watchID++

	return m
}

// current says the answer belongs to the stream the model is holding.
func (m Model) current(id int) bool {
	return id == m.watchID
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
