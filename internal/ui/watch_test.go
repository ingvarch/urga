package ui

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// watching is a model whose screen the cluster says changes about. The
// stream itself is read by hand: a test must not wait on it.
func watching(t *testing.T, client *fakeClient) Model {
	t.Helper()

	m := newTestModel(client)
	m = drain(m, m.fetch())

	m, _ = m.update(m.watch()())

	return m
}

func TestWatch_AScreenAsksForTheTopicItShows(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), changes: newChanges()}

	m := watching(t, client)

	// The jobs screen watches jobs, in the namespace it is looking at.
	r.Equal([]string{nomad.TopicJob}, client.watchedTopics)
	r.Equal("production", client.watchedNamespace)
	r.True(m.watching)
}

func TestWatch_AChangeAsksTheClusterOnceTheBurstSettles(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), changes: newChanges()}

	m := watching(t, client)
	before := client.calls

	// A deploy fires events by the dozen. One answer is enough for all of
	// them: the screen must not ask once per event.
	for range 20 {
		m, _ = m.update(changeMsg{change: nomad.Change{Topic: nomad.TopicJob}})
	}

	r.Equal(before, client.calls)
	r.True(m.settling)

	m, cmd := m.update(settleMsg{})
	drain(m, cmd)

	r.Equal(before+1, client.calls)
}

func TestWatch_TheScreenPollsSlowlyWhileTheStreamIsUp(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), changes: newChanges()}

	m := watching(t, client)

	// Watching is the reason a screen may take its time asking again.
	r.Equal(slowPoll, m.pollEvery())

	m, _ = m.update(watchEndedMsg{err: errors.New("Permission denied")})

	// Without the stream, the screen goes back to asking on its own.
	r.False(m.watching)
	r.Equal(time.Millisecond, m.pollEvery())
}

func TestWatch_AStreamTheClusterRefusesIsNotAnError(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), watchErr: errors.New("Permission denied")}

	m := newTestModel(client)
	m = drain(m, m.fetch())

	m, _ = m.update(m.watch()())

	// A cluster that will not stream is a cluster urga polls, and says
	// nothing about it over the rows.
	r.False(m.watching)
	r.Nil(m.err)
	r.Contains(plain(m.render()), "web")
}

func TestWatch_LeavingAScreenLetsItsStreamGo(t *testing.T) {
	r := require.New(t)

	changes := newChanges()
	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), changes: changes}

	m := watching(t, client)

	m, _ = m.update(enter())

	// What the jobs screen opened is closed rather than left running, and
	// the allocations watch allocations.
	r.True(changes.closed)

	m.update(m.watch()())
	r.Equal([]string{nomad.TopicAllocation}, client.watchedTopics)
}

func TestWatch_AScreenWithNothingToWatchAsksOnItsOwn(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{namespaces: twoNamespaces(), changes: newChanges()}

	m := newTestModel(client)
	m, _ = m.update(key(':'))
	m = typeIn(m, "namespaces")
	m, _ = m.update(enter())

	// The cluster says nothing about namespaces, so there is nothing to
	// ask it for.
	r.Nil(m.watch())
	r.False(m.watching)
}

func TestWatch_ANewNamespaceIsWatchedInstead(t *testing.T) {
	r := require.New(t)

	changes := newChanges()
	client := &fakeClient{jobs: twoJobs(), namespaces: twoNamespaces(), changes: changes}

	m := watching(t, client)
	m, _ = m.update(namespacesMsg(twoNamespaces()))

	m, _ = m.update(key('2'))

	// The stream is asked for one namespace at a time, so switching one
	// closes what was open and asks again for the other.
	r.True(changes.closed)

	m.update(m.watch()())
	r.Equal("staging", client.watchedNamespace)
}
