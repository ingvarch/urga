package ui

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// fire runs what a model asked for, without waiting on the commands that
// are waiting on the cluster.
func fire(t *testing.T, cmd tea.Cmd) {
	t.Helper()

	if cmd == nil {
		return
	}

	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()

	select {
	case msg := <-done:
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, next := range batch {
				fire(t, next)
			}
		}
	case <-time.After(200 * time.Millisecond):
	}
}

func TestWatch_ArrivingAtAScreenOpensItsStream(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), changes: newChanges()}

	m := newTestModel(client)

	// Opening urga watches what the first screen shows, without anything
	// asking for it by hand.
	fire(t, m.Init())
	r.Equal([]string{nomad.TopicJob}, client.watchedTopics)

	m, _ = m.update(jobsMsg(twoJobs()))

	_, cmd := m.update(enter())
	fire(t, cmd)

	// And walking into the allocations watches allocations.
	r.Equal([]string{nomad.TopicAllocation}, client.watchedTopics)
}

// watching is a model whose screen watches the event stream. The stream
// itself is read by hand: a test must not wait on it.
func watching(t *testing.T, client *fakeClient) Model {
	t.Helper()

	m := newTestModel(client)
	m = drain(m, m.fetch())

	m, _ = m.update(m.watchScreen()())

	return m
}

func TestWatch_AScreenAsksForTheTopicItShows(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), changes: newChanges()}

	m := watching(t, client)

	// The jobs screen watches jobs, in the namespace it is looking at.
	r.Equal([]string{nomad.TopicJob}, client.watchedTopics)
	r.Equal("production", client.watchedNamespace)
	r.True(m.watch.live())
}

func TestWatch_AChangeAsksTheClusterOnceTheBurstSettles(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), changes: newChanges()}

	m := watching(t, client)
	before := client.calls

	// A deploy fires events by the dozen. One answer is enough for all of
	// them: the screen must not ask once per event.
	for range 20 {
		m, _ = m.update(changeMsg{})
	}

	r.Equal(before, client.calls)
	r.True(m.watch.settling)

	m, cmd := m.update(settleMsg{})
	drain(m, cmd)

	r.Equal(before+1, client.calls)
}

func TestWatch_TheScreenPollsSlowlyWhileTheStreamIsUp(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), changes: newChanges()}

	m := watching(t, client)

	// While it watches, a screen may poll slowly.
	r.Equal(slowPoll, m.pollEvery())

	m, _ = m.update(watchEndedMsg{})

	// Without the stream, the screen polls at its usual pace again.
	r.False(m.watch.live())
	r.Equal(time.Millisecond, m.pollEvery())
}

func TestWatch_AStreamTheClusterRefusesIsNotAnError(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), watchErr: errors.New("Permission denied")}

	m := newTestModel(client)
	m = drain(m, m.fetch())

	m, _ = m.update(m.watchScreen()())

	// A cluster that will not stream is a cluster urga polls, and the rows
	// stay on the screen with no error over them.
	r.False(m.watch.live())
	r.NotEqual(flashErr, m.flash.level)
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

	m.update(m.watchScreen()())
	r.Equal([]string{nomad.TopicAllocation}, client.watchedTopics)
}

func TestWatch_AScreenWithNothingToWatchAsksOnItsOwn(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{namespaces: twoNamespaces(), changes: newChanges()}

	m := newTestModel(client)
	m, _ = m.update(key(':'))
	m = typeIn(m, "namespaces")
	m, _ = m.update(enter())

	// The cluster sends no events about namespaces, so there is nothing
	// to watch.
	r.Nil(m.watchScreen())
	r.False(m.watch.live())
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

	m.update(m.watchScreen()())
	r.Equal("staging", client.watchedNamespace)
}

func TestWatch_OneTimerHoweverManyAnswersArrive(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), changes: newChanges()}

	m := watching(t, client)

	// The timer that was set when the list first arrived has fired, so
	// there is room for exactly one more.
	m, _ = m.update(pollMsg{})

	// Every answer from the cluster used to schedule a poll of its own, so
	// a talkative cluster ended up with a timer per burst, for the rest of
	// the session.
	timers := 0

	for range 10 {
		var cmd tea.Cmd

		m, cmd = m.update(jobsMsg(twoJobs()))
		if cmd != nil {
			timers++
		}
	}

	r.Equal(1, timers)

	// The timer that fired makes room for the next one, and no more.
	m, _ = m.update(pollMsg{})

	m, cmd := m.update(jobsMsg(twoJobs()))
	r.NotNil(cmd)

	_, cmd = m.update(jobsMsg(twoJobs()))
	r.Nil(cmd)
}

func TestWatch_ASettledBurstStartsNoTimerOfItsOwn(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), changes: newChanges()}

	m := watching(t, client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, _ = m.update(changeMsg{})

	next, cmd := m.update(settleMsg{})
	r.NotNil(cmd)

	// A request made because of an event is still one chain: the answer to
	// it must not add another timer.
	_, cmd = next.update(jobsMsg(twoJobs()))
	r.Nil(cmd)
}

func TestWatch_TheEndOfAStreamThatWasLeftIsNotThisOne(t *testing.T) {
	r := require.New(t)

	first, second := newChanges(), newChanges()
	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), changes: first}

	m := watching(t, client)

	// Into the allocations: the stream of the jobs closes, and the reader
	// that was waiting on it answers late.
	m, _ = m.update(enter())
	client.changes = second
	m, _ = m.update(m.watchScreen()())

	m, _ = m.update(watchEndedMsg{})

	// The end of a stream that was closed does not affect the one that is
	// open now.
	r.True(m.watch.live())
	r.False(second.closed)
}

func TestWatch_AStreamThatArrivesTooLateIsLetGoOf(t *testing.T) {
	r := require.New(t)

	late := newChanges()
	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), changes: newChanges()}

	m := watching(t, client)

	m, _ = m.update(enter())
	m, _ = m.update(m.watchScreen()())

	// The stream the jobs screen asked for arrives after the screen was
	// left: it is closed rather than left running.
	m.update(watchingMsg{changes: late.stream()})

	r.True(late.closed)
}

func TestWatch_TheStreamFollowsTheNamespaceTheRowsComeFrom(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), namespaces: twoNamespaces(), changes: newChanges()}

	m := watching(t, client)
	m, _ = m.update(namespacesMsg(twoNamespaces()))

	// Down into the allocations, switch namespace there, and back.
	m, _ = m.update(enter())
	m, _ = m.update(allocsMsg(twoAllocs()))
	m, _ = m.update(key('2'))
	m, _ = m.update(escape())

	// The jobs screen reads the namespace of the session, so the stream
	// has to watch that one and not the one the screen was opened in.
	m.update(m.watchScreen()())
	r.Equal("staging", client.watchedNamespace)
}

func TestWatch_EveryListTheClusterTalksAboutIsWatched(t *testing.T) {
	r := require.New(t)

	// A list the cluster has a topic for is watched; one it does not, like
	// the namespaces or the variables, is asked for on a timer.
	watched := map[*view][]string{
		jobsView:        {nomad.TopicJob},
		allocationsView: {nomad.TopicAllocation},
		deploymentsView: {nomad.TopicDeployment},
		evaluationsView: {nomad.TopicEvaluation},
		nodesView:       {nomad.TopicNode},
		servicesView:    {nomad.TopicService},
		nodePoolsView:   {nomad.TopicNodePool},
	}

	// Each is checked the way the program opens it, page and all.
	for v, topics := range watched {
		r.Equal(topics, v.open().topics(), nameOf(v))
	}

	for _, v := range []*view{namespacesView, variablesView, serversView} {
		r.Empty(v.open().topics(), nameOf(v))
	}
}

func TestWatch_AClusterThatWillNotStreamSaysSo(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), watchErr: errors.New("Permission denied")}

	m := newTestModel(client)
	m = drain(m, m.fetch())

	m, _ = m.update(m.watchScreen()())

	// Nothing is broken, but the screen is no longer live and the person
	// at the keyboard has no other way of knowing.
	out := plain(m.render())
	r.Contains(out, "Permission denied")
	r.Contains(out, "asking every")

	// It is worth knowing, not an error of the screen: the rows are there
	// and the list is not marked as failed.
	r.NotEqual(flashErr, m.flash.level)
	r.Contains(out, "web")
}

func TestWatch_LeavingAScreenSaysNothing(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), changes: newChanges()}

	m := watching(t, client)

	// The stream of a screen that was left ends because urga closed it.
	// Nothing about that is news.
	m, _ = m.update(watchEndedMsg{id: m.watch.id})

	r.Empty(m.flash.text)
}

func TestWatch_AStreamThatDropsKeepsItsReason(t *testing.T) {
	r := require.New(t)

	// The cluster drops the stream: the reason is sent and the channel is
	// closed right after it, so both are there to be read. Reading the
	// close first must not lose the reason.
	for range 50 {
		changes := newChanges()
		changes.errs <- errors.New("EOF")
		close(changes.c)

		client := &fakeClient{jobs: twoJobs(), changes: changes}

		m := watching(t, client)
		m, _ = m.update(m.watch.waitForChange()())

		r.Contains(m.flash.text, "EOF")
	}
}

func TestWatch_AClusterThatWillNotStreamIsSaidOnce(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), watchErr: errors.New("Permission denied")}

	m := newTestModel(client)
	m = drain(m, m.fetch())

	m, _ = m.update(m.watchScreen()())
	r.Contains(m.flash.text, "Permission denied")

	// Walking around a cluster that will not stream must not show the same
	// line again on every screen: nothing has changed since it was shown.
	m = m.quiet()

	m, _ = m.update(enter())
	m, _ = m.update(m.watchScreen()())

	r.Empty(m.flash.text)
}

func TestWatch_AStreamThatComesBackAndGoesAgainSaysSo(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), changes: newChanges()}

	m := watching(t, client)
	m = m.quiet()

	// The stream was up, so its going away is news again.
	m, _ = m.update(watchEndedMsg{id: m.watch.id, err: errors.New("EOF")})

	r.Contains(m.flash.text, "EOF")
}

func TestWatch_ASecondStreamForTheSameScreenClosesTheFirst(t *testing.T) {
	r := require.New(t)

	first, second := newChanges(), newChanges()
	client := &fakeClient{jobs: twoJobs(), changes: first}

	m := watching(t, client)

	// Two asks of the same screen both returned a stream: one is kept, the
	// other must not run for the rest of the session.
	m, _ = m.update(watchingMsg{id: m.watch.id, changes: second.stream()})

	r.True(first.closed)
	r.False(second.closed)
	r.True(m.watch.live())
}

func TestWatch_AChangeOfAStreamThatWasLeftAsksNothing(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), changes: newChanges()}

	m := watching(t, client)
	left := m.watch.id

	m, _ = m.update(enter())

	// The reader of the jobs stream was waiting when the screen was left.
	m, cmd := m.update(changeMsg{id: left})

	r.Nil(cmd)
	r.False(m.watch.settling)
}

func TestWatch_AClusterThatStreamsAgainIsNewsWhenItStops(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), watchErr: errors.New("Permission denied")}

	m := newTestModel(client)
	m = drain(m, m.fetch())
	m, _ = m.update(m.watchScreen()())
	m = m.quiet()

	// The next screen is streamed: the refusal is over.
	client.watchErr, client.changes = nil, newChanges()
	m, _ = m.update(enter())
	m, _ = m.update(m.watchScreen()())

	m, _ = m.update(watchEndedMsg{id: m.watch.id, err: errors.New("EOF")})

	r.Contains(m.flash.text, "EOF")
}
