package ui

import (
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
	r.Contains(plain(m.render()), "stop giving new work to server-01")

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
