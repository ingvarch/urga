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

func TestWorkflows_ReleaseWhatTheReleaseConfigSays(t *testing.T) {
	r := require.New(t)

	content := workflows(t)["release.yml"]
	r.NotEmpty(content, "there is no release workflow")

	// The platforms and packages live in .goreleaser.yaml alone, so the
	// workflow keeps no list of its own.
	r.Contains(content, "args: release --clean")
	r.NotContains(content, "goos:")

	// The notes are written from the tags, and a shallow clone has none.
	r.Contains(content, "fetch-depth: 0")

	// The job's own token cannot push the cask to the tap.
	r.Contains(content, "HOMEBREW_TAP_GITHUB_TOKEN: ${{ secrets.HOMEBREW_TAP_GITHUB_TOKEN }}")
}

func TestWorkflows_GiveTheReleaseTheAppleKeys(t *testing.T) {
	r := require.New(t)

	content := workflows(t)["release.yml"]

	// The release signs and notarizes the macOS binaries with these.
	for _, secret := range []string{
		"MACOS_SIGN_P12",
		"MACOS_SIGN_PASSWORD",
		"MACOS_NOTARY_ISSUER_ID",
		"MACOS_NOTARY_KEY_ID",
		"MACOS_NOTARY_KEY",
	} {
		r.Contains(content, secret+": ${{ secrets."+secret+" }}")
	}
}

func TestWorkflows_TryTheReleaseOnEveryPush(t *testing.T) {
	r := require.New(t)

	content := workflows(t)["ci.yml"]

	// A broken release config shows up in the pull request, not on the tag,
	// and it builds the same platforms the release does.
	r.Contains(content, "args: release --snapshot --clean")
	r.NotContains(content, "goos:")
}

func TestWorkflows_PinGoreleaser(t *testing.T) {
	r := require.New(t)

	data, err := os.ReadFile(filepath.Join("..", "..", ".tool-versions"))
	r.NoError(err)

	// The tool that builds the release changes when this repository says
	// so, the same as the runner.
	r.Regexp(`(?m)^goreleaser \d+\.\d+\.\d+$`, string(data))

	for name, content := range workflows(t) {
		if !strings.Contains(content, "goreleaser-action") {
			continue
		}

		r.Contains(content, "version-file: .tool-versions", name)
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

func TestMakefile_DistBuildsWhatTheReleaseBuilds(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "Makefile"))
	require.NoError(t, err)

	// make dist is how a release is tried before it is tagged, so it runs the
	// same config the release does.
	require.Regexp(t, `(?m)^dist:\n\tgoreleaser release --snapshot --clean$`, string(data))
}
