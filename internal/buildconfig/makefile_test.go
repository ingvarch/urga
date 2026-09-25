package buildconfig_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// ciLintVersion is the version of golangci-lint the CI workflow lints with.
func ciLintVersion(t *testing.T) string {
	t.Helper()

	pinned := regexp.MustCompile(`golangci/golangci-lint-action@\S+\s+with:\s+version:\s*v(\S+)`)

	match := pinned.FindStringSubmatch(workflows(t)["ci.yml"])
	require.NotNil(t, match, "ci.yml pins no golangci-lint version")

	return match[1]
}

// lintWith runs `make lint` with nothing on the PATH but the golangci-lint
// the test put there, if any: what answers `version --short` with version.
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

func TestMakefile_LintFailsWithoutTheLinter(t *testing.T) {
	r := require.New(t)

	// A check that passes because the linter is missing checks nothing.
	out, err := lintWith(t, "")
	r.Error(err)
	r.Contains(out, "golangci-lint is not installed")
	r.Contains(out, "v"+ciLintVersion(t))
}

func TestMakefile_LintFailsWithAnotherVersion(t *testing.T) {
	r := require.New(t)

	// Another version finds other things, and the build in CI says otherwise.
	out, err := lintWith(t, "2.0.0")
	r.Error(err)
	r.Contains(out, "2.0.0")
	r.Contains(out, "v"+ciLintVersion(t))
}

func TestMakefile_LintsWithTheVersionCIUses(t *testing.T) {
	r := require.New(t)

	out, err := lintWith(t, ciLintVersion(t))
	r.NoError(err, out)
	r.Contains(out, "linted run ./...")
}
