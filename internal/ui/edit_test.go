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
}

func (e *fakeEditor) Edit(path string) tea.Cmd {
	return func() tea.Msg {
		e.opened = path

		if e.err != nil {
			return editedMsg{path: path, err: e.err}
		}

		if e.replace != "" {
			if err := os.WriteFile(path, []byte(e.replace), 0o600); err != nil {
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

	client := &fakeClient{jobs: twoJobs(), spec: "job \"web\" {\n  type = \"service\"\n}"}
	m, editor := editModel(t, client)
	editor.replace = "job \"web\" {\n  type = \"batch\"\n}"

	m, cmd := m.update(key('e'))
	m = follow(m, cmd, 5)

	// What was on the screen is what the editor was given.
	r.NotEmpty(editor.opened)
	r.Equal(".hcl", filepath.Ext(editor.opened))

	// What came back is submitted without asking again: saving was the
	// decision.
	r.Equal(1, client.submitted)
	r.Equal("job \"web\" {\n  type = \"batch\"\n}", client.submittedSource)
	r.Equal("production", client.askedNamespace)
	r.Contains(plain(m.render()), "Job web submitted")
}

func TestEdit_UnchangedFileChangesNothing(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), spec: "job \"web\" {}"}
	m, _ := editModel(t, client)

	m, cmd := m.update(key('e'))
	m = follow(m, cmd, 5)

	// Leaving the editor without touching the file leaves the cluster alone.
	r.Zero(client.submitted)
	r.Contains(plain(m.render()), "unchanged")
}

func TestEdit_ANamespace(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{namespaces: twoNamespaces(), namespaceSpec: `{"Name": "production"}`}

	editor := &fakeEditor{replace: `{"Name": "production", "Description": "live"}`}
	m := New(client, Options{Namespace: "production", Version: "v-test", Editor: editor, PollEvery: time.Millisecond})
	m, _ = m.update(sizeMsg())

	m, _ = m.update(key(':'))
	m = typeIn(m, "ns")
	m, _ = m.update(enter())
	m, _ = m.update(namespacesMsg(twoNamespaces()))

	m, cmd := m.update(key('e'))
	m = follow(m, cmd, 5)

	r.Equal(1, client.submittedNamespaces)
	r.Contains(plain(m.render()), "Namespace production submitted")
}

func TestEdit_WithoutAnEditor(t *testing.T) {
	r := require.New(t)

	// urga runs where EDITOR is not set, and says so instead of doing
	// nothing.
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

	_, cmd := m.update(key('e'))
	follow(m, cmd, 5)

	// Without the file it was submitted with, the job is edited as what the
	// cluster does have: its JSON. The sentence "no source kept" must never
	// reach the editor, because whatever comes back is submitted.
	r.Equal(".json", filepath.Ext(editor.opened))
	r.Equal(1, client.submitted)
	r.Contains(client.submittedSource, "\"Type\": \"batch\"")
}

func TestDescribe_JobSpecWhenTheClusterKeptNone(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), specErr: nomad.ErrNoSource}

	m, _ := editModel(t, client)

	m, cmd := m.update(key('h'))
	m = follow(m, cmd, 3)

	// Asking for the job file says plainly that there is none.
	out := plain(m.render())
	r.Contains(out, "kept no source")
	r.Zero(client.submitted)
}
