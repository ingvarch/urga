package ui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

const (
	// settle is how long the screen waits for a burst of changes to end
	// before it asks the cluster. A deploy fires dozens of events, and one
	// answer is enough for all of them.
	settle = 300 * time.Millisecond

	// slowPoll is how often a watched screen polls anyway. The stream
	// reports changes; this poll only catches one the stream missed.
	slowPoll = 30 * time.Second
)

// Messages of the stream. Each carries the id of its stream: a screen that
// was left closes its stream, and messages still in flight from it must
// not change the screen that is open.
type (
	// watchingMsg means the cluster opened the event stream.
	watchingMsg struct {
		id      int
		changes *nomad.Changes
	}

	// watchEndedMsg means the stream stopped. A stream urga closed itself
	// carries no error; one the cluster refused or dropped carries the error,
	// because the screen is no longer live and nothing else would report it.
	watchEndedMsg struct {
		id  int
		err error
	}

	// changeMsg reports that something the screen shows changed in the
	// cluster. The screen reads the new state with its usual request, so
	// what exactly changed is not carried here.
	changeMsg struct{ id int }

	// settleMsg is the end of a burst of changes.
	settleMsg struct{}
)

// watchState is the stream the open screen watches, and what is known
// from the ones before it.
type watchState struct {
	// changes reports when what the screen shows changed; settling is true
	// while a burst of them waits for the request.
	changes  *nomad.Changes
	settling bool

	// id counts the streams this session has opened, so that a message from
	// a closed one does not change the open one.
	id int

	// refused says the cluster has already refused the stream and the warning
	// was shown: every screen asks again, and every screen is refused.
	refused bool
}

// watchScreen opens an event stream for what the screen shows. A screen
// that watches nothing, or a cluster that does not stream, is polled the
// way it always was.
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

// waitForChange waits for the next message on the stream.
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
				// A stream that dropped sends its error before it
				// closes, and both are then ready at once: reading the
				// close first must not lose the error.
				return watchEndedMsg{id: id, err: reasonOf(changes)}
			}

			return changeMsg{id: id}

		case err := <-changes.Err:
			return watchEndedMsg{id: id, err: err}
		}
	}
}

// reasonOf is the error a stream ended with, if it sent one.
func reasonOf(changes *nomad.Changes) error {
	select {
	case err := <-changes.Err:
		return err
	default:
		return nil
	}
}

// start keeps the stream and starts reading it. A stream that opens after
// the screen that asked for it is gone is closed instead. Once a stream
// opens, a later refusal is worth a warning again.
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

// keepChange notes that something changed and waits for the burst to end
// before it asks the cluster: the stream only signals a change, and one
// request reads the new state.
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

// settled asks the cluster for the rows once a burst of changes is over.
func (m Model) settled() (Model, tea.Cmd) {
	m.watch.settling = false

	return m, m.fetch()
}

// watchEnded drops the stream that stopped.
func (m Model) watchEnded(msg watchEndedMsg) Model {
	// A cluster that does not stream is polled, as it was before. No
	// error is shown over the rows, and the poll timer it already has
	// keeps running.
	if !m.watch.current(msg.id) {
		return m
	}

	m.watch = m.watch.end()

	return m.noteWatchEnded(msg.err)
}

// noteWatchEnded warns that the screen is no longer live, when urga did not
// stop the stream itself. The rows are still there and still polled, so it
// shows as a warning.
//
// It is shown once. Moving between screens of a cluster that does not
// stream asks for a stream on every screen and is refused every time; a
// warning each time would leave the status line with nothing else on it.
func (m Model) noteWatchEnded(err error) Model {
	if err == nil || m.watch.refused {
		return m
	}

	m.watch.refused = true

	return m.warn(fmt.Sprintf("%s: not following the cluster, asking every %s instead",
		err, m.opts.PollEvery))
}

// end closes the stream. What it was watching is polled again, and a
// cluster that does not stream is not an error of the screen. The id goes
// up so that any message still in flight for that stream is known to be
// stale.
func (w watchState) end() watchState {
	w.stop()
	w.id++

	return w
}

// current says the message comes from the stream the model has now.
func (w watchState) current(id int) bool {
	return id == w.id
}

// live says the screen has an open event stream.
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
// the event stream is open, often when it is not.
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

// keepChange handles a change the stream reported.
func (m Model) keepChange(msg changeMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.watch, cmd = m.watch.keepChange(msg)

	return m, cmd
}
