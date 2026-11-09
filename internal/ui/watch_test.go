package ui

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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

func TestWatch_AScreenOpensItsStreamOnItsFirstAnswer(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		jobs: twoJobs(), allocs: twoAllocs(), changes: newChanges(),
		readIndex: map[string]uint64{nomad.TopicJob: 40, nomad.TopicAllocation: 90},
	}

	m := newTestModel(client)

	// Opening urga asks for the rows, and for no stream: the stream starts
	// after what the rows read.
	fire(t, m.Init())
	r.Zero(client.watches)

	m, cmd := m.update(m.fetch()())
	fire(t, cmd)

	r.Equal([]string{nomad.TopicJob}, client.watchedTopics)
	r.Equal(uint64(41), client.watchedFrom)

	m, cmd = m.update(enter())
	fire(t, cmd)
	r.Equal(1, client.watches)

	// Walking into the allocations watches allocations, from their own
	// read.
	_, cmd = m.update(m.fetch()())
	fire(t, cmd)

	r.Equal(2, client.watches)
	r.Equal([]string{nomad.TopicAllocation}, client.watchedTopics)
	r.Equal(uint64(91), client.watchedFrom)
}

func TestWatch_TheStreamStartsAfterTheLowestReadIndexOfItsTopics(t *testing.T) {
	r := require.New(t)

	w := watchState{read: map[string]uint64{nomad.TopicNode: 12, nomad.TopicAllocation: 900}}

	r.Equal(uint64(13), w.from([]string{nomad.TopicNode, nomad.TopicAllocation}))

	// A topic nothing read has no index to start after.
	w = watchState{read: map[string]uint64{nomad.TopicAllocation: 900}}

	r.Equal(uint64(0), w.from([]string{nomad.TopicAllocation, nomad.TopicNode}))
}

func TestWatch_AReadWithoutAnIndexStartsTheStreamWhereItAlwaysDid(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), changes: newChanges()}

	m := newTestModel(client)

	_, cmd := m.update(m.fetch()())
	fire(t, cmd)

	r.Equal(1, client.watches)
	r.Equal(uint64(0), client.watchedFrom)
}

func TestWatch_AFailedFirstReadStillOpensTheStream(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{err: errors.New("boom"), changes: newChanges()}

	m := newTestModel(client)

	_, cmd := m.update(m.fetch()())
	fire(t, cmd)

	r.Equal(1, client.watches)
	r.Equal(uint64(0), client.watchedFrom)
}

func TestWatch_ANamespaceSwitchBeforeTheAnswerOpensNoStreamInTheOldOne(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), changes: newChanges(), readIndex: map[string]uint64{nomad.TopicJob: 40}}

	m := newTestModel(client)
	m, _ = m.update(namespacesMsg(twoNamespaces()))

	read := m.fetch()

	m, cmd := m.update(key('2'))
	client.readIndex[nomad.TopicJob] = 60

	m, late := m.update(read())
	r.Nil(late)
	r.Zero(client.watches)

	playOut(m, cmd)

	r.Equal(1, client.watches)
	r.Equal("staging", client.watchedNamespace)
	r.Equal(uint64(61), client.watchedFrom)
}

func TestWatch_TheVersionArrivingAfterTheAnswerStartsAfterTheRead(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{variables: []nomad.Variable{{Path: "a/b"}}, changes: newChanges(), readIndex: map[string]uint64{nomad.TopicVariable: 30}}

	m, _ := sessionModel(t, client)
	m = typeCommand(m, "variables")
	r.Zero(client.watches)

	_, cmd := m.update(agentMsg{Version: "2.0.7"})
	fire(t, cmd)

	r.Equal(1, client.watches)
	r.Equal(uint64(31), client.watchedFrom)
}

func TestWatch_TheVersionArrivingBeforeTheAnswerOpensNoStreamYet(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{variables: []nomad.Variable{{Path: "a/b"}}, changes: newChanges(), readIndex: map[string]uint64{nomad.TopicVariable: 30}}

	m, _ := sessionModel(t, client)
	m, _ = m.update(key(':'))
	m = typeIn(m, "variables")
	m, _ = m.update(enter())

	// The version comes first: the stream waits for the read.
	m, cmd := m.update(agentMsg{Version: "2.0.7"})
	fire(t, cmd)
	r.Zero(client.watches)

	_, cmd = m.update(m.fetch()())
	fire(t, cmd)

	r.Equal(1, client.watches)
	r.Equal(uint64(31), client.watchedFrom)
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
	m, _ = m.update(pollMsg{tick: m.pollTick})

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
	m, _ = m.update(pollMsg{tick: m.pollTick})

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
	// the namespaces or the servers, is asked for on a timer.
	watched := map[*view][]string{
		jobsView:        {nomad.TopicJob},
		allocationsView: {nomad.TopicAllocation},
		deploymentsView: {nomad.TopicDeployment},
		evaluationsView: {nomad.TopicEvaluation},
		nodesView:       {nomad.TopicNode, nomad.TopicAllocation},
		servicesView:    {nomad.TopicService},
		nodePoolsView:   {nomad.TopicNodePool},
		volumesView:     {nomad.TopicCSIVolume, nomad.TopicHostVolume, nomad.TopicCSIPlugin},
		pluginsView:     {nomad.TopicCSIPlugin},
		variablesView:   {nomad.TopicVariable},
	}

	// Each is checked the way the program opens it, page and all.
	for v, topics := range watched {
		r.Equal(topics, v.open().topics(), nameOf(v))
	}

	for _, v := range []*view{namespacesView, serversView} {
		r.Empty(v.open().topics(), nameOf(v))
	}
}

func TestWatch_ThePagesOfStorageAndVariablesWatchTheirOwn(t *testing.T) {
	r := require.New(t)

	for _, c := range []struct {
		page   page
		topics []string
	}{
		{volumePage{kind: nomad.VolumeCSI}, []string{nomad.TopicCSIVolume, nomad.TopicCSIPlugin, nomad.TopicAllocation}},
		{volumePage{kind: nomad.VolumeHost}, []string{nomad.TopicHostVolume, nomad.TopicAllocation}},
		{pluginPage{id: "ebs"}, []string{nomad.TopicCSIPlugin}},
		{variablePage{path: "app/db"}, []string{nomad.TopicVariable}},
	} {
		r.Equal(c.topics, c.page.topics(), "%T", c.page)
	}

	// A variable is watched in its own namespace, whatever the session looks at.
	for _, session := range []string{"production", nomad.AllNamespaces} {
		client := &fakeClient{changes: newChanges()}
		m := newTestModel(client)
		m.nomadVersion = "2.0.7"
		m.namespace = session
		m.screen.page = variablePage{namespace: "staging", path: "app/db"}

		m.update(m.watchScreen()())
		r.Equal("staging", client.watchedNamespace, session)
	}
}

func TestWatch_AScreenOfOneObjectFollowsOnlyThatObject(t *testing.T) {
	r := require.New(t)

	const alloc, deployment, host = "af1f37df-7b19-6b1c-da67-5e8f482b5a15", "5d1a2b3c-0000-0000-0000-000000000000", "7f3c1a2b-0000-0000-0000-000000000000"

	watched := func(p page) map[string]string {
		client := &fakeClient{changes: newChanges()}
		m := newTestModel(client)
		m.nomadVersion = "2.0.7"
		m.screen.page = p

		if w, ok := m.watchScreen()().(watchingMsg); ok {
			w.changes.Close()
		}

		return client.watchedKeys
	}

	for _, c := range []struct {
		page page
		keys map[string]string
	}{
		{jobMetaPage{namespace: "staging", jobID: "web"}, map[string]string{nomad.TopicJob: "web"}},
		{scalingPage{namespace: "staging", jobID: "web", group: "api"}, map[string]string{nomad.TopicJob: "web"}},
		{tasksPage{namespace: "production", jobID: "web", allocID: alloc}, map[string]string{nomad.TopicAllocation: alloc}},
		{taskEventsPage{namespace: "production", allocID: alloc, task: "server"}, map[string]string{nomad.TopicAllocation: alloc}},
		{deploymentPage{namespace: "production", jobID: "web", deploymentID: deployment},
			map[string]string{nomad.TopicDeployment: deployment, nomad.TopicAllocation: deployment}},
		{deployWatchPage{namespace: "production", jobID: "web"}, map[string]string{
			nomad.TopicJob: "web", nomad.TopicEvaluation: "web", nomad.TopicDeployment: "web", nomad.TopicAllocation: "web",
		}},
		{variablePage{namespace: "staging", path: "app/db"}, map[string]string{nomad.TopicVariable: "app/db"}},
		{pluginPage{id: "ebs"}, map[string]string{nomad.TopicCSIPlugin: "ebs"}},
		// Nomad keys no allocation by its client or its volume, so these
		// follow every allocation.
		{clientPage{nodeID: "node-1"}, map[string]string{nomad.TopicNode: "node-1"}},
		{volumePage{namespace: "default", kind: nomad.VolumeCSI, id: "pg-data"}, map[string]string{nomad.TopicCSIVolume: "pg-data"}},
		{volumePage{namespace: "default", kind: nomad.VolumeHost, id: host}, map[string]string{nomad.TopicHostVolume: host}},
	} {
		r.Equal(c.keys, watched(c.page), "%T", c.page)
	}

	// A list follows every object of its topics.
	for _, v := range []*view{jobsView, allocationsView, nodesView, deploymentsView, variablesView, volumesView, pluginsView} {
		r.Nil(watched(v.open()), nameOf(v))
	}
}

func TestWatch_AClusterOlderThanTheTopicIsPolled(t *testing.T) {
	r := require.New(t)

	for version, topics := range map[string][]string{
		"1.11.3": nil,
		"":       nil,
		"2.0.7":  {nomad.TopicVariable},

		"1.11.4+ent": {nomad.TopicVariable},
		"1.11.3+ent": nil,
	} {
		client := &fakeClient{changes: newChanges()}
		m := newTestModel(client)
		m.nomadVersion = version
		m = typeCommand(m, "variables")

		r.Equal(topics, client.watchedTopics, version)

		if topics == nil {
			r.Equal(time.Millisecond, m.pollEvery(), version)
		}
	}
}

func TestWatch_TheVersionArrivingAfterTheScreenOpensItsStream(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{changes: newChanges()}
	m := typeCommand(newTestModel(client), "variables")
	r.Empty(client.watchedTopics)

	m, cmd := m.update(agentMsg{Version: "2.0.7"})
	drain(m, cmd)
	r.Equal([]string{nomad.TopicVariable}, client.watchedTopics)

	// The stream is open: another answer about the version does not open a second.
	m, _ = m.update(m.watchScreen()())
	r.True(m.watch.live())

	_, cmd = m.update(agentMsg{Version: "2.0.7"})
	r.Nil(cmd)

	// A screen of topics every cluster streams already asked on its own.
	client = &fakeClient{jobs: twoJobs(), changes: newChanges()}
	m = newTestModel(client)

	_, cmd = m.update(agentMsg{Version: "2.0.7"})
	r.Nil(cmd)
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

func TestWatch_ASecondStreamKeepsTheSlowTimerThatRuns(t *testing.T) {
	r := require.New(t)

	holdTimers(t)
	holdStreams(t)

	client := &fakeClient{jobs: twoJobs(), changes: newChanges()}

	m := watching(t, client)
	slow := m.pollTick

	m, cmd := m.update(watchingMsg{id: m.watch.id, changes: newChanges().stream()})
	r.Empty(timersOf(cmd))

	// The next poll comes when it was due.
	_, cmd = m.update(pollMsg{tick: slow})
	r.NotNil(cmd)
}

func TestWatch_AStreamThatOpensWhileThePollReadsLeavesTheTimerToTheAnswer(t *testing.T) {
	r := require.New(t)

	holdTimers(t)
	holdStreams(t)

	client := &fakeClient{jobs: twoJobs(), changes: newChanges()}

	m := newTestModel(client)
	m = drain(m, m.fetch())

	// The fast timer fires, and its read is out when the stream opens.
	m, read := m.update(pollMsg{tick: m.pollTick})
	r.NotNil(read)

	m, cmd := m.update(m.watchScreen()())
	r.Empty(timersOf(cmd))

	// The answer sets the next timer, at the pace of a live screen.
	m, cmd = m.update(read())
	r.Contains(timersOf(cmd), heldTimer{wait: slowPoll, msg: pollMsg{tick: m.pollTick}})
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

// timersOf are the timers among what a command asks for, with the stream
// waits and the other work held.
func timersOf(cmd tea.Cmd) []heldTimer {
	if cmd == nil {
		return nil
	}

	switch msg := cmd().(type) {
	case tea.BatchMsg:
		var timers []heldTimer
		for _, next := range msg {
			timers = append(timers, timersOf(next)...)
		}

		return timers
	case heldTimer:
		return []heldTimer{msg}
	}

	return nil
}

func TestWatch_AStreamThatOpensAfterTheAnswerSlowsThePoll(t *testing.T) {
	r := require.New(t)

	holdTimers(t)
	holdStreams(t)

	client := &fakeClient{jobs: twoJobs(), changes: newChanges()}

	m := newTestModel(client)
	m = drain(m, m.fetch())

	// The answer came first, so the timer it set is the fast one.
	fast := m.pollTick
	r.True(m.polling)

	calls := client.calls

	m, cmd := m.update(m.watchScreen()())
	r.Equal([]heldTimer{{wait: slowPoll, msg: pollMsg{tick: fast + 1}}}, timersOf(cmd))

	// The fast timer fires and asks for nothing.
	m, cmd = m.update(pollMsg{tick: fast})
	r.Nil(cmd)
	r.Equal(calls, client.calls)
}

func TestWatch_AnAnswerKeepsTheReadIndexOfWhatItRead(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), readIndex: map[string]uint64{nomad.TopicJob: 40}}

	m := newTestModel(client)
	m = drain(m, m.fetch())

	r.Equal(map[string]uint64{nomad.TopicJob: 40}, m.watch.read)
	r.Contains(plain(m.render()), "web")
}

func TestWatch_ALaterReadMovesTheReadIndexOn(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), readIndex: map[string]uint64{nomad.TopicJob: 40}}

	m := newTestModel(client)
	m = drain(m, m.fetch())

	client.readIndex[nomad.TopicJob] = 55
	m = drain(m, m.fetch())

	r.Equal(map[string]uint64{nomad.TopicJob: 55}, m.watch.read)
}

func TestWatch_TheAnswerThatArrivesLastSetsTheReadIndex(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), readIndex: map[string]uint64{nomad.TopicJob: 40}}

	m := newTestModel(client)
	older := m.fetch()()

	client.readIndex[nomad.TopicJob] = 55
	m = drain(m, m.fetch())

	// The page shows the answer that came last, though it was read first.
	m, _ = m.update(older)

	r.Equal(map[string]uint64{nomad.TopicJob: 40}, m.watch.read)
}

func TestWatch_AReadWithoutAnIndexKeepsNone(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs()}

	m := newTestModel(client)
	m = drain(m, m.fetch())

	r.Empty(m.watch.read)
}

func TestWatch_OnlyTheReadOfTheScreenKeepsItsReadIndex(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), readIndex: map[string]uint64{nomad.TopicJob: 40}}
	m := newTestModel(client)

	jobs := func() tea.Cmd {
		return fetchList(func(ctx context.Context) ([]nomad.Job, error) {
			return client.Jobs(ctx, "production")
		}, func(items []nomad.Job) tea.Msg { return jobsMsg(items) })
	}

	for name, labelled := range map[string]tea.Cmd{
		"asked": askedFor(m.asked, jobs()),
		"wrote": wroteFor(m.asked, jobs()),
	} {
		answer, ok := labelled().(answerMsg)
		r.True(ok, name)
		r.Nil(answer.read, name)
		r.Equal(jobsMsg(twoJobs()), answer.msg, name)
	}

	answer, ok := m.fetch()().(answerMsg)
	r.True(ok)
	r.Equal(map[string]uint64{nomad.TopicJob: 40}, answer.read)
	r.Equal(jobsMsg(twoJobs()), answer.msg)
}

func TestWatch_AnAnswerOfAScreenThatWasLeftKeepsNothing(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		jobs: twoJobs(), allocs: twoAllocs(),
		readIndex: map[string]uint64{nomad.TopicJob: 40, nomad.TopicAllocation: 90},
	}

	// The screen that is left was read at an index of its own.
	m := newTestModel(client)
	m = drain(m, m.fetch())
	old := m.fetch()

	m, _ = m.update(enter())
	m, _ = m.update(old())

	r.NotContains(m.watch.read, nomad.TopicJob)
}

// readOnce is a model whose screen was read once, at the read index of its
// topics, and whose stream is open. The test holds its timers and streams.
func readOnce(t *testing.T, client *fakeClient) Model {
	t.Helper()

	m := newTestModel(client)
	m, _ = m.update(m.fetch()())
	m, _ = m.update(m.watchScreen()())

	return m
}

func jobChange(m Model, index uint64) changeMsg {
	return changeMsg{id: m.watch.id, change: nomad.Change{Topic: nomad.TopicJob, Index: index}}
}

func TestWatch_AScreenIsReadOnceWhenItOpens(t *testing.T) {
	r := require.New(t)

	holdTimers(t)
	holdStreams(t)

	client := &fakeClient{jobs: twoJobs(), changes: newChanges(), readIndex: map[string]uint64{nomad.TopicJob: 40}}

	m := readOnce(t, client)

	r.Equal(1, client.calls)
	r.Equal(uint64(41), client.watchedFrom)

	// The cluster sends its newest event again: the screen read it already.
	m, cmd := m.update(jobChange(m, 40))
	r.Empty(timersOf(cmd))
	r.False(m.watch.settling)

	m, cmd = m.update(settleMsg{})
	r.Nil(cmd)
	r.Equal(1, client.calls)
}

func TestWatch_TheJobsListIsReadOnceWhenTheCountsAnswerAtALowerIndex(t *testing.T) {
	r := require.New(t)

	holdTimers(t)
	holdStreams(t)

	var lists atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch req.URL.Path {
		case "/v1/jobs":
			lists.Add(1)
			w.Header().Set("X-Nomad-Index", "50")
			_, _ = w.Write([]byte(`[{"ID": "web", "Name": "web", "Namespace": "production", "Type": "service"}]`))
		case "/v1/jobs/statuses":
			// The newest change of the services on the page, which is
			// older than a change of another job.
			w.Header().Set("X-Nomad-Index", "45")
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, req)
		}
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	r.NoError(err)

	m := newTestModel(client)
	m, _ = m.update(m.fetch()())

	// A change the list shows does not read it again.
	m, cmd := m.update(jobChange(m, 48))
	r.Empty(timersOf(cmd))
	r.False(m.watch.settling)
	r.Equal(int32(1), lists.Load())
}

func TestWatch_TheClientListIsReadOnceWhenItOpens(t *testing.T) {
	r := require.New(t)

	holdTimers(t)
	holdStreams(t)

	client := &fakeClient{nodes: readyNode(), changes: newChanges(), readIndex: map[string]uint64{nomad.TopicNode: 12}}

	m := newTestModel(client)
	m, _ = m.update(key(':'))
	m = typeIn(m, "nodes")
	m, _ = m.update(enter())
	m, cmd := m.update(m.fetch()())
	fire(t, cmd)

	r.Equal([]string{nomad.TopicNode, nomad.TopicAllocation}, client.watchedTopics)
	r.Equal(uint64(13), client.watchedFrom)

	// No client has a limit, so no allocation shows on the list: one far
	// newer than the clients reads nothing.
	m, cmd = m.update(changeMsg{id: m.watch.id, change: nomad.Change{Topic: nomad.TopicAllocation, Index: 900}})
	r.Empty(timersOf(cmd))
	r.False(m.watch.settling)

	// The cluster sends the client it had at the time of the read again.
	m, cmd = m.update(changeMsg{id: m.watch.id, change: nomad.Change{Topic: nomad.TopicNode, Index: 12}})
	r.Empty(timersOf(cmd))
	r.False(m.watch.settling)

	// A newer one reads the list again.
	m, _ = m.update(changeMsg{id: m.watch.id, change: nomad.Change{Topic: nomad.TopicNode, Index: 13}})
	r.True(m.watch.settling)
}

func TestWatch_AnAllocationChangeReadsAgainAListWithALimitedClient(t *testing.T) {
	r := require.New(t)

	holdTimers(t)
	holdStreams(t)

	client := limitedCluster(3, clientAllocs())
	client.changes = newChanges()
	client.readIndex = map[string]uint64{nomad.TopicNode: 12, nomad.TopicAllocation: 30}

	m := newTestModel(client)
	m, _ = m.update(key(':'))
	m = typeIn(m, "nodes")
	m, _ = m.update(enter())
	m, _ = m.update(m.fetch()())

	// The list counts the allocations of the client with a limit.
	r.Equal(map[string]uint64{nomad.TopicNode: 12, nomad.TopicAllocation: 30}, m.watch.read)

	m, cmd := m.update(changeMsg{id: m.watch.id, change: nomad.Change{Topic: nomad.TopicAllocation, Index: 31}})
	r.Equal([]heldTimer{{wait: settle, msg: settleMsg{}}}, timersOf(cmd))
	r.True(m.watch.settling)

	asked := len(client.lists["nodes"])

	m, cmd = m.update(settleMsg{})
	drain(m, cmd)

	r.Len(client.lists["nodes"], asked+1)
}

func TestWatch_AClientThatGetsALimitReadsTheListOnce(t *testing.T) {
	r := require.New(t)

	holdTimers(t)
	holdStreams(t)

	client := &fakeClient{
		nodes: readyNode(), changes: newChanges(),
		readIndex: map[string]uint64{nomad.TopicNode: 12, nomad.TopicAllocation: 30},
	}

	m := newTestModel(client)
	m, _ = m.update(key(':'))
	m = typeIn(m, "nodes")
	m, _ = m.update(enter())
	m, _ = m.update(m.fetch()())
	r.Equal(map[string]uint64{nomad.TopicNode: 12, nomad.TopicAllocation: nomad.EveryIndex}, m.watch.read)

	// The limit is a change of the client.
	client.nodes[0].MaxAllocs = 3
	client.readIndex[nomad.TopicNode] = 13

	m, _ = m.update(changeMsg{id: m.watch.id, change: nomad.Change{Topic: nomad.TopicNode, Index: 13}})
	m, read := m.update(settleMsg{})
	r.NotNil(read)

	// That read counts the allocations. The list before showed none of
	// them, so it is not newer than this one.
	m, cmd := m.update(read())
	r.Equal(map[string]uint64{nomad.TopicNode: 13, nomad.TopicAllocation: 30}, m.watch.read)
	r.NotContains(timersOf(cmd), heldTimer{wait: settle, msg: settleMsg{}})
	r.False(m.watch.settling)
	r.Len(client.lists["nodes"], 2)
}

func TestWatch_AChangeBetweenAReadAndItsAnswerReadsAgain(t *testing.T) {
	r := require.New(t)

	holdTimers(t)
	holdStreams(t)

	client := &fakeClient{
		nodes: readyNode(), changes: newChanges(),
		readIndex: map[string]uint64{nomad.TopicNode: 12, nomad.TopicAllocation: 30},
	}

	m := newTestModel(client)
	m, _ = m.update(key(':'))
	m = typeIn(m, "nodes")
	m, _ = m.update(enter())
	m, _ = m.update(m.fetch()())

	client.nodes[0].MaxAllocs = 3
	client.readIndex[nomad.TopicNode] = 13

	m, _ = m.update(changeMsg{id: m.watch.id, change: nomad.Change{Topic: nomad.TopicNode, Index: 13}})
	m, read := m.update(settleMsg{})
	answer := read()

	// An allocation changes after the read counted them, and the stream
	// reports it before the answer comes. The list still shows none.
	m, _ = m.update(changeMsg{id: m.watch.id, change: nomad.Change{Topic: nomad.TopicAllocation, Index: 31}})

	m, cmd := m.update(answer)
	r.Contains(timersOf(cmd), heldTimer{wait: settle, msg: settleMsg{}})
	r.True(m.watch.settling)

	m, cmd = m.update(settleMsg{})
	drain(m, cmd)

	r.Len(client.lists["nodes"], 3)
}

func TestWatch_AStreamThatSentOnlyWhatTheScreenReadWasNotRefused(t *testing.T) {
	r := require.New(t)

	holdTimers(t)
	holdStreams(t)

	client := &fakeClient{nodes: readyNode(), changes: newChanges(), readIndex: map[string]uint64{nomad.TopicNode: 12}}

	m := newTestModel(client)
	m, _ = m.update(key(':'))
	m = typeIn(m, "nodes")
	m, _ = m.update(enter())
	m, _ = m.update(m.fetch()())
	m, _ = m.update(m.watchScreen()())
	r.True(m.watch.live())

	// The stream sends the client the list read already, and ends at once.
	m, _ = m.update(changeMsg{id: m.watch.id, change: nomad.Change{Topic: nomad.TopicNode, Index: 12}})
	m, _ = m.update(watchEndedMsg{id: m.watch.id, err: errors.New("stream dropped")})

	// It sent an event, so the cluster did not refuse it: the list warns
	// and does not fall back to the clients alone.
	r.False(m.watch.fewer)
	r.Contains(m.flash.text, "stream dropped")
}

func TestWatch_AChangeNewerThanTheReadReadsAgain(t *testing.T) {
	r := require.New(t)

	holdTimers(t)
	holdStreams(t)

	client := &fakeClient{jobs: twoJobs(), changes: newChanges(), readIndex: map[string]uint64{nomad.TopicJob: 40}}

	m := readOnce(t, client)

	m, cmd := m.update(jobChange(m, 41))
	r.Equal([]heldTimer{{wait: settle, msg: settleMsg{}}}, timersOf(cmd))
	r.True(m.watch.settling)

	m, cmd = m.update(settleMsg{})
	r.NotNil(cmd)

	drain(m, cmd)
	r.Equal(2, client.calls)
}

func TestWatch_ABurstTheNextAnswerAlreadyShowsReadsNothing(t *testing.T) {
	r := require.New(t)

	holdTimers(t)
	holdStreams(t)

	client := &fakeClient{jobs: twoJobs(), changes: newChanges(), readIndex: map[string]uint64{nomad.TopicJob: 40}}

	m := readOnce(t, client)

	m, _ = m.update(jobChange(m, 41))

	m, read := m.update(settleMsg{})
	r.NotNil(read)

	// A change comes while the read is out, and the read already shows it.
	m, cmd := m.update(jobChange(m, 44))
	r.NotEmpty(timersOf(cmd))

	client.readIndex[nomad.TopicJob] = 45
	m = drain(m, read)
	r.Equal(map[string]uint64{nomad.TopicJob: 45}, m.watch.read)

	calls := client.calls

	m, cmd = m.update(settleMsg{})
	r.Nil(cmd)
	r.Equal(calls, client.calls)
}

func TestWatch_AChangeOfATopicNotReadYetReadsAgain(t *testing.T) {
	r := require.New(t)

	holdTimers(t)
	holdStreams(t)

	client := &fakeClient{
		nodes: readyNode(), changes: newChanges(),
		readIndex: map[string]uint64{nomad.TopicAllocation: 10},
	}

	m, _ := nodeModelOf(client)
	m, _ = m.update(enter())
	m = drain(m, m.fetch())

	r.IsType(clientPage{}, m.screen.page)
	r.Equal(map[string]uint64{nomad.TopicAllocation: 10}, m.watch.read)

	m, _ = m.update(changeMsg{id: m.watch.id, change: nomad.Change{Topic: nomad.TopicNode, Index: 5}})
	r.True(m.watch.settling)

	_, cmd := m.update(settleMsg{})
	r.NotNil(cmd)
}

func TestWatch_ASettleLeftFromTheScreenBeforeReadsNothing(t *testing.T) {
	r := require.New(t)

	holdTimers(t)
	holdStreams(t)

	client := &fakeClient{
		jobs: twoJobs(), allocs: twoAllocs(), changes: newChanges(),
		readIndex: map[string]uint64{nomad.TopicJob: 40},
	}

	m := readOnce(t, client)
	m, _ = m.update(jobChange(m, 41))
	r.True(m.watch.settling)

	m, _ = m.update(enter())

	m, cmd := m.update(settleMsg{})
	r.Nil(cmd)
}

func TestWatch_ABurstOfTheNextScreenStartsATimerOfItsOwn(t *testing.T) {
	r := require.New(t)

	holdTimers(t)
	holdStreams(t)

	client := &fakeClient{
		jobs: twoJobs(), allocs: twoAllocs(), changes: newChanges(),
		readIndex: map[string]uint64{nomad.TopicJob: 40, nomad.TopicAllocation: 90},
	}

	m := readOnce(t, client)
	m, _ = m.update(jobChange(m, 41))
	r.True(m.watch.settling)

	// The settle of the jobs is still to come when the allocations open.
	m, _ = m.update(enter())
	m, _ = m.update(m.fetch()())
	m, _ = m.update(m.watchScreen()())

	m, cmd := m.update(changeMsg{id: m.watch.id, change: nomad.Change{Topic: nomad.TopicAllocation, Index: 91}})
	r.Contains(timersOf(cmd), heldTimer{wait: settle, msg: settleMsg{}})
	r.True(m.watch.settling)
}

func TestWatch_AChangeAnOlderAnswerLeftOutReadsAgain(t *testing.T) {
	r := require.New(t)

	holdTimers(t)
	holdStreams(t)

	client := &fakeClient{jobs: twoJobs(), changes: newChanges(), readIndex: map[string]uint64{nomad.TopicJob: 55}}

	m := readOnce(t, client)

	// The page shows the change already.
	m, cmd := m.update(jobChange(m, 50))
	r.Empty(timersOf(cmd))

	// An answer read before the change arrives last.
	client.readIndex[nomad.TopicJob] = 45
	m, cmd = m.update(m.fetch()())
	r.Contains(timersOf(cmd), heldTimer{wait: settle, msg: settleMsg{}})
	r.True(m.watch.settling)

	calls := client.calls

	client.readIndex[nomad.TopicJob] = 55
	m, cmd = m.update(settleMsg{})
	m = drain(m, cmd)

	r.Equal(calls+1, client.calls)
	r.Equal(map[string]uint64{nomad.TopicJob: 55}, m.watch.read)
}

func TestWatch_AnAnswerOlderThanThePageReadsAgain(t *testing.T) {
	r := require.New(t)

	holdTimers(t)
	holdStreams(t)

	client := &fakeClient{jobs: twoJobs(), changes: newChanges(), readIndex: map[string]uint64{nomad.TopicJob: 60}}

	m := readOnce(t, client)
	r.Equal(uint64(61), client.watchedFrom)

	// The stream never sends what changed up to 60, and the page goes back
	// to 45.
	client.readIndex[nomad.TopicJob] = 45
	m, cmd := m.update(m.fetch()())
	r.Contains(timersOf(cmd), heldTimer{wait: settle, msg: settleMsg{}})

	calls := client.calls

	client.readIndex[nomad.TopicJob] = 60
	m, cmd = m.update(settleMsg{})
	m = drain(m, cmd)

	r.Equal(calls+1, client.calls)
	r.Equal(map[string]uint64{nomad.TopicJob: 60}, m.watch.read)
	r.False(m.watch.settling)
}

func TestWatch_AScreenOfOneObjectReadsOnceForAChangeOfAnother(t *testing.T) {
	r := require.New(t)

	holdTimers(t)
	holdStreams(t)

	// A read of one client is at the index it last changed at.
	client := &fakeClient{
		nodes: readyNode(), changes: newChanges(),
		readIndex: map[string]uint64{nomad.TopicNode: 30, nomad.TopicAllocation: 40},
	}

	m, _ := nodeModelOf(client)
	m, _ = m.update(enter())
	m = drain(m, m.fetch())
	r.IsType(clientPage{}, m.screen.page)

	// Another client changes: the screen reads its own again, still at 30,
	// and stops there.
	m, _ = m.update(changeMsg{id: m.watch.id, change: nomad.Change{Topic: nomad.TopicNode, Index: 50}})
	m, cmd := m.update(settleMsg{})
	r.NotNil(cmd)

	m = drain(m, cmd)

	r.Equal(map[string]uint64{nomad.TopicNode: 30, nomad.TopicAllocation: 40}, m.watch.read)
	r.False(m.watch.settling)
}

func TestWatch_TheNextBurstReadsOnlyForItsOwnChanges(t *testing.T) {
	r := require.New(t)

	holdTimers(t)
	holdStreams(t)

	client := &fakeClient{
		nodes: readyNode(), changes: newChanges(),
		readIndex: map[string]uint64{nomad.TopicNode: 30, nomad.TopicAllocation: 40},
	}

	m, _ := nodeModelOf(client)
	m, _ = m.update(enter())
	m = drain(m, m.fetch())
	r.IsType(clientPage{}, m.screen.page)

	// Another client changes: the screen reads its own again, still at 30.
	m, _ = m.update(changeMsg{id: m.watch.id, change: nomad.Change{Topic: nomad.TopicNode, Index: 50}})
	m, cmd := m.update(settleMsg{})
	m = drain(m, cmd)

	// An allocation changes, and an answer shows it before the burst settles.
	m, _ = m.update(changeMsg{id: m.watch.id, change: nomad.Change{Topic: nomad.TopicAllocation, Index: 41}})
	r.True(m.watch.settling)

	client.readIndex[nomad.TopicAllocation] = 45
	m = drain(m, m.fetch())

	_, cmd = m.update(settleMsg{})
	r.Nil(cmd)
}
