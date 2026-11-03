package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// noteKept is how many failures the note remembers.
const noteKept = 1000

// noteOpenedMsg is the stream of failures the cluster opened, for the
// connection with that id. It is not a streamOpened: the model closes a
// stale one itself.
type noteOpenedMsg struct {
	id   int
	feed *nomad.Feed
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
}

// open asks for the failures of every namespace, from the event after the
// newest one. Asked from 0, the cluster replays all it keeps, and a failure
// of hours ago would show as news.
func (n noteState) open(client eventsClient) tea.Cmd {
	id := n.id

	return func() tea.Msg {
		ctx := context.Background()

		index, err := client.LatestIndex(ctx)
		if err != nil {
			return noteOpenedMsg{id: id, err: err}
		}

		feed, err := client.Failures(ctx, nomad.AllNamespaces, index+1)

		return noteOpenedMsg{id: id, feed: feed, err: err}
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
// closes any other. A refusal shows nothing.
func (m Model) takeNoteOpened(msg noteOpenedMsg) (Model, tea.Cmd) {
	if msg.err != nil || msg.feed == nil {
		return m, nil
	}

	if msg.id != m.note.id {
		msg.feed.Close()

		return m, nil
	}

	m.note.feed = msg.feed

	return m, nextEvent(msg.feed)
}

// takeNoteEvents says the newest failure among the events.
func (m Model) takeNoteEvents(msg feedEventsMsg) (Model, tea.Cmd) {
	var text string

	for _, event := range msg.events {
		if line, ok := m.note.take(event); ok {
			text = line
		}
	}

	if text != "" {
		m = m.noteFailure(text)
	}

	return m, nextEvent(m.note.feed)
}

// takeNoteEnded drops a stream that ended.
func (m Model) takeNoteEnded(feedEndedMsg) (Model, tea.Cmd) {
	m.note.stop()

	return m, nil
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
