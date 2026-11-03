package ui

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// fakeEditor writes what a person would have typed, without a terminal.
type fakeEditor struct {
	opened  string
	replace string
	err     error

	// edits are typed one per open, in turn; once they run out the file is
	// left as it is. seen is what each open found in the file.
	edits []string
	seen  []string

	// onEdit runs while the file is open, like a change made elsewhere.
	onEdit func()
}

func (e *fakeEditor) Edit(path string) tea.Cmd {
	return func() tea.Msg {
		e.opened = path

		if e.onEdit != nil {
			e.onEdit()
		}

		if data, err := os.ReadFile(path); err == nil {
			e.seen = append(e.seen, string(data))
		}

		if e.err != nil {
			return editedMsg{path: path, err: e.err}
		}

		replace := e.replace
		if e.edits != nil {
			replace = ""

			if len(e.edits) > 0 {
				replace, e.edits = e.edits[0], e.edits[1:]
			}
		}

		if replace != "" {
			if err := os.WriteFile(path, []byte(replace), 0o600); err != nil {
				return editedMsg{path: path, err: err}
			}
		}

		return editedMsg{path: path}
	}
}

func editModel(t *testing.T, client *fakeClient) (Model, *fakeEditor) {
	t.Helper()

	editor := &fakeEditor{}

	m := New(client, Options{Namespace: "production", Version: "v-test", Editor: editor, PollEvery: time.Millisecond})
	m, _ = m.update(sizeMsg())
	m, _ = m.update(jobsMsg(twoJobs()))

	return m, editor
}

// tempDir points the temp dir urga writes to at a new directory of the test,
// and returns it.
func tempDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	useTempDir(t, dir)

	return dir
}

// useTempDir points the temp dir urga writes to at dir. Windows reads TMP and
// TEMP instead of TMPDIR.
func useTempDir(t *testing.T, dir string) {
	t.Helper()

	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, dir)
	}
}

func TestEdit_OpensTheJobFile(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), spec: nomad.JobSource{Source: "job \"web\" {\n  type = \"service\"\n}"}}
	m, editor := editModel(t, client)
	editor.replace = "job \"web\" {\n  type = \"batch\"\n}"

	m, cmd := m.update(key('e'))
	m = follow(m, cmd, 5)

	// What was on the screen is what the editor was given.
	r.NotEmpty(editor.opened)
	r.Equal(".hcl", filepath.Ext(editor.opened))

	// What came back is planned first, and submitted from the plan.
	r.IsType(planPage{}, m.screen.page)

	m, cmd = m.update(key('y'))
	m = drain(m, cmd)

	r.Equal(1, client.submitted)
	r.Equal("job \"web\" {\n  type = \"batch\"\n}", client.submittedSource)
	r.Equal("production", client.askedNamespace)
	r.Contains(plain(m.render()), "Job web submitted")
}

func TestEdit_AJSONSourceOpensAsJSON(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		jobs: twoJobs(),
		spec: nomad.JobSource{Source: `{"ID": "web"}`, Format: nomad.FormatJSON},
	}
	m, editor := editModel(t, client)

	_, cmd := m.update(key('e'))
	follow(m, cmd, 5)

	// A job submitted as JSON is JSON in the editor too: named .hcl, the
	// editor colors it and checks it as the wrong language.
	r.Equal(".json", filepath.Ext(editor.opened))
}

func TestEdit_KeepsTheVariables(t *testing.T) {
	r := require.New(t)

	vars := nomad.JobVariables{Flags: map[string]string{"image": "nginx:1.27"}, File: "count = 3\n"}

	client := &fakeClient{
		jobs: twoJobs(),
		spec: nomad.JobSource{Source: "job \"web\" {}", Format: "hcl2", Variables: vars},
	}
	m, editor := editModel(t, client)
	editor.replace = "job \"web\" {\n  type = \"batch\"\n}"

	m, cmd := m.update(key('e'))
	m = follow(m, cmd, 6)

	// The editor holds the file only. The values the job ran with go with
	// it, to the plan and to the submit, or the edit quietly changes them to
	// the defaults.
	r.Equal(vars, client.plannedVars)

	m, cmd = m.update(key('y'))
	drain(m, cmd)

	r.Equal(1, client.submitted)
	r.Equal(vars, client.submittedVars)
}

func TestEdit_KeepsTheCountsOfTheFileAsOpened(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		jobs:   twoJobs(),
		spec:   nomad.JobSource{Source: "job \"web\" {}"},
		counts: map[string]int{"web": 1},
	}
	m, editor := editModel(t, client)
	editor.replace = "job \"web\" {\n  type = \"batch\"\n}"

	m, cmd := m.update(key('e'))
	m = follow(m, cmd, 6)

	// The counts of the file as it was opened go to the plan and to the
	// submit: they tell which counts the editor changed.
	r.Equal(client.counts, client.openedWith)

	m, cmd = m.update(key('y'))
	drain(m, cmd)

	r.Equal(client.counts, client.submittedOpened)
}

func TestEdit_OpensAFileWhoseCountsCannotBeRead(t *testing.T) {
	r := require.New(t)

	// The cluster parses a job without the files it reads, so a file that
	// calls file() opens and is planned once the call is gone.
	client := &fakeClient{
		jobs:      twoJobs(),
		spec:      nomad.JobSource{Source: "job \"web\" {}", Index: 42},
		countsErr: errors.New("filesystem function disabled"),
		plan:      changedEnv(),
	}
	m, editor := editModel(t, client)
	editor.replace = "job \"web\" {\n  type = \"batch\"\n}"

	m, cmd := m.update(key('e'))
	m = follow(m, cmd, 6)

	r.NotEmpty(editor.opened)
	r.IsType(planPage{}, m.screen.page)
	r.Nil(client.openedWith)

	out := plain(m.render())
	r.Contains(out, "Counts as the file says them")
	r.Contains(out, "Submitting sets every count to the one in the file.")
}

func TestEdit_UnchangedFileChangesNothing(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), spec: nomad.JobSource{Source: "job \"web\" {}"}}
	m, _ := editModel(t, client)

	m, cmd := m.update(key('e'))
	m = follow(m, cmd, 5)

	// Leaving the editor without touching the file leaves the cluster alone.
	r.Zero(client.submitted)
	r.Contains(plain(m.render()), "unchanged")
}

// onNamespaces is the list of namespaces, with the editor at hand.
func onNamespaces(t *testing.T, client *fakeClient, editor *fakeEditor) Model {
	t.Helper()

	client.namespaces, client.namespaceSpec = twoNamespaces(), `{"Name": "production"}`

	m := New(client, Options{Namespace: "production", Version: "v-test", Editor: editor, PollEvery: time.Millisecond})
	m, _ = m.update(sizeMsg())

	m, _ = m.update(key(':'))
	m = typeIn(m, "ns")
	m, _ = m.update(enter())
	m, _ = m.update(namespacesMsg(twoNamespaces()))

	return m
}

func TestEdit_ANamespace(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	editor := &fakeEditor{replace: `{"Name": "production", "Description": "live"}`}
	m := onNamespaces(t, client, editor)

	m, cmd := m.update(key('e'))
	m = follow(m, cmd, 5)

	r.Equal(1, client.submittedNamespaces)
	r.Contains(plain(m.render()), "Namespace production submitted")
}

func TestEdit_WithoutAnEditor(t *testing.T) {
	r := require.New(t)

	// urga runs where EDITOR is not set, and shows an error instead of
	// doing nothing.
	m := newTestModel(&fakeClient{jobs: twoJobs()})
	m, _ = m.update(jobsMsg(twoJobs()))

	m, cmd := m.update(key('e'))
	m = follow(m, cmd, 5)

	r.Contains(plain(m.render()), "no editor")
}

func TestEdit_WhenTheClusterKeptNoSource(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		jobs:    twoJobs(),
		spec:    nomad.JobSource{Source: "{\n  \"ID\": \"web\",\n  \"Type\": \"service\"\n}", Format: nomad.FormatJSON},
		specErr: nomad.ErrNoSource,
	}

	m, editor := editModel(t, client)
	editor.replace = "{\n  \"ID\": \"web\",\n  \"Type\": \"batch\"\n}"

	m, cmd := m.update(key('e'))
	m = follow(m, cmd, 6)

	// Without the file it was submitted with, the job is edited as what the
	// cluster does have: its JSON. The sentence "no source kept" must never
	// reach the editor, because whatever comes back is submitted.
	r.Equal(".json", filepath.Ext(editor.opened))

	m, cmd = m.update(key('y'))
	drain(m, cmd)

	r.Equal(1, client.submitted)
	r.Contains(client.submittedSource, "\"Type\": \"batch\"")
}

func TestEdit_AJobWithoutASourceKeepsTheIndexItWasReadAt(t *testing.T) {
	r := require.New(t)

	read := "{\n  \"ID\": \"web\"\n}"
	client := &fakeClient{
		jobs:    twoJobs(),
		spec:    nomad.JobSource{Source: read, Format: nomad.FormatJSON, Index: 42},
		specErr: nomad.ErrNoSource,
		plan:    changedEnv(),
	}

	m, editor := editModel(t, client)
	editor.replace = "{\n  \"ID\": \"web\",\n  \"Type\": \"batch\"\n}"

	m, cmd := m.update(key('e'))
	m = follow(m, cmd, 6)

	// The JSON and its index come from one read, so a plan at that index
	// says nothing changed meanwhile.
	r.Equal(read, editor.seen[0])
	r.IsType(planPage{}, m.screen.page)
	r.NotContains(plain(m.render()), "Changed after the file was read")
}

func TestDescribe_JobSpecWhenTheClusterKeptNone(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), specErr: nomad.ErrNoSource}

	m, _ := editModel(t, client)

	m, cmd := m.update(key('h'))
	m = follow(m, cmd, 3)

	// Asking for the job file shows a message that there is none.
	out := plain(m.render())
	r.Contains(out, "kept no source")
	r.Zero(client.submitted)
}

func TestEdit_AFileThatNamesAnotherJobOpensTheEditorAgain(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		jobs:     twoJobs(),
		spec:     nomad.JobSource{Source: `job "web" {}`},
		refusals: []error{&nomad.OtherJob{File: "cron", Edited: "web"}},
	}
	m, editor := editModel(t, client)
	editor.edits = []string{`job "cron" {}`}

	m, cmd := m.update(key('e'))
	m = follow(m, cmd, 8)

	r.Equal("web", client.plannedJobID)
	r.IsType(jobsPage{}, m.screen.page)
	r.Len(editor.seen, 2)
	r.Contains(editor.seen[1], "# Not saved: the file names job \"cron\", not \"web\".\n# To make a new job, use n or :run.\n")
	r.Contains(editor.seen[1], `job "cron" {}`)
	r.Zero(client.submitted)
}

func TestEdit_AFileThatFailsToReadAfterLeavingShowsNoError(t *testing.T) {
	r := require.New(t)

	tempDir(t)

	client := &fakeClient{jobs: twoJobs(), specErr: errors.New("no such job")}
	m, editor := editModel(t, client)

	m, read := m.update(key('e'))
	m, _ = m.update(enter())
	m = drain(m, read)

	r.IsType(allocationsPage{}, m.screen.page)
	r.NotContains(plain(m.render()), "no such job")
	r.Empty(editor.seen)
}

func TestEdit_TheFileOfAnEditThatArrivesAfterLeavingIsRemoved(t *testing.T) {
	r := require.New(t)

	tempDir(t)

	client := &fakeClient{jobs: twoJobs(), spec: nomad.JobSource{Source: "job \"web\" {}"}}
	m, editor := editModel(t, client)

	m, read := m.update(key('e'))
	late := read()

	path := answered(t, late).(editFileMsg).path
	r.FileExists(path)

	m, _ = m.update(enter())
	m, _ = m.update(late)

	r.NoFileExists(path)
	r.Empty(editor.seen)
	r.Empty(m.editing.path)
}

// keptTexts are the files urga left in dir.
func keptTexts(t *testing.T, dir string) []string {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join(dir, "urga-*"))
	require.NoError(t, err)

	texts := []string{}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		require.NoError(t, err)

		texts = append(texts, string(data))
	}

	return texts
}

// keptFile is the one file urga left in dir: its extension and its text.
func keptFile(t *testing.T, dir string) (string, string) {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join(dir, "urga-*"))
	require.NoError(t, err)
	require.Len(t, paths, 1)

	data, err := os.ReadFile(paths[0])
	require.NoError(t, err)

	return filepath.Ext(paths[0]), string(data)
}

func TestEdit_ARefusalAfterLeavingKeepsTheTextAndDoesNotReopen(t *testing.T) {
	r := require.New(t)

	dir := tempDir(t)

	client := &fakeClient{jobs: twoJobs(), refusals: []error{errors.New("syntax error")}}
	m, editor := editModel(t, client)
	editor.replace = "job \"web\" {\n  type = \"batch\"\n}"

	m, read := m.update(key('e'))
	m, edit := m.update(read())
	m, submit := m.update(edit())

	m, _ = m.update(enter())
	m = follow(m, submit, 4)

	r.Len(editor.seen, 1)
	r.IsType(allocationsPage{}, m.screen.page)
	r.Equal([]string{editor.replace}, keptTexts(t, dir))

	// The path of the kept text comes first and the temp dir of a test is
	// long: the reason is read from the message, not the status line.
	r.Regexp(`^Your text is in .+\. Not saved: syntax error\.$`, m.flash.text)
}

func TestEdit_ARefusedEditReopenedAfterLeavingKeepsTheTextAndDoesNotOpen(t *testing.T) {
	r := require.New(t)

	dir := tempDir(t)

	client := &fakeClient{jobs: twoJobs(), refusals: []error{errors.New("syntax error")}}
	m, editor := editModel(t, client)
	editor.replace = "job \"web\" {\n  type = \"batch\"\n}"

	m, read := m.update(key('e'))
	m, edit := m.update(read())
	m, submit := m.update(edit())

	m, reopen := m.update(submit())

	m, _ = m.update(enter())
	m = follow(m, reopen, 3)

	r.Len(editor.seen, 1)
	r.IsType(allocationsPage{}, m.screen.page)

	texts := keptTexts(t, dir)
	r.Len(texts, 1)
	r.Contains(texts[0], editor.replace)
	r.Contains(texts[0], "syntax error")
	r.Regexp(`^Your text is in .+\. The edit was not opened again: the screen changed\.$`, m.flash.text)
}

func TestEdit_ARefusalAfterLeavingTheTokenExplainsSaysWhy(t *testing.T) {
	r := require.New(t)

	tempDir(t)

	m, _ := onAllocations(t)
	m, _ = m.update(tokenMsg(deployBot))

	refused := refusedEditMsg{err: forbidden(t), again: file{extension: "hcl", content: "job \"web\" {}"}}
	m, _ = m.update(answerMsg{asked: m.asked - 1, wrote: true, msg: refused})

	r.Regexp(`^Your text is in .+\. Not saved: Permission denied: deploy-bot may not do this\.$`, m.flash.text)
}

func TestEdit_ALateEditWhoseTextCannotBeKeptSaysWhy(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	text := "job \"web\" {}"

	for name, tc := range map[string]struct {
		late tea.Msg
		then string
	}{
		"plan": {
			openMsg{page: planPage{planState: planState{jobID: "web", source: text}}},
			"The plan of web was not opened: the screen changed.",
		},
		"refusal": {
			refusedEditMsg{err: errors.New("syntax error"), again: file{extension: "hcl", content: text}},
			"Not saved: syntax error.",
		},
	} {
		t.Run(name, func(t *testing.T) {
			useTempDir(t, missing)

			m, _ := onAllocations(t)
			m, _ = m.update(answerMsg{asked: m.asked - 1, wrote: true, msg: tc.late})

			why := `open ` + regexp.QuoteMeta(missing) + `.+: .+`
			require.Regexp(t, `^Your text could not be kept: `+why+`\. `+regexp.QuoteMeta(tc.then)+`$`, m.flash.text)
			require.Equal(t, flashErr, m.flash.level)
		})
	}
}

func TestEdit_TheFileThatKeepsALateEditFitsOnTheStatusLine(t *testing.T) {
	text := "job \"web\" {\n  type = \"batch\"\n}"
	refused := errors.New("<stdin>:1,1-10: Missing required argument; The argument \"group\" is required, but no definition was found")

	// The message for a file in the temp dir of macOS, the longest of the
	// usual ones.
	onMacOS := "Your text is in /var/folders/xx/" + strings.Repeat("x", 30) + "/T/urga-1234567890.hcl."

	for name, late := range map[string]func(t *testing.T, dir string) tea.Msg{
		"plan": func(*testing.T, string) tea.Msg {
			return openMsg{page: planPage{planState: planState{jobID: "web", source: text}}}
		},
		"refusal": func(*testing.T, string) tea.Msg {
			return refusedEditMsg{err: refused, again: file{extension: "hcl", content: text}}
		},
		"reopened": func(t *testing.T, dir string) tea.Msg {
			path := filepath.Join(dir, "urga-1234567890.hcl")
			require.NoError(t, os.WriteFile(path, []byte(text), 0o600))

			return editFileMsg{path: path, edited: true}
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			dir := tempDir(t)

			m, _ := onAllocations(t)
			r.Equal(120, m.width)

			m, _ = m.update(answerMsg{asked: m.asked - 1, wrote: true, msg: late(t, dir)})

			paths, err := filepath.Glob(filepath.Join(dir, "urga-*"))
			r.NoError(err)
			r.Len(paths, 1)

			said := m.flash.text
			r.True(strings.HasPrefix(said, "Your text is in "+paths[0]+"."), said)

			// The temp dir of a test is longer: the status line shows as much
			// of the message as the path on macOS takes.
			r.Greater(len(said), len(onMacOS))
			r.Contains(plain(m.render()), said[:len(onMacOS)])
		})
	}
}
