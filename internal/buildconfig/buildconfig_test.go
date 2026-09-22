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

	floating := regexp.MustCompile(`runs-on:\s*\S*-latest`)

	for name, content := range workflows(t) {
		// A floating label changes the machine under the build on a date
		// somebody else picks. The image is named here instead.
		r.NotRegexp(floating, content, name)
		r.Contains(content, "runs-on: ubuntu-26.04", name)
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
		"actions/download-artifact@v3",
	}

	for name, content := range workflows(t) {
		for _, action := range retired {
			r.NotContains(content, action, name)
		}
	}
}
