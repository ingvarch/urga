package ui

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// faultyClients is a client of every state the client lines tell apart.
func faultyClients() []nomad.Node {
	return []nomad.Node{
		{ID: "ok", Datacenter: "dc1", Status: "ready", Eligibility: "eligible"},
		{ID: "down", Datacenter: "dc1", Status: "down", Eligibility: "ineligible"},
		{ID: "lost", Datacenter: "dc1", Status: "disconnected", Eligibility: "eligible"},
		{ID: "booting", Datacenter: "dc1", Status: "initializing", Eligibility: "eligible"},
		{ID: "draining", Datacenter: "dc1", Status: "ready", Eligibility: "ineligible", Drain: true},
		{ID: "cordoned", Datacenter: "dc1", Status: "ready", Eligibility: "ineligible"},
		{ID: "down-draining", Datacenter: "dc1", Status: "down", Eligibility: "ineligible", Drain: true},
	}
}

func TestClientFaults_EachStateKeepsItsOwn(t *testing.T) {
	cases := map[fault][]string{
		faultDown:         {"down", "down-draining"},
		faultDisconnected: {"lost"},
		faultDraining:     {"draining"},
		faultIneligible:   {"cordoned"},
	}

	for f, want := range cases {
		t.Run(f.String(), func(t *testing.T) {
			page := nodesPage{nodes: faultyClients(), fault: f}

			require.Equal(t, want, page.ids(env{}))
		})
	}
}

func TestClientFaults_TitleSaysWhatTheListIsNarrowedTo(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{})
	m, _ = m.update(openMsg{nodesPage{nodes: faultyClients(), fault: faultDraining}})

	r.Equal("Clients (draining) [1]", m.title())
}

func TestClientFaults_KeepTheDatacenterToo(t *testing.T) {
	r := require.New(t)

	nodes := append(faultyClients(), nomad.Node{ID: "far", Datacenter: "dc2", Status: "down"})
	page := nodesPage{nodes: nodes, fault: faultDown}

	r.Equal([]string{"down", "down-draining"}, page.ids(env{datacenter: "dc1"}))
	r.Equal([]string{"down", "down-draining", "far"}, page.ids(env{}))
}
