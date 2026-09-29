package ui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/config"
	"github.com/ingvarch/urga/internal/nomad"
)

func twoPools() []nomad.NodePool {
	return []nomad.NodePool{
		{Name: "default", Scheduler: "binpack", Description: "every client"},
		{Name: "gpu", Scheduler: "spread", Description: "the ones with cards"},
	}
}

func TestNodePools_ListsThePoolsOfTheCluster(t *testing.T) {
	r := require.New(t)

	m, cfg := sessionModel(t, &fakeClient{nodePools: twoPools()})
	m = typeCommand(m, "nodepools")

	// A pool belongs to the cluster, not to a namespace: the title names none.
	out := plain(m.render())
	r.Contains(out, "Node Pools [2]")
	r.NotContains(out, "Node Pools (")

	r.Contains(fileRow(t, m, -1), "Scheduler")
	r.Contains(fileRow(t, m, 0), "default")
	r.Contains(fileRow(t, m, 0), "binpack")
	r.Contains(fileRow(t, m, 1), "the ones with cards")

	// The next run opens it again, and it watches the events of pools.
	r.Equal("nodepools", cfg.Screen)
	r.Equal([]string{nomad.TopicNodePool}, m.screen.page.topics())
}

func TestNodePools_ASessionStartsOnThem(t *testing.T) {
	r := require.New(t)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := config.Load()
	r.NoError(err)

	cfg.Screen = "nodepools"

	client := &fakeClient{nodePools: twoPools()}
	m := New(client, Options{Version: "v-test", Config: cfg, PollEvery: time.Millisecond})
	m, _ = m.update(sizeMsg())
	m = drain(m, m.fetch())

	r.Contains(plain(m.render()), "Node Pools [2]")
	r.Contains(fileRow(t, m, 1), "gpu")
}

func TestNodePools_OfTheRegionLeftAreLetGo(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{nodePools: twoPools()}
	m := regionalModel(t, client)
	m = typeCommand(m, "nodepools")
	r.Contains(plain(m.render()), "Node Pools [2]")

	// The pools of eu must not be shown as those of us before us answers.
	client.nodePools = nil
	m, _ = runLine(m, "region us")

	r.Contains(plain(m.render()), "Node Pools [0]")
	r.NotContains(plain(m.render()), "gpu")
}

func TestNodePools_EnterShowsTheJobsOfThePool(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{nodePools: twoPools(), changes: newChanges(), poolJobs: []nomad.Job{
		{ID: "train", Name: "train", Type: "batch", Namespace: "ml", Status: "running", Periodic: true},
		{ID: "train/periodic-1700000000", ParentID: "train", Type: "batch", Namespace: "ml", Status: "dead"},
	}}
	m, _ := sessionModel(t, client)
	m = typeCommand(m, "nodepools")
	m.namespaceOrder = []string{"production", "staging"}

	// On gpu.
	m, _ = m.update(key('j'))
	m, cmd := m.update(enter())
	m = playOut(m, cmd)

	// The jobs of every namespace that run in the pool. A launch stands
	// behind the job that launches it, as on the list of jobs.
	r.Equal("gpu", client.askedPool)
	r.Contains(plain(m.render()), "Jobs (Pool: gpu) [1]")
	r.Contains(fileRow(t, m, 0), "ml")
	r.Equal(nomad.AllNamespaces, client.watchedNamespace)

	// The pool is not in a namespace: switching the session's leaves it,
	// with its filter.
	m.list.filter = "train"
	m, cmd = m.update(key('2'))
	m = playOut(m, cmd)

	r.Equal("staging", m.namespace)
	r.Contains(plain(m.render()), "Jobs (Pool: gpu) [1]")
	r.Equal("train", m.list.filter)
}

func TestNodePools_CShowsTheClientsOfThePool(t *testing.T) {
	r := require.New(t)

	// The cluster says which clients a pool holds: all holds every one,
	// whatever pool each is in.
	client := &fakeClient{
		nodePools: []nomad.NodePool{{Name: "all"}, {Name: "default"}},
		poolNodes: []nomad.Node{
			{ID: "node-1", Name: "gpu-01", NodePool: "gpu", Status: "ready"},
			{ID: "node-2", Name: "web-01", NodePool: "default", Status: "ready"},
		},
	}
	m, _ := sessionModel(t, client)
	m = typeCommand(m, "nodepools")

	m, cmd := m.update(key('c'))
	m = playOut(m, cmd)

	r.Equal("all", client.askedPool)

	out := plain(m.render())
	r.Contains(out, "Clients (Pool: all) [2]")
	r.Contains(out, "gpu-01")
	r.Contains(out, "web-01")
}
