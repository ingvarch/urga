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
