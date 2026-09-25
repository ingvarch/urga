package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/config"
)

func TestConfig_RoundTrip(t *testing.T) {
	r := require.New(t)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := config.Load()
	r.NoError(err)
	r.Nil(cfg.Namespace)
	r.Empty(cfg.Screen)

	cfg.UseNamespace("production")
	cfg.Screen = "deployments"
	cfg.Namespaces = []string{"production", "staging"}

	r.NoError(cfg.Save())

	again, err := config.Load()
	r.NoError(err)

	r.Equal("production", *again.Namespace)
	r.Equal("deployments", again.Screen)
	r.Equal([]string{"production", "staging"}, again.Namespaces)
}

func TestConfig_AllNamespacesSurvives(t *testing.T) {
	r := require.New(t)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := config.Load()
	r.NoError(err)

	// Every namespace at once ("*") is saved like any other namespace. A
	// session that ends on it starts on it again.
	cfg.UseNamespace("*")
	r.NoError(cfg.Save())

	again, err := config.Load()
	r.NoError(err)

	r.NotNil(again.Namespace)
	r.Equal("*", *again.Namespace)
}

func TestConfig_UseNamespaceKeepsTheOrder(t *testing.T) {
	r := require.New(t)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := config.Load()
	r.NoError(err)

	cfg.Remember([]string{"default", "production", "staging"})
	cfg.Remember([]string{"staging", "default", "production"})

	// The keys keep pointing at the same namespaces whatever order the
	// cluster answers in.
	r.Equal([]string{"default", "production", "staging"}, cfg.Namespaces)
}

func TestConfig_MissingFileIsNoError(t *testing.T) {
	r := require.New(t)

	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "nothing", "here"))

	cfg, err := config.Load()

	// A first run has nothing to read, and that is not a failure.
	r.NoError(err)
	r.NotNil(cfg)
}

func TestConfig_BrokenFileIsNoError(t *testing.T) {
	r := require.New(t)

	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	r.NoError(os.MkdirAll(filepath.Join(home, "urga"), 0o755))
	r.NoError(os.WriteFile(filepath.Join(home, "urga", "config.json"), []byte("{not json"), 0o600))

	cfg, err := config.Load()

	// A file that cannot be read starts the session fresh instead of
	// stopping it.
	r.NoError(err)
	r.Nil(cfg.Namespace)
}

func TestConfig_ASessionForEachCluster(t *testing.T) {
	r := require.New(t)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := config.Load()
	r.NoError(err)

	cfg.Of("prod").UseNamespace("batch")
	cfg.Of("prod").Screen = "nodes"
	cfg.Of("").UseNamespace("default")
	r.NoError(cfg.Save())

	again, err := config.Load()
	r.NoError(err)

	// Each cluster starts again on the namespace and screen it was left on,
	// and so does the cluster of the environment.
	r.Equal("batch", *again.Of("prod").Namespace)
	r.Equal("nodes", again.Of("prod").Screen)
	r.Equal("default", *again.Of("").Namespace)
	r.Nil(again.Of("dev").Namespace)
}

func TestConfig_TheFileOfAnEarlierUrga(t *testing.T) {
	r := require.New(t)

	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	r.NoError(os.MkdirAll(filepath.Join(home, "urga"), 0o755))
	r.NoError(os.WriteFile(filepath.Join(home, "urga", "config.json"),
		[]byte(`{"namespace": "staging", "screen": "jobs", "namespaces": ["default", "staging"]}`), 0o600))

	cfg, err := config.Load()
	r.NoError(err)

	// What an earlier urga wrote is read as the session of the environment.
	session := cfg.Of("")
	r.Equal("staging", *session.Namespace)
	r.Equal("jobs", session.Screen)
	r.Equal([]string{"default", "staging"}, session.Namespaces)
}
