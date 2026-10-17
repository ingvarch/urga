//go:build !windows

package config_test

// File modes are checked off Windows only: there Stat reports 0666 for any
// file that can be written, and Chmod cannot stop new files in a folder.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/config"
)

func TestConfig_SaveKeepsTheFilePrivate(t *testing.T) {
	r := require.New(t)

	path := filepath.Join(t.TempDir(), "urga", "config.json")

	cfg := config.Load(path)

	cfg.UseNamespace("production")
	save(t, cfg)

	info, err := os.Stat(path)
	r.NoError(err)
	r.Equal(os.FileMode(0o600), info.Mode().Perm())
}

func TestConfig_AFailedWriteKeepsTheOldFile(t *testing.T) {
	r := require.New(t)

	folder := filepath.Join(t.TempDir(), "urga")
	file := filepath.Join(folder, "config.json")

	cfg := config.Load(file)

	cfg.UseNamespace("production")
	save(t, cfg)

	before, err := os.ReadFile(file)
	r.NoError(err)

	r.NoError(os.Chmod(folder, 0o500))
	t.Cleanup(func() { _ = os.Chmod(folder, 0o700) })

	cfg.UseNamespace("staging")
	r.Error(write(cfg))

	after, err := os.ReadFile(file)
	r.NoError(err)
	r.Equal(string(before), string(after))
}
