package buildconfig_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// workflows are the files that say how urga is built on every push.
func workflows(t *testing.T) map[string]string {
	t.Helper()

	dir := filepath.Join("..", "..", ".github", "workflows")

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	out := map[string]string{}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		require.NoError(t, err)

		out[entry.Name()] = string(data)
	}

	require.NotEmpty(t, out, "no workflows to check")

	return out
}

func TestWorkflows_PinTheRunner(t *testing.T) {
	r := require.New(t)

	floating := regexp.MustCompile(`(runs-on:\s*\S*-latest|\S+-latest\s*[,\]])`)

	for name, content := range workflows(t) {
		// A floating label changes the machine under the build on a date
		// somebody else picks. The image is named here instead.
		r.NotRegexp(floating, content, name)
	}
}

func TestWorkflows_BuildForEveryPlatform(t *testing.T) {
	r := require.New(t)

	content := workflows(t)["release.yml"]
	r.NotEmpty(content, "there is no release workflow")

	// urga is downloaded for the machine it will run on, so every one of
	// them is built.
	for _, pair := range []string{
		"{ goos: linux, goarch: amd64 }",
		"{ goos: linux, goarch: arm64 }",
		"{ goos: linux, goarch: arm }",
		"{ goos: darwin, goarch: amd64 }",
		"{ goos: darwin, goarch: arm64 }",
		"{ goos: windows, goarch: amd64 }",
		"{ goos: windows, goarch: arm64 }",
		"{ goos: freebsd, goarch: amd64 }",
	} {
		r.Contains(content, pair)
	}
}

func TestWorkflows_TestOnEverySystem(t *testing.T) {
	r := require.New(t)

	content := workflows(t)["ci.yml"]

	// The tests run where people run urga, not only on Linux.
	for _, runner := range []string{"ubuntu-26.04", "macos-15", "windows-2025"} {
		r.Contains(content, runner)
	}
}

func TestWorkflows_TakeTheGoVersionFromTheModule(t *testing.T) {
	r := require.New(t)

	for name, content := range workflows(t) {
		if !strings.Contains(content, "setup-go") {
			continue
		}

		// One place says which Go builds urga, and it is go.mod.
		r.Contains(content, "go-version-file: go.mod", name)
	}
}

func TestWorkflows_UseActionsThatAreStillThere(t *testing.T) {
	r := require.New(t)

	retired := []string{
		"actions/checkout@v3",
		"actions/checkout@v4",
		"actions/setup-go@v4",
		"actions/setup-go@v5",
		"actions/upload-artifact@v3",
		"actions/upload-artifact@v4",
		"actions/download-artifact@v3",
		"actions/download-artifact@v4",
		"actions/download-artifact@v7",
	}

	for name, content := range workflows(t) {
		for _, action := range retired {
			r.NotContains(content, action, name)
		}
	}
}
