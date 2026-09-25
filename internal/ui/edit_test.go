package ui

import (
	"os"
	"path/filepath"
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
}

func (e *fakeEditor) Edit(path string) tea.Cmd {
	return func() tea.Msg {
		e.opened = path

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
		jobs:     twoJobs(),
		specErr:  nomad.ErrNoSource,
		describe: "{\n  \"ID\": \"web\",\n  \"Type\": \"service\"\n}",
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
