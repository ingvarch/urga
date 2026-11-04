package ui

import (
	"errors"
	"io"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func nodeModel(t *testing.T, nodes []nomad.Node) (Model, *fakeClient) {
	t.Helper()

	return nodeModelOf(&fakeClient{nodes: nodes})
}

// nodeModelOf opens the clients screen on the given fake cluster.
func nodeModelOf(client *fakeClient) (Model, *fakeClient) {
	nodes := client.nodes

	m := newTestModel(client)
	m, _ = m.update(key(':'))
	m = typeIn(m, "nodes")
	m, _ = m.update(enter())
	m, _ = m.update(nodesMsg(nodes))

	return m, client
}

func readyNode() []nomad.Node {
	return []nomad.Node{{ID: "node-1", Name: "server-01", Status: "ready", Eligibility: "eligible"}}
}

// keyNames are the keys the header offers, each as its key and label, in
// header order.
func keyNames(m Model) []string {
	out := []string{}
	for _, b := range m.keys() {
		out = append(out, b.press+" "+b.label)
	}

	return out
}

func TestClients_TheList(t *testing.T) {
	r := require.New(t)

	m, _ := nodeModel(t, busyClient())

	r.Contains(plain(m.render()), "Clients [1]")
	r.Equal([]string{"ID", "Name", "Datacenter", "Pool", "Version", "Status", "Eligibility", "Drain", "CPU", "MEM", "Address"}, m.screen.page.titles())
	r.Equal([]string{nomad.TopicNode, nomad.TopicAllocation}, m.screen.page.topics())

	// The CPU and memory use of the machine is unknown until it is read.
	r.Regexp(`^\s*node-1\s+nomad-server-01\s+dc1\s+default\s+1\.11\.1\s+ready\s+eligible\s+false\s+-\s+-\s+10\.0\.0\.2`, fileRow(t, m, 0))

	r.Equal([]string{"enter Allocations", "ctrl+l Agent Logs", "ctrl+d Drain", "i Toggle Eligibility", "ctrl+g GC", "space Mark", "ctrl+a Mark All"}, keyNames(m))
}

func TestClients_FollowTheWorkOfEveryNamespace(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{nodes: busyClient(), changes: newChanges()}

	m, _ := nodeModelOf(client)
	r.Equal("production", m.namespace)

	m.update(m.watchScreen()())

	r.Equal(nomad.AllNamespaces, client.watchedNamespace)
	r.Equal([]string{nomad.TopicNode, nomad.TopicAllocation}, client.watchedTopics)
}

// endedAtOnce is a stream that ends as soon as it opens, with no event: how
// Nomad 2.0 refuses one.
func endedAtOnce() *nomad.Changes {
	ended := newChanges()
	ended.errs <- io.EOF
	close(ended.c)

	return ended.stream()
}

// refusals are the two ways a cluster refuses a stream: a 403, or a stream
// that ends at once.
func refusals(t *testing.T) map[string]func() (*nomad.Changes, error) {
	t.Helper()

	return map[string]func() (*nomad.Changes, error){
		"refused":       func() (*nomad.Changes, error) { return nil, forbidden(t) },
		"ended at once": func() (*nomad.Changes, error) { return endedAtOnce(), nil },
	}
}

// throughRefusal runs the answer to an ask for a stream through the model. A
// stream that opened is read first, to its end.
func throughRefusal(m Model, msg tea.Msg) (Model, tea.Cmd) {
	if opened, ok := msg.(watchingMsg); ok {
		var read tea.Cmd
		m, read = m.update(opened)
		msg = read()
	}

	return m.update(msg)
}

func TestClients_ARefusedStreamOfTheWorkFollowsTheClientsAlone(t *testing.T) {
	for name, refuse := range refusals(t) {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			var asked [][]string

			client := &fakeClient{jobs: twoJobs(), nodes: busyClient(), changes: newChanges()}

			// The jobs screen streams, so a refusal shown on the client list
			// would be a warning. It hears a change, which the stream of the
			// list does not inherit.
			m := watching(t, client)
			m, _ = m.update(changeMsg{id: m.watch.id})

			client.streamOf = func(_ string, topics []string) (*nomad.Changes, error) {
				asked = append(asked, topics)
				if slices.Contains(topics, nomad.TopicAllocation) {
					return refuse()
				}

				return newChanges().stream(), nil
			}

			m, _ = m.update(key(':'))
			m = typeIn(m, "nodes")
			m, _ = m.update(enter())
			m, _ = m.update(nodesMsg(busyClient()))
			tick := m.pollTick

			msg := m.watchScreen()()
			r.Equal([]string{nomad.TopicNode, nomad.TopicAllocation}, client.watchedTopics)

			m, cmd := throughRefusal(m, msg)
			r.NotNil(cmd)
			m, _ = m.update(cmd())

			// The list follows what the token may read: the clients, with no
			// warning and no faster poll.
			r.Equal([][]string{{nomad.TopicNode, nomad.TopicAllocation}, {nomad.TopicNode}}, asked)
			r.Equal(nomad.AllNamespaces, client.watchedNamespace)
			r.True(m.watch.live())
			r.Empty(m.flash.text)
			r.Equal(slowPoll, m.pollEvery())
			r.Equal(tick, m.pollTick)
		})
	}
}

func TestClients_ARefusalOfTheClientsAloneSaysSo(t *testing.T) {
	for name, refuse := range refusals(t) {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			client := &fakeClient{nodes: busyClient()}
			client.streamOf = func(string, []string) (*nomad.Changes, error) { return refuse() }

			m, _ := nodeModelOf(client)

			m, cmd := throughRefusal(m, m.watchScreen()())
			r.NotNil(cmd)
			m, _ = throughRefusal(m, cmd())

			// Refused the clients too, the list is polled the way it was.
			r.Equal([]string{nomad.TopicNode}, client.watchedTopics)
			r.False(m.watch.live())
			r.Contains(m.flash.text, "asking every")
			r.Equal(time.Millisecond, m.pollEvery())
		})
	}
}

func TestClients_AStreamOfTheWorkThatDropsLaterSaysSo(t *testing.T) {
	for name, before := range map[string]func(Model) Model{
		"after a change": func(m Model) Model {
			m, _ = m.update(changeMsg{id: m.watch.id})

			return m
		},
		"after a while": func(m Model) Model {
			m.watch.opened = m.watch.opened.Add(-refusedWithin)

			return m
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			client := &fakeClient{nodes: busyClient(), changes: newChanges()}

			m, _ := nodeModelOf(client)
			m, _ = m.update(m.watchScreen()())
			r.True(m.watch.live())

			m = before(m)
			m, _ = m.update(watchEndedMsg{id: m.watch.id, err: io.EOF})

			// A stream that worked and dropped is not a refusal: the list does
			// not ask for the clients alone.
			r.Equal([]string{nomad.TopicNode, nomad.TopicAllocation}, client.watchedTopics)
			r.False(m.watch.live())
			r.Contains(m.flash.text, "EOF")
			r.Equal(time.Millisecond, m.pollEvery())
		})
	}
}

func TestClients_OpenedAgainAsksForTheWorkAgain(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), nodes: busyClient()}
	client.streamOf = func(_ string, topics []string) (*nomad.Changes, error) {
		if slices.Contains(topics, nomad.TopicAllocation) {
			return nil, forbidden(t)
		}

		return newChanges().stream(), nil
	}

	m, _ := nodeModelOf(client)
	m, cmd := throughRefusal(m, m.watchScreen()())
	m, _ = m.update(cmd())
	r.Equal([]string{nomad.TopicNode}, client.watchedTopics)

	// The token may be another by the time the list opens again.
	m, _ = m.update(escape())
	m, _ = m.update(key(':'))
	m = typeIn(m, "nodes")
	m, _ = m.update(enter())

	m.update(m.watchScreen()())
	r.Equal([]string{nomad.TopicNode, nomad.TopicAllocation}, client.watchedTopics)
}

func TestClients_OnlyThoseThatTakeNoWork(t *testing.T) {
	r := require.New(t)

	nodes := []nomad.Node{
		{ID: "node-1", Name: "server-01", Status: "ready", Eligibility: "eligible"},
		{ID: "node-2", Name: "server-02", Status: "down", Eligibility: "eligible"},
		{ID: "node-3", Name: "server-03", Status: "ready", Eligibility: "ineligible", Drain: true},
	}

	m, _ := nodeModel(t, nodes)
	m, _ = m.update(key('!'))

	// One that is down and one that is draining; the one that works is
	// left out.
	out := plain(m.render())
	r.NotContains(out, "server-01")
	r.Contains(out, "server-02")
	r.Contains(out, "server-03")
	r.Contains(out, "2 of 3")

	// Enter opens the first one that is left.
	m, _ = m.update(enter())
	r.Contains(plain(m.render()), "Client server-02")
}

func TestNode_Drain(t *testing.T) {
	r := require.New(t)

	m, client := nodeModel(t, readyNode())

	m, _ = m.update(ctrlKey('d'))
	r.Contains(plain(m.render()), "drain the client server-01")

	_, cmd := answerYes(m)
	drain(m, cmd)

	r.True(client.drained)
	r.Equal("node-1", client.askedID)
}

func TestNode_StopDraining(t *testing.T) {
	r := require.New(t)

	draining := []nomad.Node{{ID: "node-1", Name: "server-01", Status: "ready", Eligibility: "ineligible", Drain: true}}

	m, client := nodeModel(t, draining)

	m, _ = m.update(ctrlKey('d'))

	// A client that is already draining is asked to stop, not to start again.
	r.Contains(plain(m.render()), "stop draining the client server-01")

	_, cmd := answerYes(m)
	drain(m, cmd)

	r.False(client.drained)
	r.Equal(1, client.drainCalls)
}

func TestNode_Eligibility(t *testing.T) {
	r := require.New(t)

	m, client := nodeModel(t, readyNode())

	m, _ = m.update(key('i'))
	r.Contains(plain(m.render()), "take new work from the client server-01")

	_, cmd := answerYes(m)
	drain(m, cmd)

	r.False(client.eligible)
	r.Equal(1, client.eligibleCalls)
}

func TestDeployment_Promote(t *testing.T) {
	r := require.New(t)

	deployments := []nomad.Deployment{{ID: "dep-1", JobID: "web", Namespace: "production", Status: "running"}}

	client := &fakeClient{deployments: deployments}
	m := newTestModel(client)
	m, _ = m.update(key(':'))
	m = typeIn(m, "dp")
	m, _ = m.update(enter())
	m, _ = m.update(deploymentsMsg(deployments))

	m, _ = m.update(key('p'))
	r.Contains(plain(m.render()), "promote the canaries")

	m, cmd := answerYes(m)
	m = drain(m, cmd)

	r.Equal(1, client.promoted)
	r.Contains(plain(m.render()), "promoted")
}

func TestDeployment_Fail(t *testing.T) {
	r := require.New(t)

	deployments := []nomad.Deployment{{ID: "dep-1", JobID: "web", Namespace: "production", Status: "running"}}

	client := &fakeClient{deployments: deployments}
	m := newTestModel(client)
	m, _ = m.update(key(':'))
	m = typeIn(m, "dp")
	m, _ = m.update(enter())
	m, _ = m.update(deploymentsMsg(deployments))

	m, _ = m.update(key('f'))
	_, cmd := answerYes(m)
	drain(m, cmd)

	r.Equal(1, client.failed)
}

func TestNode_StoppingADrainMovesNothing(t *testing.T) {
	r := require.New(t)

	draining := []nomad.Node{{ID: "node-1", Name: "server-01", Status: "ready", Drain: true}}

	m, _ := nodeModel(t, draining)
	m, _ = m.update(ctrlKey('d'))

	out := plain(m.render())

	// Stopping a drain moves nothing; it stops allocations being moved.
	r.Contains(out, "stop draining the client server-01")
	r.NotContains(out, "move elsewhere")
}

func TestNode_DrainingSaysWhereTheWorkGoes(t *testing.T) {
	r := require.New(t)

	m, _ := nodeModel(t, readyNode())
	m, _ = m.update(ctrlKey('d'))

	r.Contains(plain(m.render()), "move elsewhere")
}

func TestNode_WhatWentWrongReadsAsASentence(t *testing.T) {
	r := require.New(t)

	nodes := []nomad.Node{
		{ID: "node-1", Name: "server-01", Status: "ready", Eligibility: "eligible"},
		{ID: "node-2", Name: "server-02", Status: "ready", Eligibility: "eligible"},
	}

	client := &fakeClient{nodes: nodes, actionErr: errors.New("Permission denied")}

	m, _ := nodeModelOf(client)
	m, _ = m.update(ctrlKey('a'))
	m, _ = m.update(key('i'))

	m, cmd := answerYes(m)
	m = drain(m, cmd)

	// How far an action got is a sentence about the action, not a noun
	// phrase built for a different one.
	r.Contains(plain(m.render()), "took new work from 0 of 2")
}
