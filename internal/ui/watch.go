package ui

import (
	"context"
	"fmt"
	"maps"
	"math"
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

	// refusedWithin is how soon a stream the cluster refuses ends: Nomad 2.0
	// answers 200 and ends the stream at once, with no event.
	refusedWithin = 5 * time.Second
)

// streamStart is when a stream opened, and whether it sent an event since.
type streamStart struct {
	opened time.Time
	heard  bool
}

// endedAtOnce says a stream that ends at now was refused: it sent nothing
// and ends within refusedWithin of opening.
func (s streamStart) endedAtOnce(now time.Time) bool {
	return !s.heard && now.Sub(s.opened) < refusedWithin
}

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
	// cluster. The screen reads the new state with its usual request; the
	// change only says whether the screen read it already.
	changeMsg struct {
		id     int
		change nomad.Change
	}

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

	// fewer says the screen follows its fewer topics: the cluster refused the
	// stream of all of them.
	fewer bool

	// read is the read index of what the screen shows, by topic.
	read map[string]uint64

	// newest is the newest index the screen waits to read, by topic: of a
	// change of the burst, or of the page before an older answer replaced it.
	// A change of a topic the page shows nothing of is kept too, for an
	// answer that shows that topic from an index below it.
	newest map[string]uint64

	// due says the screen wants its stream and has had no answer yet: the
	// stream opens after the first answer, from what that answer read.
	due bool

	// streamStart is when the open stream opened and whether it sent an event.
	streamStart
}

// keepRead notes the read index of an answer. The newest answer replaces the
// old index because it is what the page shows. An answer read before the one
// the page showed leaves out what changed in between, which the stream does
// not send again: the screen reads again once the burst settles. A page that
// showed nothing of a topic left nothing of it out, but a change of the topic
// that came since may be newer than the answer.
func (w watchState) keepRead(read map[string]uint64) (watchState, tea.Cmd) {
	if len(read) == 0 {
		return w, nil
	}

	shown := w.read

	w.read = maps.Clone(w.read)
	if w.read == nil {
		w.read = map[string]uint64{}
	}

	maps.Copy(w.read, read)

	var settles []tea.Cmd

	for topic, index := range read {
		was := shown[topic]
		if was == nomad.EveryIndex {
			was = w.newest[topic]
		}

		if index < was {
			var cmd tea.Cmd
			w, cmd = w.await(topic, was)
			settles = append(settles, cmd)
		}
	}

	return w, tea.Batch(settles...)
}

// entered is the state of a screen just opened: no stream, nothing read.
func (w watchState) entered() watchState {
	w = w.end()
	w.read, w.newest, w.settling, w.due = nil, nil, false, true

	return w
}

// from is where the stream of the topics starts: after the lowest index the
// screen read them at, or 0, the oldest event the cluster keeps, when a topic
// was not read at an index.
func (w watchState) from(topics []string) uint64 {
	lowest := uint64(math.MaxUint64)

	for _, topic := range topics {
		index, ok := w.read[topic]
		if !ok {
			return 0
		}

		lowest = min(lowest, index)
	}

	if lowest == math.MaxUint64 {
		return 0
	}

	return lowest + 1
}

// watchScreen opens an event stream for what the screen shows. A screen
// that watches nothing, or a cluster that does not stream, is polled the
// way it always was. A screen whose topics were all refused may follow fewer.
func (m Model) watchScreen() tea.Cmd {
	topics := m.screen.page.topics()
	if f, ok := m.screen.page.(fallback); ok && m.watch.fewer {
		topics = f.fewerTopics()
	}
	if len(topics) == 0 || !nomad.Streams(m.nomadVersion, topics) {
		return nil
	}

	client, namespace, id, from := m.client, namespaceOf(m.screen.page, m.env()), m.watch.id, m.watch.from(topics)

	return func() tea.Msg {
		changes, err := client.Events(context.Background(), namespace, topics, from)
		if err != nil {
			return watchEndedMsg{id: id, err: err}
		}

		return watchingMsg{id: id, changes: changes}
	}
}

// watchOnVersion opens the stream of a screen that has answered, once the
// version its topics need arrives. Before the first answer, the answer opens
// it; a screen whose topics need no version opened it on its answer.
func (m Model) watchOnVersion() tea.Cmd {
	topics := m.screen.page.topics()
	if m.watch.due || m.watch.live() || nomad.Streams("", topics) {
		return nil
	}

	return m.watchScreen()
}

// watchOnAnswer opens the stream of a screen on its first answer, from what
// the answer read.
func (m Model) watchOnAnswer() (Model, tea.Cmd) {
	if !m.watch.due {
		return m, nil
	}

	m.watch.due = false

	return m, m.watchScreen()
}

// waitForChange waits for the next message on the stream.
func (w watchState) waitForChange() tea.Cmd {
	changes := w.changes
	if changes == nil {
		return nil
	}

	id := w.id

	return waitOn(changes.C, changes.Err, func(change nomad.Change, ok bool) tea.Msg {
		if !ok {
			// A stream that dropped sends its error before it
			// closes, and both are then ready at once: reading the
			// close first must not lose the error.
			return watchEndedMsg{id: id, err: reasonOf(changes)}
		}

		return changeMsg{id: id, change: change}
	}, func(err error) tea.Msg {
		return watchEndedMsg{id: id, err: err}
	})
}

// reasonOf is the error a stream ended with, if it sent one.
func reasonOf[T any](stream *nomad.Stream[T]) error {
	select {
	case err := <-stream.Err:
		return err
	default:
		return nil
	}
}

// start keeps the stream and starts reading it. A stream that opens after
// the screen that asked for it is gone is closed instead. Once a stream
// opens, a later refusal is worth a warning again.
func (w watchState) start(msg watchingMsg, now time.Time) (watchState, tea.Cmd) {
	if msg.id != w.id {
		msg.changes.Close()

		return w, nil
	}

	w.stop()

	w.changes = msg.changes
	w.refused = false
	w.streamStart = streamStart{opened: now}

	return w, w.waitForChange()
}

// keepChange notes that something changed and waits for the burst to end
// before it asks the cluster: the stream only signals a change, and one
// request reads the new state. A change the screen read already only waits
// for the next one; a change of a topic the page shows nothing of is kept
// for the answer that shows it.
func (w watchState) keepChange(msg changeMsg) (watchState, tea.Cmd) {
	if !w.current(msg.id) {
		return w, nil
	}

	w.heard = true
	next := w.waitForChange()

	topic, index := msg.change.Topic, msg.change.Index
	if read, ok := w.read[topic]; ok && index <= read {
		if read == nomad.EveryIndex {
			w = w.keep(topic, index)
		}

		return w, next
	}

	w, settles := w.await(topic, index)

	return w, tea.Batch(next, settles)
}

// await keeps a change the screen has yet to read, and waits for the burst
// to end unless it waits already.
func (w watchState) await(topic string, index uint64) (watchState, tea.Cmd) {
	w = w.keep(topic, index)

	if w.settling {
		return w, nil
	}

	w.settling = true

	return w, startTimer(settle, settleMsg{})
}

// keep notes the newest change of a topic.
func (w watchState) keep(topic string, index uint64) watchState {
	w.newest = maps.Clone(w.newest)
	if w.newest == nil {
		w.newest = map[string]uint64{}
	}

	w.newest[topic] = max(w.newest[topic], index)

	return w
}

// stale says a change of the burst is newer than what the screen read, or
// is of a topic it read at no index.
func (w watchState) stale() bool {
	for topic, index := range w.newest {
		if read, ok := w.read[topic]; !ok || index > read {
			return true
		}
	}

	return false
}

// settled asks the cluster for the rows once a burst of changes is over,
// unless the screen read them since.
func (m Model) settled() (Model, tea.Cmd) {
	m.watch.settling = false

	stale := m.watch.stale()
	m.watch.newest = nil

	if !stale {
		return m, nil
	}

	return m, m.fetch()
}

// watchEnded drops the stream that stopped.
func (m Model) watchEnded(msg watchEndedMsg) (Model, tea.Cmd) {
	// A cluster that does not stream is polled, as it was before. No
	// error is shown over the rows.
	if !m.watch.current(msg.id) {
		return m, nil
	}

	refused := m.refusedAll()

	m.watch = m.watch.end()

	// A screen that can follow fewer topics does, with no warning. Its poll
	// timer is not sped up: the stream of the fewer topics resets it at the
	// slow pace once it opens.
	if refused {
		m.watch.fewer = true

		return m, m.watchScreen()
	}

	// The timer set while the stream was up is slow: the screen polls at
	// its own pace from now on.
	return m.noteWatchEnded(msg.err).restartPoll().schedulePoll()
}

// refusedAll says the cluster refused the stream of all the topics of a
// screen that can follow fewer: it did not open it, or ended it at once.
func (m Model) refusedAll() bool {
	_, ok := m.screen.page.(fallback)

	return ok && !m.watch.fewer && (!m.watch.live() || m.watch.endedAtOnce(m.now))
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
	w.fewer = false

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
	wasLive := m.watch.live()

	var cmd tea.Cmd
	m.watch, cmd = m.watch.start(msg, m.now)

	if wasLive || !m.watch.live() || !m.polling {
		return m, cmd
	}

	// The timer set before the stream opened is the fast one.
	m, poll := m.restartPoll().schedulePoll()

	return m, tea.Batch(cmd, poll)
}

// keepChange handles a change the stream reported.
func (m Model) keepChange(msg changeMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.watch, cmd = m.watch.keepChange(msg)

	return m, cmd
}
