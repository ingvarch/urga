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

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg, err := config.Load()
	r.NoError(err)

	cfg.UseNamespace("production")
	r.NoError(cfg.Save())

	info, err := os.Stat(filepath.Join(dir, "urga", "config.json"))
	r.NoError(err)
	r.Equal(os.FileMode(0o600), info.Mode().Perm())
}

func TestConfig_AFailedWriteKeepsTheOldFile(t *testing.T) {
	r := require.New(t)

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg, err := config.Load()
	r.NoError(err)

	cfg.UseNamespace("production")
	r.NoError(cfg.Save())

	folder := filepath.Join(dir, "urga")
	file := filepath.Join(folder, "config.json")

	before, err := os.ReadFile(file)
	r.NoError(err)

	r.NoError(os.Chmod(folder, 0o500))
	t.Cleanup(func() { _ = os.Chmod(folder, 0o700) })

	cfg.UseNamespace("staging")
	r.Error(cfg.Save())

	after, err := os.ReadFile(file)
	r.NoError(err)
	r.Equal(string(before), string(after))
}
