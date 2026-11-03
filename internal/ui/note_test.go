package ui

import (
	"errors"
	"io"
	"strconv"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// noting is a session whose stream of failures opened.
func noting(t *testing.T, client *fakeClient) Model {
	t.Helper()

	m := newTestModel(client)
	m, _ = m.update(m.note.open(client)())

	return m
}

func failedAlloc(key, name string) nomad.Event {
	return nomad.Event{
		Topic: nomad.TopicAllocation, Type: nomad.TypeAllocationUpdated, Key: key,
		Namespace: "production", Name: name, State: statusFailed,
	}
}

func noteArrives(m Model, events ...nomad.Event) Model {
	return noteArrivesOn(m, m.note.feed, events...)
}

func noteArrivesOn(m Model, feed *nomad.Feed, events ...nomad.Event) Model {
	m, _ = m.update(feedEventsMsg{feed: feed, events: events})

	return m
}

func TestNote_FollowsEveryNamespaceFromTheStart(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{latestIndex: 40}
	fire(t, newTestModel(client).Init())

	// From the event after the newest one: the cluster replays what it
	// keeps when it is asked from 0.
	r.Equal(1, client.failuresCalls)
	r.Equal(nomad.AllNamespaces, client.failuresNamespace)
	r.Equal(uint64(41), client.failuresFrom)
}

func TestNote_NoIndexToStartFromShowsNothing(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{latestErr: forbidden(t)}
	m := newTestModel(client)

	m, cmd := m.update(m.note.open(client)())

	r.Nil(cmd)
	r.Zero(client.failuresCalls)
	r.Empty(m.flash.text)
	r.Nil(m.note.feed)
}

func TestNote_SaysWhatFailed(t *testing.T) {
	r := require.New(t)

	feed, _ := feedOf()
	m := noting(t, &fakeClient{failures: feed})

	m = noteArrives(m, failedAlloc("a1", "web.frontend[0]"))

	r.Contains(plain(statusLine(m)), "! allocation web.frontend[0] failed in namespace production")
	r.Equal(flashWarn, m.flash.level)
}

func TestNote_KeepsReadingItsStream(t *testing.T) {
	r := require.New(t)

	events := make(chan nomad.Event, 1)
	events <- failedAlloc("a1", "web.frontend[0]")
	client := &fakeClient{failures: nomad.NewStream(events, nil, func() {})}

	m := newTestModel(client)
	m, cmd := m.update(m.note.open(client)())
	r.NotNil(cmd)

	m, cmd = m.update(cmd())
	r.Contains(plain(statusLine(m)), "allocation web.frontend[0] failed")
	r.NotNil(cmd)

	events <- failedAlloc("a2", "api.backend[1]")
	m, _ = m.update(cmd())
	r.Contains(plain(statusLine(m)), "allocation api.backend[1] failed")
}

func TestFailureOf_NamesTheFailureAndItsNamespace(t *testing.T) {
	for name, c := range map[string]struct {
		event nomad.Event
		want  string
	}{
		"allocation failed": {failedAlloc("a1", "web.frontend[0]"), "allocation web.frontend[0] failed in namespace production"},
		"deployment failed": {
			nomad.Event{Topic: nomad.TopicDeployment, Namespace: "production", Name: "web", State: statusFailed},
			"deployment of web failed in namespace production",
		},
		"node down": {
			nomad.Event{Topic: nomad.TopicNode, Name: "client-01", State: stateDown},
			"client client-01 went down",
		},
		"allocation running":       {nomad.Event{Topic: nomad.TopicAllocation, Name: "w", State: "running"}, ""},
		"allocation lost":          {nomad.Event{Topic: nomad.TopicAllocation, Name: "w", State: statusLost}, ""},
		"allocation complete":      {nomad.Event{Topic: nomad.TopicAllocation, Name: "w", State: "complete"}, ""},
		"deployment successful":    {nomad.Event{Topic: nomad.TopicDeployment, Name: "w", State: "successful"}, ""},
		"deployment cancelled":     {nomad.Event{Topic: nomad.TopicDeployment, Name: "w", State: "cancelled"}, ""},
		"node ready":               {nomad.Event{Topic: nomad.TopicNode, Name: "n", State: "ready"}, ""},
		"node initializing":        {nomad.Event{Topic: nomad.TopicNode, Name: "n", State: "initializing"}, ""},
		"job":                      {nomad.Event{Topic: nomad.TopicJob, Name: "w", State: statusFailed}, ""},
		"evaluation":               {nomad.Event{Topic: nomad.TopicEvaluation, Name: "w", State: statusFailed}, ""},
		"state not in the payload": {nomad.Event{Topic: nomad.TopicAllocation, Name: "w"}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			text, ok := failureOf(c.event)

			require.Equal(t, c.want != "", ok)
			require.Equal(t, c.want, text)
		})
	}
}

func TestNote_SaysAFailureOnce(t *testing.T) {
	r := require.New(t)

	feed, _ := feedOf()
	m := noting(t, &fakeClient{failures: feed})

	again := failedAlloc("a1", "web.frontend[0]")
	again.Type = "PlanResult"

	m = noteArrives(m, failedAlloc("a1", "web.frontend[0]")).quiet()
	m = noteArrives(m, again)

	r.Empty(m.flash.text)

	down := nomad.Event{Topic: nomad.TopicNode, Type: nomad.TypeNodeStreamEvent, Key: "n1", Name: "client-01", State: stateDown}
	ready := down
	ready.State = "ready"

	m = noteArrives(m, down)
	r.NotEmpty(m.flash.text)

	m = noteArrives(m.quiet(), ready)
	r.Empty(m.flash.text)

	m = noteArrives(m, down)
	r.NotEmpty(m.flash.text)
}

func TestNoteState_ShowsTheEventThatReportsTheFailure(t *testing.T) {
	for name, event := range map[string]nomad.Event{
		"the client fails an allocation": failedAlloc("a1", "web.frontend[0]"),
		"a status update fails a deployment": {
			Topic: nomad.TopicDeployment, Type: nomad.TypeDeploymentStatusUpdate, Key: "d1", Name: "web", State: statusFailed,
		},
		"an unhealthy allocation fails a deployment": {
			Topic: nomad.TopicDeployment, Type: nomad.TypeDeploymentAllocHealth, Key: "d1", Name: "web", State: statusFailed,
		},
		"a status update takes a client down": {
			Topic: nomad.TopicNode, Type: nomad.TypeNodeStreamEvent, Key: "n1", Name: "client-01", State: stateDown,
		},
	} {
		t.Run(name, func(t *testing.T) {
			var note noteState

			_, ok := note.take(event)
			require.True(t, ok)
		})
	}
}

func TestNoteState_ShowsNothingOfAFailureSentAgain(t *testing.T) {
	again := func(topic, kind, state string) nomad.Event {
		return nomad.Event{Topic: topic, Type: kind, Key: "k1", Namespace: "production", Name: "web", State: state}
	}

	for name, event := range map[string]nomad.Event{
		"a plan replaces a failed allocation":          again(nomad.TopicAllocation, "PlanResult", statusFailed),
		"a forced reschedule of a failed allocation":   again(nomad.TopicAllocation, "AllocationUpdateDesiredStatus", statusFailed),
		"the health of a failed allocation":            again(nomad.TopicAllocation, nomad.TypeDeploymentAllocHealth, statusFailed),
		"a down client is purged":                      again(nomad.TopicNode, "NodeDeregistration", stateDown),
		"a plan sends a deployment that failed before": again(nomad.TopicDeployment, "PlanResult", statusFailed),
	} {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			var note noteState

			_, ok := note.take(event)
			r.False(ok)

			// The object was failed already: the next report of it is not
			// news either.
			switch event.Topic {
			case nomad.TopicAllocation:
				event.Type = nomad.TypeAllocationUpdated
			case nomad.TopicNode:
				event.Type = nomad.TypeNodeStreamEvent
			case nomad.TopicDeployment:
				event.Type = nomad.TypeDeploymentStatusUpdate
			}

			_, ok = note.take(event)
			r.False(ok)
		})
	}
}

func TestNote_NamesTheNewestOfABurst(t *testing.T) {
	feed, _ := feedOf()
	m := noting(t, &fakeClient{failures: feed})

	m = noteArrives(m, failedAlloc("a1", "web.frontend[0]"), failedAlloc("a2", "api.backend[1]"))

	require.Contains(t, plain(statusLine(m)), "allocation api.backend[1] failed")
}

func TestNote_DoesNotCoverAnError(t *testing.T) {
	feed, _ := feedOf()
	m := noting(t, &fakeClient{failures: feed})

	m, _ = m.update(errMsg{err: errors.New("connection refused")})
	m = noteArrives(m, failedAlloc("a1", "web.frontend[0]"))
	require.Contains(t, plain(statusLine(m)), "connection refused")

	m = m.quiet().busy("Running the command in web...")
	m = noteArrives(m, failedAlloc("a2", "web.frontend[1]"))
	require.Contains(t, plain(statusLine(m)), "Running the command in web...")
}

func TestNote_TakesThePlaceOfAnErrorWhoseTimeIsUp(t *testing.T) {
	feed, _ := feedOf()
	m := noting(t, &fakeClient{failures: feed})

	m, _ = m.update(errMsg{err: errors.New("connection refused")})
	m.flash.at = m.flash.at.Add(-flashFor)
	m = noteArrives(m, failedAlloc("a1", "web.frontend[0]"))

	require.Contains(t, plain(statusLine(m)), "allocation web.frontend[0] failed in namespace production")
}

func TestNote_DoesNotCoverAWarningThatIsNotAFailure(t *testing.T) {
	feed, _ := feedOf()
	m := noting(t, &fakeClient{failures: feed})

	m = m.warn(keptIn("/tmp/plan.hcl") + " The edit was not opened again: the screen changed.")
	m = noteArrives(m, failedAlloc("a1", "web.frontend[0]"))

	require.Contains(t, plain(statusLine(m)), "/tmp/plan.hcl")
}

func TestNote_ReplacesAnOlderFailure(t *testing.T) {
	feed, _ := feedOf()
	m := noting(t, &fakeClient{failures: feed})

	m = noteArrives(m, failedAlloc("a1", "web.frontend[0]"))
	m = noteArrives(m, failedAlloc("a2", "api.backend[1]"))

	require.Contains(t, plain(statusLine(m)), "allocation api.backend[1] failed")
}

func TestNote_ARefusedStreamShowsNothing(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{failuresErr: forbidden(t)}
	m := newTestModel(client)

	m, cmd := m.update(m.note.open(client)())

	r.Nil(cmd)
	r.Empty(m.flash.text)
	r.Nil(m.note.feed)
}

func TestNote_AStreamThatEndsShowsNothing(t *testing.T) {
	r := require.New(t)

	feed, closed := feedOf()
	m := noting(t, &fakeClient{failures: feed})

	m, cmd := m.update(feedEndedMsg{feed: feed, err: io.EOF})

	r.Nil(cmd)
	r.Empty(m.flash.text)
	r.Nil(m.note.feed)
	r.True(*closed)
}

func TestNote_ClosesItsStreamWhenTheClusterChanges(t *testing.T) {
	r := require.New(t)

	m, clusters := onDev(t)
	old, closed := feedOf()
	clusters.dev.failures = old
	m, _ = m.update(m.note.open(clusters.dev)())
	r.NotNil(m.note.feed)

	devFailure := failedAlloc("a1", "web.frontend[0]")
	devFailure.Index = 500
	m = noteArrives(m, devFailure)

	feed, feedClosed := feedOf()
	clusters.prod.failures, clusters.prod.latestIndex = feed, 40

	m, cmd := m.update(connectedMsg{attempt: m.attempt, conn: Connection{Name: "prod", Client: clusters.prod}})
	r.True(*closed)

	// prod starts at its own newest event: the index of dev means nothing
	// there.
	opened, ok := noteOpenedIn(t, cmd)
	r.True(ok)
	r.Equal(1, clusters.prod.failuresCalls)
	r.Equal(uint64(41), clusters.prod.failuresFrom)

	m, _ = m.update(opened)
	r.Same(feed, m.note.feed)

	m = noteArrivesOn(m.quiet(), old, failedAlloc("a2", "web.frontend[1]"))
	m, _ = m.update(feedEndedMsg{feed: old, err: io.EOF})
	r.NotContains(m.flash.text, "failed")
	r.Same(feed, m.note.feed)
	r.False(*feedClosed)

	// What failed on dev is news on prod.
	m = noteArrives(m, failedAlloc("a1", "web.frontend[0]"))
	r.Contains(plain(statusLine(m)), "allocation web.frontend[0] failed")
}

// noteOpenedIn is the stream of failures among what a model asked for.
func noteOpenedIn(t *testing.T, cmd tea.Cmd) (noteOpenedMsg, bool) {
	t.Helper()

	msg := within(cmd, 200*time.Millisecond)

	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, next := range batch {
			if opened, ok := noteOpenedIn(t, next); ok {
				return opened, true
			}
		}
	}

	opened, ok := msg.(noteOpenedMsg)

	return opened, ok
}

func TestNote_ClosesItsStreamWhenTheRegionChanges(t *testing.T) {
	r := require.New(t)

	feed, closed := feedOf()
	client := &fakeClient{jobs: twoJobs(), failures: feed}
	m := regionalModel(t, client)
	m, _ = m.update(m.note.open(client)())
	r.Equal(1, client.failuresCalls)

	m, cmd := runLine(m, "region us")

	r.True(*closed)
	fire(t, cmd)
	r.Equal(2, client.failuresCalls)
}

func TestNote_AStreamOpenedForAClusterLeftIsClosed(t *testing.T) {
	r := require.New(t)

	m, clusters := onDev(t)
	stale := m.note.id
	m, _ = m.update(connectedMsg{attempt: m.attempt, conn: Connection{Name: "prod", Client: clusters.prod}})

	feed, closed := feedOf()
	m, _ = m.update(noteOpenedMsg{id: stale, feed: feed})

	r.True(*closed)
	r.Nil(m.note.feed)
}

func TestNoteState_StartsOverPastItsLimit(t *testing.T) {
	var note noteState

	for i := range noteKept + 1 {
		key := strconv.Itoa(i)
		_, ok := note.take(failedAlloc(key, "web"))
		require.True(t, ok)
	}

	require.Len(t, note.failing, 1)
}
