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

// watchState is the stream the open screen watches, and what came of the
// ones before it.
type watchState struct {
	// changes is the cluster saying when what the screen shows changed, and
	// settling a burst of them waiting to be asked about.
	changes  *nomad.Changes
	settling bool

	// id counts the streams this session has opened, so that an answer from
	// one that was let go of does not touch the one that is up.
	id int

	// refused says the cluster has already turned the stream down and been
	// said so about: every screen asks again, and every screen is refused.
	refused bool
}

// watchScreen asks the cluster to say when what the screen shows changes. A
// screen that watches nothing, or a cluster that will not stream, is polled
// the way it always was.
func (m Model) watchScreen() tea.Cmd {
	topics := m.screen.page.topics()
	if len(topics) == 0 {
		return nil
	}

	client, namespace, id := m.client, namespaceOf(m.screen.page, m.env()), m.watch.id

	return func() tea.Msg {
		changes, err := client.Events(context.Background(), namespace, topics)
		if err != nil {
			return watchEndedMsg{id: id, err: err}
		}

		return watchingMsg{id: id, changes: changes}
	}
}

// waitForChange takes the next thing the cluster says.
func (w watchState) waitForChange() tea.Cmd {
	changes := w.changes
	if changes == nil {
		return nil
	}

	id := w.id

	return func() tea.Msg {
		select {
		case _, ok := <-changes.C:
			if !ok {
				// A stream that dropped says why before it closes, and
				// both are then ready at once: reading the close first
				// must not lose the reason.
				return watchEndedMsg{id: id, err: reasonOf(changes)}
			}

			return changeMsg{id: id}

		case err := <-changes.Err:
			return watchEndedMsg{id: id, err: err}
		}
	}
}

// reasonOf is why a stream ended, when it said.
func reasonOf(changes *nomad.Changes) error {
	select {
	case err := <-changes.Err:
		return err
	default:
		return nil
	}
}

// start keeps the stream and starts reading it. A stream that comes up after
// the screen that asked for it is gone is closed instead. A cluster that
// talks again is a cluster whose going quiet is news again.
func (w watchState) start(msg watchingMsg) (watchState, tea.Cmd) {
	if msg.id != w.id {
		msg.changes.Close()

		return w, nil
	}

	w.stop()

	w.changes = msg.changes
	w.refused = false

	return w, w.waitForChange()
}

// keepChange writes down that something changed and lets the burst settle
// before asking the cluster: the stream says when, one request says what.
func (w watchState) keepChange(msg changeMsg) (watchState, tea.Cmd) {
	if !w.current(msg.id) {
		return w, nil
	}

	next := w.waitForChange()

	if w.settling {
		return w, next
	}

	w.settling = true

	return w, tea.Batch(next, tea.Tick(settle, func(time.Time) tea.Msg { return settleMsg{} }))
}

// settled asks the cluster what a burst of changes left behind.
func (m Model) settled() (Model, tea.Cmd) {
	m.watch.settling = false

	return m, m.fetch()
}

// watchEnded lets go of the stream that stopped.
func (m Model) watchEnded(msg watchEndedMsg) Model {
	// A cluster that will not stream is one urga asks on its own, which
	// is what it did before. Nothing about that belongs over the rows,
	// and the timer it already has goes on without help.
	if !m.watch.current(msg.id) {
		return m
	}

	m.watch = m.watch.end()

	return m.noteWatchEnded(msg.err)
}

// noteWatchEnded says the screen is no longer live, when it is not urga that
// stopped it. The rows are still there and still asked for, so this is worth
// knowing rather than something gone wrong.
//
// It is said once. Walking around a cluster that will not stream asks it on
// every screen and is refused every time; saying so every time would leave
// the status line with nothing else on it.
func (m Model) noteWatchEnded(err error) Model {
	if err == nil || m.watch.refused {
		return m
	}

	m.watch.refused = true

	return m.warn(fmt.Sprintf("%s: not following the cluster, asking every %s instead",
		err, m.opts.PollEvery))
}

// end lets the stream go. What it was watching is polled again, and a
// cluster that will not stream is not an error of the screen. The count goes
// up so that whatever was in flight for that stream is known to be stale.
func (w watchState) end() watchState {
	w.stop()
	w.id++

	return w
}

// current says the answer belongs to the stream the model is holding.
func (w watchState) current(id int) bool {
	return id == w.id
}

// live says the cluster agreed to say when what the screen shows changes.
func (w watchState) live() bool {
	return w.changes != nil
}

// stop closes the stream, which stops the request behind it.
func (w *watchState) stop() {
	if w.changes != nil {
		w.changes.Close()
		w.changes = nil
	}
}

// pollEvery is how long the screen waits before asking again: rarely while
// the cluster says when things change, often when it does not.
func (m Model) pollEvery() time.Duration {
	if m.watch.live() {
		return slowPoll
	}

	return m.opts.PollEvery
}

// startWatch keeps the stream the cluster opened.
func (m Model) startWatch(msg watchingMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.watch, cmd = m.watch.start(msg)

	return m, cmd
}

// keepChange takes what the cluster said changed.
func (m Model) keepChange(msg changeMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.watch, cmd = m.watch.keepChange(msg)

	return m, cmd
}
