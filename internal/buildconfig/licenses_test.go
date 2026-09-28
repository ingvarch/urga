package buildconfig_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/ingvarch/urga/internal/licenses"
)

// noticesFile is the third-party notices file make notices writes and the
// release ships.
const noticesFile = "THIRD_PARTY_NOTICES"

// repoFile reads a file at the root of the repository.
func repoFile(t *testing.T, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", name))
	require.NoError(t, err)

	return string(data)
}

// oneCommand returns the only command of a Makefile target.
func oneCommand(t *testing.T, target string) string {
	t.Helper()

	lines := strings.Split(repoFile(t, "Makefile"), "\n")
	i := slices.IndexFunc(lines, func(line string) bool {
		return line == target+":" || strings.HasPrefix(line, target+": ")
	})
	require.GreaterOrEqual(t, i, 0, "the Makefile has no target %q", target)

	var commands []string
	for _, line := range lines[i+1:] {
		if !strings.HasPrefix(line, "\t") {
			break
		}

		commands = append(commands, strings.TrimPrefix(line, "\t"))
	}

	require.Len(t, commands, 1, "make %s runs %q", target, commands)

	return commands[0]
}

// listEntry returns an entry of a release config list that is either text or
// a map, such as a hook (cmd) or an archive file (src).
func listEntry(v any, key string) string {
	switch v := v.(type) {
	case string:
		return v
	case map[string]any:
		s, _ := v[key].(string)
		return s
	}

	return ""
}

func TestMakefile_NoticesCheckWhatLicensesChecks(t *testing.T) {
	r := require.New(t)

	// Both run the same check on the same packages; notices also writes the
	// file the release ships.
	notices, check := oneCommand(t, "notices"), oneCommand(t, "licenses")
	r.Contains(notices, " -notices "+noticesFile+" ")
	r.Equal(check, strings.Replace(notices, " -notices "+noticesFile, "", 1))
}

func TestMakefile_CheckRunsTheLicenseCheck(t *testing.T) {
	m := regexp.MustCompile(`(?m)^check: (.+)$`).FindStringSubmatch(repoFile(t, "Makefile"))
	require.NotNil(t, m, "the Makefile has no check target")

	// A module under a license urga may not ship fails before a commit.
	require.Equal(t, "fmt vet lint licenses test build", m[1])
}

func TestMakefile_CleanRemovesTheNotices(t *testing.T) {
	// The release writes it in the root of the tree.
	require.Contains(t, strings.Fields(oneCommand(t, "clean")), noticesFile)
}

func TestRelease_WritesTheNoticesBeforeItBuilds(t *testing.T) {
	// A before hook runs in the snapshot too, so every pull request writes
	// the notices and checks the licenses.
	want := oneCommand(t, "notices")
	hooks := release(t).Before.Hooks
	require.True(t, slices.ContainsFunc(hooks, func(h any) bool { return listEntry(h, "cmd") == want }),
		"before hooks %v do not run %q, as make notices does", hooks, want)
}

func TestRelease_ArchivesShipTheLicenseAndTheNotices(t *testing.T) {
	r := require.New(t)

	archives := release(t).Archives
	r.NotEmpty(archives)

	// MIT, BSD and Apache ask for the license text in every copy, MPL for
	// where to get the source; the notices hold both for what urga links.
	for i, a := range archives {
		for _, file := range []string{"LICENSE", noticesFile} {
			r.True(slices.ContainsFunc(a.Files, func(f any) bool { return listEntry(f, "src") == file }),
				"archive %d does not ship %s: files %v", i, file, a.Files)
		}
	}
}

func TestLicenses_CoverEveryReleasePlatform(t *testing.T) {
	// A module linked on only one platform still ships in that archive.
	var want []string
	for _, b := range release(t).Builds {
		for _, target := range b.Targets {
			parts := strings.SplitN(target, "_", 3)
			want = append(want, parts[0]+"/"+parts[1])
		}
	}

	require.ElementsMatch(t, want, licenses.Platforms)
}

// workflowSteps returns the steps of a job in a workflow.
func workflowSteps(t *testing.T, workflow, job string) []workflowStep {
	t.Helper()

	var parsed struct {
		Jobs map[string]struct {
			Steps []workflowStep
		}
	}
	require.NoError(t, yaml.Unmarshal([]byte(workflows(t)[workflow]), &parsed))
	require.Contains(t, parsed.Jobs, job, "%s has no job %s", workflow, job)

	return parsed.Jobs[job].Steps
}

// workflowStep is the part of a workflow step the tests look at.
type workflowStep struct {
	If  string
	Run string
}

func TestWorkflows_CheckTheLicensesLikeMakeLicenses(t *testing.T) {
	r := require.New(t)

	want := oneCommand(t, "licenses")
	steps := workflowSteps(t, "ci.yml", "check")

	i := slices.IndexFunc(steps, func(s workflowStep) bool { return s.Run == want })
	r.GreaterOrEqual(i, 0, "ci.yml check does not run %q, as make licenses does", want)

	// A failed step before it, such as the linter, must not hide a license
	// failure; the job still fails.
	r.Equal("success() || failure()", steps[i].If)
}

func TestGit_IgnoresTheNotices(t *testing.T) {
	// The release writes it; a tracked copy would go stale, and an untracked
	// one would make the tree dirty.
	ignored := strings.Split(repoFile(t, ".gitignore"), "\n")
	require.True(t, slices.Contains(ignored, "/"+noticesFile), ".gitignore does not ignore /%s", noticesFile)
}
