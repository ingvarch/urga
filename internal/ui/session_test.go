package ui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/config"
	"github.com/ingvarch/urga/internal/nomad"
)

func sessionModel(t *testing.T, client Client) (Model, *config.Config) {
	t.Helper()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := config.Load()
	require.NoError(t, err)

	m := New(client, Options{Namespace: "production", Version: "v-test", Config: cfg, PollEvery: time.Millisecond})
	m, _ = m.update(sizeMsg())

	return m, cfg
}

func TestSession_RemembersTheNamespace(t *testing.T) {
	r := require.New(t)

	m, cfg := sessionModel(t, &fakeClient{namespaces: threeNamespaces()})
	m, _ = m.update(namespacesMsg(threeNamespaces()))

	m, cmd := m.update(key('3'))
	drain(m, cmd)

	r.NotNil(cfg.Namespace)
	r.Equal("staging", *cfg.Namespace)

	// Every namespace at once is remembered as such, not as nothing.
	m, cmd = m.update(key('0'))
	drain(m, cmd)

	r.Equal(nomad.AllNamespaces, *cfg.Namespace)
}

func TestSession_RemembersTheScreen(t *testing.T) {
	r := require.New(t)

	m, cfg := sessionModel(t, &fakeClient{})

	m, _ = m.update(key(':'))
	m = typeIn(m, "dp")
	m, cmd := m.update(enter())
	drain(m, cmd)

	r.Equal("deployments", cfg.Screen)
}

func TestSession_RemembersTheOrderOfTheKeys(t *testing.T) {
	r := require.New(t)

	m, cfg := sessionModel(t, &fakeClient{namespaces: threeNamespaces()})
	_, _ = m.update(namespacesMsg(threeNamespaces()))

	r.Equal([]string{"default", "production", "staging"}, cfg.Namespaces)
}

func TestSession_StartsWhereItStopped(t *testing.T) {
	r := require.New(t)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := config.Load()
	r.NoError(err)

	cfg.UseNamespace(nomad.AllNamespaces)
	cfg.Screen = "nodes"
	cfg.Namespaces = []string{"staging", "default"}

	m := New(&fakeClient{}, Options{Namespace: "production", Version: "v-test", Config: cfg, PollEvery: time.Millisecond})
	m, _ = m.update(sizeMsg())

	// The session comes back where it was left, keys and all.
	r.Equal(nomad.AllNamespaces, m.namespace)
	r.Equal(screenNodes, m.screen.kind)
	r.Equal([]string{"staging", "default"}, m.namespaceOrder)
}

func TestSession_ANamespaceGivenOnTheCommandLineWins(t *testing.T) {
	r := require.New(t)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := config.Load()
	r.NoError(err)

	cfg.UseNamespace("staging")
	cfg.Screen = "nodes"

	m := New(&fakeClient{}, Options{
		Namespace:      "production",
		NamespaceGiven: true,
		Version:        "v-test",
		Config:         cfg,
		PollEvery:      time.Millisecond,
	})
	m, _ = m.update(sizeMsg())

	// Typing a namespace on the command line is asking for it now. The
	// rest of the session still comes back.
	r.Equal("production", m.namespace)
	r.Equal("production", m.screen.namespace)
	r.Equal(screenNodes, m.screen.kind)
}

func TestSession_WithoutAConfig(t *testing.T) {
	r := require.New(t)

	// urga runs with nothing to remember it by.
	m := newTestModel(&fakeClient{})

	m, cmd := m.update(key('0'))

	r.NotPanics(func() { drain(m, cmd) })
}

func TestSession_RemembersANamespaceFromTheCommandLine(t *testing.T) {
	r := require.New(t)

	m, cfg := sessionModel(t, &fakeClient{namespaces: threeNamespaces()})
	m, _ = m.update(namespacesMsg(threeNamespaces()))

	// The job list is already open, and the command names the same resource
	// with another namespace.
	m, _ = m.update(key(':'))
	m = typeIn(m, "jobs staging")
	m, cmd := m.update(enter())
	drain(m, cmd)

	r.Equal("staging", m.namespace)

	// Which namespace the session looks at is written down whichever way it
	// was chosen, so the next run comes back to it.
	r.NotNil(cfg.Namespace)
	r.Equal("staging", *cfg.Namespace)
}

func TestSession_RemembersTheScreenAfterGoingBack(t *testing.T) {
	r := require.New(t)

	m, cfg := sessionModel(t, &fakeClient{jobs: twoJobs()})

	m, _ = m.update(key(':'))
	m = typeIn(m, "dp")
	m, cmd := m.update(enter())
	m = drain(m, cmd)
	r.Equal("deployments", cfg.Screen)

	// Escape comes back to the job list, and that is where the next run has
	// to open, not on the screen that was left.
	m, cmd = m.update(escape())
	drain(m, cmd)

	r.Equal(screenJobs, m.screen.kind)
	r.Equal("jobs", cfg.Screen)
}

func TestSession_EachClusterComesBackToItsOwn(t *testing.T) {
	r := require.New(t)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := config.Load()
	r.NoError(err)

	cfg.Of("").UseNamespace("default")
	cfg.Of("prod").UseNamespace("batch")
	cfg.Of("prod").Screen = "nodes"
	cfg.Of("prod").Namespaces = []string{"batch", "web"}

	// prod comes back to prod's namespace, screen and keys.
	m := New(&fakeClient{}, Options{Cluster: "prod", Version: "v-test", Config: cfg})
	r.Equal("batch", m.namespace)
	r.Equal(screenNodes, m.screen.kind)
	r.Equal([]string{"batch", "web"}, m.namespaceOrder)

	// What prod looks at is written down for prod, and nowhere else.
	m, _ = m.update(sizeMsg())
	m, _ = m.update(namespacesMsg(threeNamespaces()))
	m, cmd := m.update(key('2'))
	drain(m, cmd)

	r.Equal("web", *cfg.Of("prod").Namespace)
	r.Equal("default", *cfg.Of("").Namespace)
}
