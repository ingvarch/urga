package ui

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func nodeModel(t *testing.T, nodes []nomad.Node) (Model, *fakeClient) {
	t.Helper()

	return nodeModelOf(&fakeClient{nodes: nodes})
}

// nodeModelOf opens the clients screen on a cluster of its own.
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

// keyNames are the keys the header offers, by what is pressed and what it is
// called, in its order.
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
	r.Equal([]string{"ID", "Name", "Datacenter", "Pool", "Version", "Status", "Eligibility", "Drain", "CPU", "MEM", "Address"}, m.screen.titles())
	r.Equal([]string{nomad.TopicNode}, m.screen.topics())

	// What the machine takes is not known until it is read.
	r.Regexp(`^\s*node-1\s+nomad-server-01\s+dc1\s+default\s+1\.11\.1\s+ready\s+eligible\s+false\s+-\s+-\s+10\.0\.0\.2`, fileRow(t, m, 0))

	r.Equal([]string{"enter Allocations", "ctrl+d Drain", "i Toggle Eligibility", "space Mark", "ctrl+a Mark All"}, keyNames(m))
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

	// Stopping a drain does not move anything; it stops the moving.
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
