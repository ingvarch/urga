package ui

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/config"
	"github.com/ingvarch/urga/internal/nomad"
)

// twoClusters is a session on dev that can switch to prod, which is red and
// read-only. connected are the names it was asked to connect to.
type twoClusters struct {
	dev, prod *fakeClient
	cfg       *config.Config
	connected []string
	refuse    error
}

func (c *twoClusters) connect(name string) (Connection, error) {
	c.connected = append(c.connected, name)

	if c.refuse != nil {
		return Connection{}, c.refuse
	}

	if name == "prod" {
		return Connection{Name: "prod", Color: "red", ReadOnly: true, Client: c.prod, Namespace: "payments"}, nil
	}

	return Connection{Name: "dev", Client: c.dev}, nil
}

func onDev(t *testing.T) (Model, *twoClusters) {
	t.Helper()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := config.Load()
	require.NoError(t, err)

	// Both clusters stream their events, like a cluster that allows it, so
	// no warning about polling hides the messages of the switch.
	clusters := &twoClusters{
		dev: &fakeClient{jobs: twoJobs(), changes: newChanges()},
		prod: &fakeClient{
			jobs:    []nomad.Job{{ID: "billing", Name: "billing", Namespace: "payments", Status: "running"}},
			changes: newChanges(),
		},
		cfg: cfg,
	}

	m := New(clusters.dev, Options{
		Cluster:  "dev",
		Clusters: []string{"dev", "prod"},
		Connect:  clusters.connect,
		Config:   cfg,
		Version:  "v-test",
	})
	m, _ = m.update(sizeMsg())
	m = drain(m, m.fetch())

	return m, clusters
}

// typeCommand types a line into the prompt and runs what it starts, the
// way the program does.
func typeCommand(m Model, line string) Model {
	m, _ = m.update(key(':'))
	m = typeIn(m, line)
	m, cmd := m.update(enter())

	return playOut(m, cmd)
}

func TestClusters_SwitchByName(t *testing.T) {
	r := require.New(t)

	m, clusters := onDev(t)
	m = typeCommand(m, "ctx prod")

	r.Equal([]string{"prod"}, clusters.connected)

	// The other cluster, as the settings say it is.
	out := plain(m.render())
	r.Contains(out, "Cluster:   prod")
	r.Contains(out, "read-only")
	r.Contains(out, "billing")
	r.NotContains(out, "cron")

	// Shown once connected, not while the connection is still being made.
	r.Contains(out, "Connected to prod.")
	r.NotContains(out, "Connecting")

	m, _ = m.update(key('e'))
	r.Contains(plain(m.render()), "read-only: Edit is off")
}

func TestClusters_EachKeepsItsNamespace(t *testing.T) {
	r := require.New(t)

	m, clusters := onDev(t)
	clusters.cfg.Of("dev").UseNamespace("default")

	// The first time, prod starts in the namespace its settings give.
	m = typeCommand(m, "ctx prod")
	r.Equal("payments", m.namespace)

	m = typeCommand(m, "ctx dev")
	r.Equal("default", m.namespace)
	r.Equal("dev", m.opts.Cluster)
	r.False(m.opts.ReadOnly)

	m = typeCommand(m, "ctx prod")
	r.Equal("payments", *clusters.cfg.Of("prod").Namespace)
	r.Equal("payments", m.namespace)
}

func TestClusters_PickFromTheList(t *testing.T) {
	r := require.New(t)

	m, clusters := onDev(t)
	m = typeCommand(m, "ctx")

	r.IsType(clustersPage{}, m.screen.page)
	r.Contains(fileRow(t, m, 0), "dev")
	r.Contains(fileRow(t, m, 0), "in use")
	r.Contains(fileRow(t, m, 1), "prod")

	m, _ = m.update(key('j'))
	m, cmd := m.update(enter())
	m = playOut(m, cmd)

	r.Equal([]string{"prod"}, clusters.connected)
	r.Equal("prod", m.opts.Cluster)
}

func TestClusters_ANameNotInTheSettings(t *testing.T) {
	r := require.New(t)

	m, clusters := onDev(t)
	m = typeCommand(m, "ctx stage")

	r.Empty(clusters.connected)
	r.Contains(plain(m.render()), "no such cluster: stage (dev, prod)")
}

func TestClusters_WithoutSettings(t *testing.T) {
	r := require.New(t)

	m := New(&fakeClient{}, Options{Version: "v-test"})
	m, _ = m.update(sizeMsg())

	m = typeCommand(m, "ctx prod")
	r.Contains(plain(m.render()), "no clusters to switch to: the settings file names none")
}

func TestClusters_AClusterThatCannotBeReached(t *testing.T) {
	r := require.New(t)

	m, clusters := onDev(t)
	clusters.refuse = errors.New("cluster \"prod\": NOMAD_TOKEN_PROD is not set")

	m = typeCommand(m, "ctx prod")

	// The session stays on dev and shows the error.
	r.Equal("dev", m.opts.Cluster)
	r.Contains(plain(m.render()), "NOMAD_TOKEN_PROD is not set")
	r.Contains(plain(m.render()), "cron")
}

func TestClusters_ClosesTheLogsOfEveryAllocation(t *testing.T) {
	r := require.New(t)

	m, clusters := onDev(t)
	clusters.dev.allocs = webAllocs("server")[:2]
	clusters.dev.logsByAlloc = map[string]*nomad.LogStream{newer: writing(), older: writing()}

	m, cmd := m.update(key('l'))
	m = playOut(m, cmd)
	r.IsType(jobLogsPage{}, m.screen.page)

	closed := countClosed(clusters.dev.logsByAlloc)
	m = typeCommand(m, "ctx prod")

	// Each stream holds a request open to a client of the old cluster.
	r.Equal(2, *closed)
	r.Equal("prod", m.opts.Cluster)
}

func TestClusters_AnAnswerOfTheClusterLeftIsDropped(t *testing.T) {
	r := require.New(t)

	m, _ := onDev(t)
	left := m.connection

	m = typeCommand(m, "ctx prod")

	// Asked of dev before the switch, answered after it.
	m, _ = m.update(connectionMsg{connection: left, msg: agentMsg{Version: "1.0.0-dev"}})
	r.NotContains(plain(m.render()), "1.0.0-dev")

	m, _ = m.update(connectionMsg{connection: m.connection, msg: agentMsg{Version: "2.0.7"}})
	r.Contains(plain(m.render()), "2.0.7")
}
