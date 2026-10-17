package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/config"
)

// write snapshots and writes, the way the UI does.
func write(cfg *config.Config) error {
	s, err := cfg.Snapshot()
	if err != nil {
		return err
	}

	return cfg.Write(s)
}

func save(t *testing.T, cfg *config.Config) {
	t.Helper()
	require.NoError(t, write(cfg))
}

func TestConfig_RoundTrip(t *testing.T) {
	r := require.New(t)

	path := filepath.Join(t.TempDir(), "config.json")

	cfg := config.Load(path)
	r.Nil(cfg.Namespace)
	r.Empty(cfg.Screen)

	cfg.UseNamespace("production")
	cfg.Screen = "deployments"
	cfg.Namespaces = []string{"production", "staging"}

	save(t, cfg)

	again := config.Load(path)

	r.Equal("production", *again.Namespace)
	r.Equal("deployments", again.Screen)
	r.Equal([]string{"production", "staging"}, again.Namespaces)
}

func TestConfig_AllNamespacesSurvives(t *testing.T) {
	r := require.New(t)

	path := filepath.Join(t.TempDir(), "config.json")

	cfg := config.Load(path)

	// Every namespace at once ("*") is saved like any other namespace. A
	// session that ends on it starts on it again.
	cfg.UseNamespace("*")
	save(t, cfg)

	again := config.Load(path)

	r.NotNil(again.Namespace)
	r.Equal("*", *again.Namespace)
}

func TestConfig_UseNamespaceKeepsTheOrder(t *testing.T) {
	r := require.New(t)

	path := filepath.Join(t.TempDir(), "config.json")

	cfg := config.Load(path)

	cfg.Remember([]string{"default", "production", "staging"})
	cfg.Remember([]string{"staging", "default", "production"})

	// The keys keep pointing at the same namespaces whatever order the
	// cluster answers in.
	r.Equal([]string{"default", "production", "staging"}, cfg.Namespaces)
}

func TestConfig_ANamespaceThatIsGoneLosesItsKey(t *testing.T) {
	r := require.New(t)

	path := filepath.Join(t.TempDir(), "config.json")

	cfg := config.Load(path)

	cfg.Remember([]string{"default", "ml", "staging"})

	// ml was deleted: a key of it would open nothing, and the one after it
	// takes its key.
	cfg.Remember([]string{"staging", "default"})
	r.Equal([]string{"default", "staging"}, cfg.Namespaces)

	// A cluster that names none says nothing about what is gone.
	cfg.Remember(nil)
	r.Equal([]string{"default", "staging"}, cfg.Namespaces)
}

func TestConfig_MissingFileIsNoError(t *testing.T) {
	r := require.New(t)

	cfg := config.Load(filepath.Join(t.TempDir(), "nothing", "here", "config.json"))

	// A first run has nothing to read, and that is not a failure.
	r.NotNil(cfg)
}

func TestConfig_BrokenFileIsNoError(t *testing.T) {
	r := require.New(t)

	path := filepath.Join(t.TempDir(), "config.json")

	r.NoError(os.WriteFile(path, []byte("{not json"), 0o600))

	cfg := config.Load(path)

	// A file that cannot be read starts the session fresh instead of
	// stopping it.
	r.Nil(cfg.Namespace)
}

func TestConfig_ASessionForEachCluster(t *testing.T) {
	r := require.New(t)

	path := filepath.Join(t.TempDir(), "config.json")

	cfg := config.Load(path)

	cfg.Of("prod").UseNamespace("batch")
	cfg.Of("prod").Screen = "nodes"
	cfg.Of("").UseNamespace("default")
	save(t, cfg)

	again := config.Load(path)

	// Each cluster starts again on the namespace and screen it was left on,
	// and so does the cluster of the environment.
	r.Equal("batch", *again.Of("prod").Namespace)
	r.Equal("nodes", again.Of("prod").Screen)
	r.Equal("default", *again.Of("").Namespace)
	r.Nil(again.Of("dev").Namespace)
}

func TestConfig_TheFileOfAnEarlierUrga(t *testing.T) {
	r := require.New(t)

	path := filepath.Join(t.TempDir(), "config.json")

	r.NoError(os.WriteFile(path,
		[]byte(`{"namespace": "staging", "screen": "jobs", "namespaces": ["default", "staging"]}`), 0o600))

	cfg := config.Load(path)

	// What an earlier urga wrote is read as the session of the environment.
	session := cfg.Of("")
	r.Equal("staging", *session.Namespace)
	r.Equal("jobs", session.Screen)
	r.Equal([]string{"default", "staging"}, session.Namespaces)
}

func TestConfig_SaveLeavesAWholeFileAndNothingElse(t *testing.T) {
	r := require.New(t)

	folder := filepath.Join(t.TempDir(), "urga")
	path := filepath.Join(folder, "config.json")

	cfg := config.Load(path)

	cfg.UseNamespace("production")
	save(t, cfg)

	again := config.Load(path)
	r.Equal("production", *again.Namespace)

	entries, err := os.ReadDir(folder)
	r.NoError(err)
	r.Len(entries, 1)
	r.Equal("config.json", entries[0].Name())

}

func TestConfig_AFailedRenameLeavesNoTempFile(t *testing.T) {
	r := require.New(t)

	folder := t.TempDir()
	blocker := filepath.Join(folder, "config.json")
	r.NoError(os.MkdirAll(filepath.Join(blocker, "inner"), 0o755))

	cfg := config.Load(blocker)

	cfg.UseNamespace("production")
	r.Error(write(cfg))

	entries, err := os.ReadDir(folder)
	r.NoError(err)
	r.Len(entries, 1)
	r.Equal("config.json", entries[0].Name())
}

func TestConfig_AnEarlierSnapshotNeverOverwritesALaterOne(t *testing.T) {
	r := require.New(t)

	path := filepath.Join(t.TempDir(), "config.json")

	cfg := config.Load(path)

	cfg.UseNamespace("first")

	first, err := cfg.Snapshot()
	r.NoError(err)

	cfg.UseNamespace("second")

	second, err := cfg.Snapshot()
	r.NoError(err)

	r.NoError(cfg.Write(second))
	r.NoError(cfg.Write(first))

	again := config.Load(path)
	r.Equal("second", *again.Namespace)
}
