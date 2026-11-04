package buildconfig_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// lintWith runs `make lint` with nothing on the PATH but the golangci-lint
// the test put there, if any: one that prints version for `version --short`.
func lintWith(t *testing.T, version string) (string, error) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("the Makefile runs under a POSIX shell")
	}

	maker, err := exec.LookPath("make")
	if err != nil {
		t.Skip("make is not installed")
	}

	bin := t.TempDir()

	if version != "" {
		fake := "#!/bin/sh\ncase \"$1\" in\n  version) echo " + version + " ;;\n  *) echo linted \"$@\" ;;\nesac\n"
		require.NoError(t, os.WriteFile(filepath.Join(bin, "golangci-lint"), []byte(fake), 0o755))
	}

	cmd := exec.Command(maker, "-s", "-C", filepath.Join("..", ".."), "lint")
	cmd.Env = []string{"PATH=" + bin}

	out, err := cmd.CombinedOutput()

	return string(out), err
}

func TestCI_LintsWithTheLatestLinter(t *testing.T) {
	// A pinned version falls behind, and then the linter on the machine of
	// whoever runs make check finds what CI does not.
	require.Regexp(t, `golangci/golangci-lint-action@\S+\s+with:\s+version:\s*latest\s`, workflows(t)["ci.yml"])
}

func TestMakefile_LintFailsWithoutTheLinter(t *testing.T) {
	r := require.New(t)

	// A check that passes because the linter is missing checks nothing.
	out, err := lintWith(t, "")
	r.Error(err)
	r.Contains(out, "golangci-lint is not installed")
	r.Contains(out, "latest")
}

func TestMakefile_LintsWithTheInstalledVersion(t *testing.T) {
	r := require.New(t)

	out, err := lintWith(t, "2.0.0")
	r.NoError(err, out)
	r.Contains(out, "linted run ./...")
}
