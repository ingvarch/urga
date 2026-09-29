package ui

import (
	"context"
	"image/color"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// feedKept is how many events the feed keeps; the oldest go first.
const feedKept = 1000

// Messages of the feed. Each carries its feed: a page that was left closes
// its feed, and messages still on their way from it are not the page's.
type (
	// feedOpenedMsg is the feed the cluster opened for the page.
	feedOpenedMsg struct{ feed *nomad.Feed }

	// feedEventsMsg are the events of the feed a page reads that were
	// waiting, oldest first.
	feedEventsMsg struct {
		feed   *nomad.Feed
		events []nomad.Event
	}

	// feedEndedMsg is the end of a feed, with the error it ended with.
	feedEndedMsg struct {
		feed *nomad.Feed
		err  error
	}
)

// feedEvent is an event as the page keeps it, numbered in the order it
// arrived: the cursor stays on an event by its number.
type feedEvent struct {
	nomad.Event

	seq int
}

// feedPage is what happens in the cluster, event by event, newest first. It
// shows the events as they came; no other screen is built out of them.
type feedPage struct {
	ofTheSession

	feed   *nomad.Feed
	events []feedEvent
	seen   int

	// namespace is the one the events were read in, and last the index of
	// the newest of them: back on the same namespace, the page goes on after
	// it.
	namespace string
	last      uint64
}

var feedTitles = []string{"Topic", "Type", "Namespace", "Name", "State", "Age"}

func (feedPage) title(e env, count int) string {
	return sprintf("Events (%s) [%d]", namespaceLabel(e.namespace), count)
}

func (feedPage) titles() []string { return feedTitles }

// topics: none. The page reads the stream itself.
func (feedPage) topics() []string  { return nil }
func (feedPage) fetch(env) tea.Cmd { return nil }
func (feedPage) follows() bool     { return false }
func (feedPage) newestOnTop()      {}

// open reads the feed. Back from a screen opened on an event, the page keeps
// what it read and goes on after the newest event; in another namespace it
// starts over from what the cluster still keeps.
func (p feedPage) open(e env) (page, tea.Cmd) {
	p.stop()

	from := uint64(1)
	if p.namespace == e.namespace && p.last > 0 {
		from = p.last + 1
	} else {
		p.events, p.last = nil, 0
	}

	p.namespace = e.namespace

	client, namespace := e.client, e.namespace

	return p, func() tea.Msg {
		feed, err := client.Feed(context.Background(), namespace, from)
		if err != nil {
			return errMsg{err: err}
		}

		return feedOpenedMsg{feed: feed}
	}
}

func (p feedPage) close() page {
	p.stop()

	return p
}

// stop closes the feed, which stops the request behind it.
func (p *feedPage) stop() {
	if p.feed != nil {
		p.feed.Close()
		p.feed = nil
	}
}

// take keeps what the feed sends. None of it counts as the answer to the
// page's request.
func (p feedPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	switch msg := msg.(type) {
	case feedOpenedMsg:
		// One opened next to the feed the page reads is the session's to
		// close.
		if p.feed != nil {
			return p, outcome{}, false
		}

		p.feed = msg.feed

		return p, outcome{cmd: nextEvent(p.feed), reading: true}, true

	case feedEventsMsg:
		if msg.feed != p.feed {
			return p, outcome{}, false
		}

		// They come oldest first, and the newest goes on top.
		arrived := make([]feedEvent, len(msg.events))
		for i, event := range msg.events {
			p.seen++
			p.last = max(p.last, event.Index)
			arrived[len(arrived)-1-i] = feedEvent{Event: event, seq: p.seen}
		}

		p.events = append(arrived, p.events...)
		p.events = p.events[:min(len(p.events), feedKept)]

		return p, outcome{cmd: nextEvent(p.feed), reading: true}, true

	case feedEndedMsg:
		if msg.feed != p.feed {
			return p, outcome{}, false
		}

		p.feed = nil

		if msg.err != nil {
			return p, outcome{now: []tea.Msg{errMsg{err: msg.err}}, reading: true}, true
		}

		return p, outcome{reading: true}, true
	}

	return p, outcome{}, false
}

// nextEvent waits for the next event of the feed, and takes along the ones
// already waiting behind it: the screen is drawn once for all of them, not
// once each. The events arrive as messages, one command at a time, so
// nothing writes to the model from a goroutine.
func nextEvent(feed *nomad.Feed) tea.Cmd {
	if feed == nil {
		return nil
	}

	return func() tea.Msg {
		select {
		case event, ok := <-feed.C:
			if !ok {
				return feedEndedMsg{feed: feed, err: reasonOf(feed)}
			}

			return feedEventsMsg{feed: feed, events: withWaiting(feed, event)}

		case err := <-feed.Err:
			return feedEndedMsg{feed: feed, err: err}
		}
	}
}

// withWaiting is the first event and every one waiting behind it, oldest
// first. The end of the feed is left for the next read to find.
func withWaiting(feed *nomad.Feed, first nomad.Event) []nomad.Event {
	events := []nomad.Event{first}

	for {
		select {
		case event, ok := <-feed.C:
			if !ok {
				return events
			}

			events = append(events, event)
		default:
			return events
		}
	}
}

func (p feedPage) rows(env) []tableRow {
	rows := make([]tableRow, 0, len(p.events))

	for _, event := range p.events {
		row := tableRow{color: stateColor(event.State)}
		row.add(event.Topic, event.Type, event.Namespace, event.Name, event.State)
		row.addAge(event.At)

		rows = append(rows, row)
	}

	return rows
}

// ids name the events by the order they arrived in.
func (p feedPage) ids(env) []string {
	ids := make([]string, 0, len(p.events))
	for _, event := range p.events {
		ids = append(ids, strconv.Itoa(event.seq))
	}

	return ids
}

const (
	stateBlocked = "blocked"
	stateDown    = "down"
)

// stateColor shows the state an event reports: what failed is red, what
// could not be placed needs attention, what waits is pending.
func stateColor(state string) color.Color {
	switch state {
	case statusFailed, statusLost, stateDown:
		return colorDead
	case stateBlocked:
		return colorAttention
	case statusPending:
		return colorPending
	}

	return nil
}

var feedKeys = []pageKey[feedPage]{
	{press: "enter", label: "Open", do: openEvent, offered: func(p feedPage, e env) bool {
		event, ok := pickedFrom(e, p.events)

		return ok && matchOf(event.Event).Kind != ""
	}},
}

func (p feedPage) keys(e env) []keyHint { return hintsOf(p, e, feedKeys) }

func (p feedPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, feedKeys, k)
}

// openEvent opens what the event under the cursor is about, the way a search
// opens what it found.
func openEvent(p feedPage, e env) (feedPage, outcome) {
	event, ok := pickedFrom(e, p.events)
	if !ok {
		return p, outcome{}
	}

	return p, openMatch(e.client, matchOf(event.Event))
}

// matchOf is what an event is about, as a search finds it: no kind for what
// urga opens nothing of.
func matchOf(event nomad.Event) nomad.Match {
	m := nomad.Match{ID: event.Key, Name: event.Name, Namespace: event.Namespace}

	switch event.Topic {
	case nomad.TopicJob:
		m.Kind = nomad.MatchJob
	case nomad.TopicAllocation:
		m.Kind = nomad.MatchAlloc
	case nomad.TopicDeployment:
		m.Kind = nomad.MatchDeployment
	case nomad.TopicEvaluation:
		m.Kind = nomad.MatchEval
	case nomad.TopicNode:
		m.Kind = nomad.MatchNode
	case nomad.TopicService:
		m.Kind = nomad.MatchService
	}

	return m
}
