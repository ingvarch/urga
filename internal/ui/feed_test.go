package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// someEvents are what the cluster keeps: a job registered, then an
// allocation of it failed.
func someEvents() []nomad.Event {
	return []nomad.Event{
		{
			Index: 10, Topic: "Job", Type: "JobRegistered", Namespace: "production", Key: "web",
			Name: "web", State: "running", At: time.Now().Add(-3 * time.Minute),
		},
		{
			Index: 11, Topic: "Allocation", Type: "AllocationUpdated", Namespace: "production", Key: "af1f37df-7b19-6b1c-da67-5e8f482b5a15",
			Name: "web.frontend[0]", State: "failed", At: time.Now().Add(-time.Minute),
		},
	}
}

// feedOf is a feed that sent these events and waits for more, and whether
// it was closed.
func feedOf(events ...nomad.Event) (*nomad.Feed, *bool) {
	c := make(chan nomad.Event, len(events))
	for _, event := range events {
		c <- event
	}

	closed := false

	return nomad.NewStream(c, nil, func() { closed = true }), &closed
}

// onEvents is the feed of the cluster, opened by name.
func onEvents(t *testing.T, client *fakeClient) Model {
	t.Helper()

	m, _ := sessionModel(t, client)

	return typeCommand(m, "events")
}

// arrives is one more event of the feed the screen reads.
func arrives(m Model, event nomad.Event) Model {
	m, _ = m.update(feedEventMsg{feed: m.screen.page.(feedPage).feed, event: event})

	return m
}

func selectedCells(m Model) []string {
	row, _ := m.list.table.selected()

	return row.cells
}

func TestEvents_ShowWhatHappenedNewestFirst(t *testing.T) {
	r := require.New(t)

	feed, _ := feedOf(someEvents()...)
	client := &fakeClient{feed: feed}
	m := onEvents(t, client)

	r.Equal("production", client.feedNamespace)
	r.Equal("Events (production) [2]", m.title())
	r.Equal([]string{"Topic", "Type", "Namespace", "Name", "State", "Age"}, m.screen.page.titles())

	rows := m.screen.page.rows(m.env())
	r.Equal([]string{"Allocation", "AllocationUpdated", "production", "web.frontend[0]", "failed", "1m"}, rows[0].cells)
	r.Equal("Job", rows[1].cells[0])

	// What failed is red; what is fine has no color.
	r.Equal(colorDead, rows[0].color)
	r.Nil(rows[1].color)
}

func TestEvents_AColorForWhatIsWrong(t *testing.T) {
	r := require.New(t)

	for state, want := range map[string]any{
		"failed": colorDead, "lost": colorDead, "down": colorDead,
		// A blocked evaluation could not place what it was asked to.
		"blocked": colorAttention,
		"pending": colorPending,
		"running": nil, "": nil,
	} {
		row := feedPage{events: []feedEvent{{Event: nomad.Event{State: state}}}}.rows(env{})[0]
		if want == nil {
			r.Nil(row.color, state)
		} else {
			r.Equal(want, row.color, state)
		}
	}
}

func TestEvents_AtTheTopTheCursorShowsTheNewest(t *testing.T) {
	r := require.New(t)

	feed, _ := feedOf(someEvents()...)
	m := onEvents(t, &fakeClient{feed: feed})

	// The cursor on the top row stays there as events arrive.
	m = arrives(m, nomad.Event{Index: 12, Topic: "Evaluation", Type: "EvaluationUpdated", Name: "web", State: "blocked"})
	r.Equal("Evaluation", selectedCells(m)[0])

	// Moved down, it stays on its event.
	m, _ = m.update(key('j'))
	r.Equal("Allocation", selectedCells(m)[0])

	m = arrives(m, nomad.Event{Index: 13, Topic: "Deployment", Type: "DeploymentStatusUpdate", Name: "web", State: "failed"})
	r.Equal("Allocation", selectedCells(m)[0])
}

func TestEvents_KeepTheLastThousand(t *testing.T) {
	r := require.New(t)

	feed, _ := feedOf()
	m := onEvents(t, &fakeClient{feed: feed})

	for i := range 1001 {
		m = arrives(m, nomad.Event{Index: uint64(i), Topic: "Job", Name: fmt.Sprintf("job-%04d", i)})
	}

	rows := m.screen.page.rows(m.env())
	r.Len(rows, 1000)
	r.Equal("job-1000", rows[0].cells[3])
	r.Equal("job-0001", rows[999].cells[3])
}

func TestEvents_EnterOpensWhatTheEventIsAbout(t *testing.T) {
	r := require.New(t)

	// A job opens alone, in its namespace.
	feed, _ := feedOf(someEvents()[0])
	m := onEvents(t, &fakeClient{feed: feed})

	m, cmd := m.update(enter())
	m = playOut(m, cmd)

	r.Equal(jobsPage{only: nomad.Job{ID: "web", Namespace: "production"}}, m.screen.page)

	// An allocation is read first, then opens on its tasks.
	feed, _ = feedOf(someEvents()...)
	client := &fakeClient{feed: feed, alloc: twoAllocs()[0]}
	m = onEvents(t, client)

	m, cmd = m.update(enter())
	m = playOut(m, cmd)

	r.Equal("af1f37df-7b19-6b1c-da67-5e8f482b5a15", client.askedID)
	r.IsType(tasksPage{}, m.screen.page)

	// A pool opens nothing.
	feed, _ = feedOf(nomad.Event{Topic: "NodePool", Type: "NodePoolUpserted", Name: "gpu"})
	m = onEvents(t, &fakeClient{feed: feed})
	r.NotContains(m.hints(), hint{Key: "<enter>", Description: "Open"})
}

func TestEvents_TheFeedFollowsTheSession(t *testing.T) {
	r := require.New(t)

	feed, closed := feedOf(someEvents()...)
	client := &fakeClient{feed: feed}
	m := onEvents(t, client)
	m.namespaceOrder = []string{"production", "staging"}

	// Another namespace is a feed of its own, read from what the cluster
	// keeps.
	next, nextClosed := feedOf(someEvents()...)
	client.feed = next

	m, cmd := m.update(key('2'))
	m = playOut(m, cmd)

	r.True(*closed)
	r.Equal("staging", client.feedNamespace)
	r.Equal("Events (staging) [2]", m.title())

	// Leaving the screen closes the feed.
	_ = typeCommand(m, "jobs")

	r.True(*nextClosed)
}

func TestEvents_WhatEachTopicOpens(t *testing.T) {
	r := require.New(t)

	for topic, kind := range map[string]string{
		nomad.TopicJob: nomad.MatchJob, nomad.TopicAllocation: nomad.MatchAlloc,
		nomad.TopicDeployment: nomad.MatchDeployment, nomad.TopicEvaluation: nomad.MatchEval,
		nomad.TopicNode: nomad.MatchNode, nomad.TopicService: nomad.MatchService,
		nomad.TopicNodePool: "", "ACLToken": "",
	} {
		m := matchOf(nomad.Event{Topic: topic, Key: "k1", Name: "web", Namespace: "production"})
		r.Equal(kind, m.Kind, topic)

		if kind != "" {
			r.Equal(nomad.Match{Kind: kind, ID: "k1", Name: "web", Namespace: "production"}, m, topic)
		}
	}
}

func TestEvents_AFeedThatBreaksSaysWhy(t *testing.T) {
	r := require.New(t)

	feed, _ := feedOf(someEvents()...)
	m := onEvents(t, &fakeClient{feed: feed})

	m, _ = m.update(feedEndedMsg{feed: feed, err: errTest})

	r.Contains(plain(statusLine(m)), "no answer from the client")
	r.Len(m.screen.page.rows(m.env()), 2)
}
