package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// noteKept is how many failures the note remembers.
const noteKept = 1000

// noteAgain is how long the note waits before it follows a stream that
// dropped.
const noteAgain = 30 * time.Second

// noteRefusedWithin is how soon a stream the cluster refuses ends: Nomad 2.0
// answers 200 and ends the stream at once, with no event.
const noteRefusedWithin = 5 * time.Second

// noteAgainMsg asks to follow the failures again, for the connection with
// that id.
type noteAgainMsg struct{ id int }

// noteOpenedMsg is the stream of failures the cluster opened from the index
// from, for the connection with that id. It is not a streamOpened: the model
// closes a stale one itself.
type noteOpenedMsg struct {
	id   int
	feed *nomad.Feed
	from uint64
	err  error
}

// noteState is the note on the status line: one stream of its own, opened
// per connection, that says when an allocation or a deployment fails or a
// client goes down. The note is an extra: a stream the cluster refuses
// leaves no note and no warning.
type noteState struct {
	feed *nomad.Feed

	// id changes with the connection, so an answer of an earlier one is
	// dropped.
	id int

	// failing is what the note knows failed and has not left that state.
	failing map[string]bool

	// heard says the open stream sent an event, and opened is when it
	// opened.
	heard  bool
	opened time.Time

	// from is where the next stream starts: past the newest event read, or
	// where the last stream started. 0 is past the newest event the cluster
	// has.
	from uint64
}

// open asks for the failures of every namespace. The first time it starts at
// the event after the newest one: asked from 0, the cluster replays all it
// keeps, and a failure of hours ago would show as news. After a stream
// dropped it starts where that one stopped.
func (n noteState) open(client eventsClient) tea.Cmd {
	id, from := n.id, n.from

	return func() tea.Msg {
		if from == 0 {
			ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
			index, err := client.LatestIndex(ctx)

			cancel()

			if err != nil {
				return noteOpenedMsg{id: id, err: err}
			}

			from = index + 1
		}

		// The stream lasts as long as it is followed, so it has no deadline.
		feed, err := client.Failures(context.Background(), nomad.AllNamespaces, from)

		return noteOpenedMsg{id: id, feed: feed, from: from, err: err}
	}
}

// restart closes the stream and starts over for the next connection.
func (n noteState) restart() noteState {
	n.stop()

	return noteState{id: n.id + 1}
}

// stop closes the stream, which stops the request behind it.
func (n *noteState) stop() {
	if n.feed != nil {
		n.feed.Close()
		n.feed = nil
	}
}

// take says what to show for an event: nothing for what is not a failure,
// for a failure already known, and for an event that sends a failed object
// again without failing it.
func (n *noteState) take(e nomad.Event) (string, bool) {
	key := e.Topic + "/" + e.Key

	text, failed := failureOf(e)
	if !failed {
		delete(n.failing, key)

		return "", false
	}

	known := n.failing[key]

	if n.failing == nil || len(n.failing) >= noteKept {
		n.failing = map[string]bool{}
	}

	n.failing[key] = true

	if known || !failureTypes[[2]string{e.Topic, e.Type}] {
		return "", false
	}

	return text, true
}

// failureTypes are the topic and the type of the events that report the
// change to a failed state.
var failureTypes = map[[2]string]bool{
	{nomad.TopicAllocation, nomad.TypeAllocationUpdated}:      true,
	{nomad.TopicDeployment, nomad.TypeDeploymentStatusUpdate}: true,
	{nomad.TopicDeployment, nomad.TypeDeploymentAllocHealth}:  true,
	{nomad.TopicNode, nomad.TypeNodeStreamEvent}:              true,
}

// failureOf is the line for an event whose object is in a failed state.
func failureOf(e nomad.Event) (string, bool) {
	switch {
	case e.Topic == nomad.TopicAllocation && e.State == statusFailed:
		return sprintf("allocation %s failed in namespace %s", e.Name, e.Namespace), true
	case e.Topic == nomad.TopicDeployment && e.State == statusFailed:
		return sprintf("deployment of %s failed in namespace %s", e.Name, e.Namespace), true
	case e.Topic == nomad.TopicNode && e.State == stateDown:
		return sprintf("client %s went down", e.Name), true
	}

	return "", false
}

// takeNoteOpened keeps the stream of the connection it was opened for and
// closes any other. An error shows nothing.
func (m Model) takeNoteOpened(msg noteOpenedMsg) (Model, tea.Cmd) {
	if msg.err != nil || msg.feed == nil {
		// Only a request with no answer is tried again, and one that servers
		// with no leader cannot answer: the cluster may be starting or
		// restarting, or the network to it may not be up yet. Any other
		// answer the cluster sends in place of the stream, a refusal or an
		// error, would be the same when asked again.
		if nomad.Unanswered(msg.err) && msg.id == m.note.id {
			return m, m.note.later()
		}

		return m, nil
	}

	if msg.id != m.note.id {
		msg.feed.Close()

		return m, nil
	}

	m.note.feed, m.note.heard, m.note.opened, m.note.from = msg.feed, false, m.now, msg.from

	return m, nextEvent(msg.feed)
}

// takeNoteEvents says the newest failure among the events.
func (m Model) takeNoteEvents(msg feedEventsMsg) (Model, tea.Cmd) {
	var text string

	m.note.heard = true

	for _, event := range msg.events {
		m.note.from = max(m.note.from, event.Index+1)

		if line, ok := m.note.take(event); ok {
			text = line
		}
	}

	if text != "" {
		m = m.noteFailure(text)
	}

	return m, nextEvent(m.note.feed)
}

// takeNoteEnded drops a stream that ended. One that ended at once and sent
// nothing was refused, and the note stays off for the connection. Any other
// dropped, and is followed again later from where it stopped.
func (m Model) takeNoteEnded(feedEndedMsg) (Model, tea.Cmd) {
	m.note.stop()

	if !m.note.heard && m.now.Sub(m.note.opened) < noteRefusedWithin {
		return m, nil
	}

	return m, m.note.later()
}

// later asks to follow the failures again after a while.
func (n noteState) later() tea.Cmd {
	return startTimer(noteAgain, noteAgainMsg{id: n.id})
}

// takeNoteAgain opens the stream again, unless the connection changed.
func (m Model) takeNoteAgain(msg noteAgainMsg) (Model, tea.Cmd) {
	if msg.id != m.note.id {
		return m, nil
	}

	return m, m.note.open(m.client)
}

// noteFailure shows the failure unless a message that matters more is on the
// status line: an error, what a running command says, or a warning of
// something else, like the file that keeps the text of an edit. It takes the
// place of an older failure.
func (m Model) noteFailure(text string) Model {
	f := m.flash
	if f.fresh(m.now) && (f.level == flashErr || f.held || (f.level == flashWarn && !f.ofNote)) {
		return m
	}

	m = m.warn(text)
	m.flash.ofNote = true

	return m
}
