package ui

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// onStoppedJob is the list of jobs, the cursor on cron, which is dead.
func onStoppedJob(t *testing.T, client *fakeClient) Model {
	t.Helper()

	client.jobs = twoJobs()

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(key('j'))

	return m
}

func TestPurge_AStoppedJob(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onStoppedJob(t, client)

	m, _ = m.update(ctrlKey('p'))
	r.Equal("Really purge the job cron? Its versions and history go with it.", m.confirm.question)

	m, cmd := answerYes(m)
	m = drain(m, cmd)

	r.Equal([]string{"cron"}, client.purged)
	r.Equal("production", client.askedNamespace)
	r.Contains(plain(m.render()), "Purged the job cron.")
}

func TestPurge_NotAJobThatRuns(t *testing.T) {
	r := require.New(t)

	m := onStoppedJob(t, &fakeClient{})
	r.True(offers(m, "ctrl-p"))

	m, _ = m.update(key('k'))
	r.False(offers(m, "ctrl-p"))
}

func TestPurge_OnlyTheStoppedOfTheMarked(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onStoppedJob(t, client)
	m, _ = m.update(ctrlKey('a'))

	m, _ = m.update(ctrlKey('p'))
	r.Equal("Really purge the job cron? Its versions and history go with it.", m.confirm.question)

	m, cmd := answerYes(m)
	drain(m, cmd)

	r.Equal([]string{"cron"}, client.purged)
}

// upAndDown are a client that runs, one that is down, and another that
// runs.
func upAndDown() []nomad.Node {
	return []nomad.Node{
		{ID: "node-1", Name: "server-01", Status: "ready", Eligibility: "eligible"},
		{ID: "node-2", Name: "server-02", Status: "down", Eligibility: "eligible"},
		{ID: "node-3", Name: "server-03", Status: "ready", Eligibility: "eligible"},
	}
}

// onClients is the list of clients, one that runs and one that is down.
func onClients(t *testing.T, client *fakeClient) Model {
	t.Helper()

	client.nodes = upAndDown()

	m := newTestModel(client)
	m, _ = m.show(nodesView)
	m, _ = m.update(nodesMsg(upAndDown()))

	return m
}

func TestPurge_AClientThatIsDown(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onClients(t, client)
	m, _ = m.update(key('j'))

	// Its garbage cannot be collected: nothing answers there.
	r.True(offers(m, "ctrl-p"))
	r.False(offers(m, "ctrl-g"))

	m, _ = m.update(ctrlKey('p'))
	r.Equal("Really purge the client server-02? The cluster forgets it until it registers again.", m.confirm.question)

	m, cmd := answerYes(m)
	m = drain(m, cmd)

	r.Equal([]string{"node-2"}, client.purged)
	r.Contains(plain(m.render()), "Purged the client server-02.")
}

func TestCollect_TheGarbageOfAClientThatRuns(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onClients(t, client)

	r.True(offers(m, "ctrl-g"))
	r.False(offers(m, "ctrl-p"))

	m, _ = m.update(ctrlKey('g'))
	r.Equal("Really collect the garbage of the client server-01? Logs of its ended allocations are deleted.", m.confirm.question)

	m, cmd := answerYes(m)
	m = drain(m, cmd)

	r.Equal([]string{"node-1"}, client.collected)
	r.Contains(plain(m.render()), "Collected the garbage of the client server-01.")
}

func TestPurge_OnlyTheClientsOfTheMarkedThatAreDown(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onClients(t, client)
	m, _ = m.update(key('j'))
	m, _ = m.update(ctrlKey('a'))

	m, _ = m.update(ctrlKey('p'))
	m, cmd := answerYes(m)
	drain(m, cmd)

	r.Equal([]string{"node-2"}, client.purged)
}

func TestGC_CollectsTheGarbageOfTheCluster(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := newTestModel(client)

	m, _ = runLine(m, "gc")
	// A question is one line: it says what goes, in words that fit.
	r.Equal("Really collect the garbage of the cluster? Dead jobs, ended allocations and down clients go now.", m.confirm.question)

	m, cmd := answerYes(m)
	m = drain(m, cmd)

	r.Equal([]string{"CollectGarbage"}, client.writes)
	r.Contains(plain(m.render()), "Garbage of the cluster collected.")
}

func TestGC_NotInReadOnlyMode(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := newTestModel(client)
	m.opts.ReadOnly = true

	m, _ = runLine(m, "gc")

	r.Equal(overlayNone, m.overlay)
	r.Empty(client.writes)
	r.Contains(plain(m.render()), "Read-only: :gc changes the cluster.")
}

func TestCollect_TheGarbageOfTheMarkedClientsThatRun(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onClients(t, client)
	m, _ = m.update(ctrlKey('a'))

	// The one that is down answers nothing, and is left out.
	m, _ = m.update(ctrlKey('g'))
	r.Equal("Really collect the garbage of 2 clients? Logs of their ended allocations are deleted.", m.confirm.question)

	m, cmd := answerYes(m)
	drain(m, cmd)

	r.Equal([]string{"node-1", "node-3"}, client.collected)
}
