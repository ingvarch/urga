package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

const (
	leaderLock = "874ae5d0-f47a-afb0-804f-fcdb04a14a0b"
	webCert    = "-----BEGIN CERTIFICATE-----\nMIIBfake\nline2\n-----END CERTIFICATE-----\n"
)

// webVariable holds what a job reads: a host, a password and a certificate
// of several lines.
func webVariable() nomad.VariableDetail {
	return nomad.VariableDetail{
		Variable: nomad.Variable{Path: "nomad/jobs/web", Namespace: "default"},
		Items:    map[string]string{"DB_HOST": "10.0.0.5", "DB_PASSWORD": "s3cr3t", "CERT": webCert},
	}
}

// leaderVariable is held as a lock.
func leaderVariable() nomad.VariableDetail {
	return nomad.VariableDetail{
		Variable: nomad.Variable{Path: "locks/leader", Namespace: "default",
			Lock: &nomad.VariableLock{ID: leaderLock, TTL: "30m0s", Delay: "15s"}},
		Items: map[string]string{"owner": "web-1"},
	}
}

// onVariable is a variable opened from the list of variables.
func onVariable(t *testing.T, client *fakeClient, detail nomad.VariableDetail) Model {
	t.Helper()

	client.variables = []nomad.Variable{detail.Variable}
	client.variable = detail

	m := newTestModel(client)
	m, _ = m.show(variablesView)
	m, _ = m.update(variablesMsg(client.variables))

	m, cmd := m.update(enter())

	return drain(m, cmd)
}

func TestVariables_EnterOpensTheValuesHidden(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onVariable(t, client, webVariable())

	r.IsType(variablePage{}, m.screen.page)
	r.Equal("nomad/jobs/web", client.variablePath)
	r.Equal("default", client.askedNamespace)

	out := plain(m.render())
	r.Contains(out, "Variable nomad/jobs/web (default) [3]")

	// Listed by key, and no value is on the screen until it is asked for.
	r.Contains(fileRow(t, m, 0), "CERT")
	r.Contains(fileRow(t, m, 1), "DB_HOST")
	r.Contains(fileRow(t, m, 2), "DB_PASSWORD")
	r.Contains(fileRow(t, m, 2), "••••••••")
	r.NotContains(out, "s3cr3t")
	r.NotContains(out, "10.0.0.5")
	r.NotContains(out, "BEGIN")

	// A value of several lines shows how many.
	r.Contains(fileRow(t, m, 0), "(4 lines)")
}

func TestVariable_VShowsTheValues(t *testing.T) {
	r := require.New(t)

	m := onVariable(t, &fakeClient{}, webVariable())

	m, _ = m.update(key('v'))

	r.Contains(fileRow(t, m, 0), "-----BEGIN CERTIFICATE----- (4 lines)")
	r.Contains(fileRow(t, m, 1), "10.0.0.5")
	r.Contains(fileRow(t, m, 2), "s3cr3t")

	m, _ = m.update(key('v'))
	r.NotContains(plain(m.render()), "s3cr3t")
}

func TestVariable_OpenedAgainItIsHidden(t *testing.T) {
	r := require.New(t)

	m := onVariable(t, &fakeClient{}, webVariable())
	m, _ = m.update(key('v'))

	m, _ = m.update(escape())
	r.IsType(variablesPage{}, m.screen.page)

	m, cmd := m.update(enter())
	m = drain(m, cmd)

	r.NotContains(plain(m.render()), "s3cr3t")
}

func TestVariable_CopyTakesTheValueHiddenOrNot(t *testing.T) {
	r := require.New(t)

	m := onVariable(t, &fakeClient{}, webVariable())

	next, cmd := m.update(key('c'))
	r.Equal(webCert, clipboardOf(cmd))
	r.Contains(plain(next.render()), "Copied CERT.")

	m, _ = m.update(key('j'))
	m, _ = m.update(key('j'))

	_, cmd = m.update(key('c'))
	r.Equal("s3cr3t", clipboardOf(cmd))
}

func TestVariable_AnswerForAnotherVariableIsDropped(t *testing.T) {
	r := require.New(t)

	m := onVariable(t, &fakeClient{}, webVariable())

	other := leaderVariable()
	m, _ = m.update(variableMsg(other))

	r.Contains(fileRow(t, m, 0), "CERT")
	r.NotContains(plain(m.render()), "owner")
}

func TestVariable_SaysWhoHoldsTheLock(t *testing.T) {
	r := require.New(t)

	m := onVariable(t, &fakeClient{}, leaderVariable())

	out := plain(m.render())
	r.Contains(out, "Lock "+leaderLock)
	r.Contains(out, "TTL 30m0s")
	r.Contains(out, "Delay 15s")
	r.Contains(fileRow(t, m, 0), "owner")

	// A variable nobody holds has no such line.
	m = onVariable(t, &fakeClient{}, webVariable())
	r.NotContains(plain(m.render()), "TTL")
}

func TestVariables_ListSaysWhoHoldsTheLock(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{variables: []nomad.Variable{leaderVariable().Variable, webVariable().Variable}}

	m := newTestModel(client)
	m, _ = m.show(variablesView)
	m, _ = m.update(variablesMsg(client.variables))

	r.Contains(fileRow(t, m, -1), "Lock")
	r.Contains(fileRow(t, m, 0), "874ae5d0")
	r.NotContains(fileRow(t, m, 0), leaderLock)
	r.NotContains(fileRow(t, m, 1), "874ae5d0")
}

func TestVariables_TheListOfTheNamespaceAndItsKeys(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{variables: []nomad.Variable{leaderVariable().Variable, webVariable().Variable}}
	m := typeCommand(newTestModel(client), "variables")

	// Asked in the namespace of the session, and titled with it.
	r.Equal("production", client.askedNamespace)
	r.Contains(plain(m.render()), "Variables (production) [2]")

	header := fileRow(t, m, -1)
	for _, title := range []string{"Path", "Namespace", "Lock", "Age", "Modified"} {
		r.Contains(header, title)
	}

	r.Contains(fileRow(t, m, 1), "nomad/jobs/web")
	r.Contains(fileRow(t, m, 1), "default")

	// The locked one has no edit, the other one has.
	r.Equal([]hint{{Key: "<enter>", Description: "Values"}}, m.hints())

	m, _ = m.update(key('j'))
	r.Equal([]hint{{Key: "<enter>", Description: "Values"}, {Key: "<e>", Description: "Edit"}}, m.hints())
}

func TestVariable_TheKeysOfTheValues(t *testing.T) {
	r := require.New(t)

	m := onVariable(t, &fakeClient{}, webVariable())
	r.Equal([]hint{
		{Key: "<v>", Description: "Toggle Values"},
		{Key: "<c>", Description: "Copy"},
		{Key: "<e>", Description: "Edit"},
	}, m.hints())

	m = onVariable(t, &fakeClient{}, leaderVariable())
	r.Equal([]hint{{Key: "<v>", Description: "Toggle Values"}, {Key: "<c>", Description: "Copy"}}, m.hints())
}

func TestVariables_AListThatAnswersLateIsDropped(t *testing.T) {
	r := require.New(t)

	m := onVariable(t, &fakeClient{}, webVariable())

	// The list answers while a variable of it is open.
	m, _ = m.update(variablesMsg{leaderVariable().Variable})
	r.Contains(fileRow(t, m, 0), "CERT")

	// Back on the list, it shows the rows it had before.
	m, _ = m.update(escape())

	out := plain(m.render())
	r.Contains(out, "nomad/jobs/web")
	r.NotContains(out, "locks/leader")
}

func TestVariables_OfTheRegionLeftAreLetGo(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{variables: []nomad.Variable{webVariable().Variable}}
	m := regionalModel(t, client)
	m = typeCommand(m, "variables")
	r.Contains(plain(m.render()), "nomad/jobs/web")

	// The variables of eu must not show under the name of us before us
	// answers.
	client.variables = nil
	m, _ = runLine(m, "region us")

	r.Contains(plain(m.render()), "Variables (production) [0]")
	r.NotContains(plain(m.render()), "nomad/jobs/web")
}

// webFile is the web variable as the editor gets it.
const webFile = "# Variable nomad/jobs/web in namespace default.\nDB_HOST = \"10.0.0.5\"\n"

// editingWeb is the web variable open with a fake editor, and a cluster that
// returns it as a file read at index 769. The editor returns the given edits
// one by one.
func editingWeb(t *testing.T, client *fakeClient, edits ...string) (Model, *fakeEditor) {
	t.Helper()

	t.Setenv("TMPDIR", t.TempDir())

	client.variableSpec = nomad.VariableSource{Source: webFile, Index: 769}

	m := onVariable(t, client, webVariable())
	editor := &fakeEditor{edits: edits}
	m.opts.Editor = editor

	return m, editor
}

func TestVariable_EditSavesWithCheckAndSet(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m, editor := editingWeb(t, client, "DB_HOST = \"10.0.0.6\"\n")

	m, cmd := m.update(key('e'))
	m = follow(m, cmd, 8)

	r.Equal([]string{webFile}, editor.seen)
	r.True(strings.HasSuffix(editor.opened, ".toml"))

	// Saved at the index it was read at, and nothing else is written.
	r.Equal([]string{"SubmitVariable"}, client.writes)
	r.Equal("default", client.askedNamespace)
	r.Equal("nomad/jobs/web", client.variablePath)
	r.Equal("DB_HOST = \"10.0.0.6\"\n", client.submittedSource)
	r.Equal(uint64(769), client.submittedIndex)
	r.Contains(plain(m.render()), "Variable nomad/jobs/web saved.")
}

func TestVariables_EditFromTheList(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m, _ := editingWeb(t, client, "DB_HOST = \"10.0.0.6\"\n")

	m, _ = m.update(escape())
	r.IsType(variablesPage{}, m.screen.page)

	m, cmd := m.update(key('e'))
	follow(m, cmd, 8)

	r.Equal("nomad/jobs/web", client.variablePath)
	r.Equal("DB_HOST = \"10.0.0.6\"\n", client.submittedSource)
}

func TestVariable_NoEditOfALockedVariable(t *testing.T) {
	r := require.New(t)

	// Nomad refuses a change to anyone but the holder of the lock.
	m := onVariable(t, &fakeClient{}, leaderVariable())

	r.False(offers(m, "e"))
	r.Contains(plain(m.render()), "Only the holder of the lock can change it.")

	m, _ = m.update(escape())
	r.False(offers(m, "e"))

	m = onVariable(t, &fakeClient{}, webVariable())
	r.True(offers(m, "e"))
	r.NotContains(plain(m.render()), "Only the holder")
}

func TestVariable_ARefusedSaveOpensTheEditAgain(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{refusals: []error{errors.New("the variable is not valid TOML: line 2: expected value")}}
	m, editor := editingWeb(t, client, webFile+"DB_PORT = \n")

	m, cmd := m.update(key('e'))
	m = follow(m, cmd, 12)

	// The edit opens again as it was, with the reason at the top.
	r.Len(editor.seen, 2)
	r.Equal(reasonFor("the variable is not valid TOML: line 2: expected value")+webFile+"DB_PORT = \n", editor.seen[1])

	// Saved unchanged, it is dropped.
	r.Equal([]string{"SubmitVariable"}, client.writes)
	r.Contains(plain(m.render()), "unchanged")
}

func TestVariable_AChangedVariableIsSavedOverWhenAsked(t *testing.T) {
	r := require.New(t)

	edit := "DB_HOST = \"10.0.0.6\"\n"
	client := &fakeClient{refusals: []error{&nomad.VariableConflict{Path: "nomad/jobs/web", Index: 800}}}

	// The second time, the lines of the refusal are deleted.
	m, editor := editingWeb(t, client, edit, edit)

	m, cmd := m.update(key('e'))
	m = follow(m, cmd, 16)

	r.Len(editor.seen, 2)
	r.Contains(editor.seen[1], "# Not saved: variable nomad/jobs/web changed since it was read.\n# Saving again replaces that change.\n")

	r.Equal([]string{"SubmitVariable", "SubmitVariable"}, client.writes)
	r.Equal(uint64(800), client.submittedIndex)
	r.Contains(plain(m.render()), "Variable nomad/jobs/web saved.")
}

func TestVariable_ADeletedVariableIsCreatedWhenAsked(t *testing.T) {
	r := require.New(t)

	edit := "DB_HOST = \"10.0.0.6\"\n"
	client := &fakeClient{refusals: []error{&nomad.VariableConflict{Path: "nomad/jobs/web", Deleted: true}}}
	m, editor := editingWeb(t, client, edit, edit)

	m, cmd := m.update(key('e'))
	follow(m, cmd, 16)

	r.Contains(editor.seen[1], "# Saving again creates it.\n")

	// Index 0 creates it, and only if nobody did meanwhile.
	r.Equal(uint64(0), client.submittedIndex)
}

func TestVariable_ALockedVariableIsNotSavedOver(t *testing.T) {
	r := require.New(t)

	lock := &nomad.VariableLock{ID: leaderLock}
	edit := "DB_HOST = \"10.0.0.6\"\n"
	client := &fakeClient{refusals: []error{&nomad.VariableConflict{Path: "nomad/jobs/web", Lock: lock, Index: 800}}}
	m, editor := editingWeb(t, client, edit, edit)

	m, cmd := m.update(key('e'))
	follow(m, cmd, 16)

	// Saved again, it is sent at the index it was read at: the lock may be
	// gone by then, but the reason to stay out of the change may not.
	r.NotContains(editor.seen[1], "Saving again")
	r.Equal(uint64(769), client.submittedIndex)
}

func TestVariable_ARefusedSaveSaysWhoseTokenWasRefused(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{refusals: []error{forbidden(t)}}
	m, editor := editingWeb(t, client, "DB_HOST = \"10.0.0.6\"\n")
	m, _ = m.update(tokenMsg(nomad.Token{Name: "deploy-bot", Type: "client"}))

	m, cmd := m.update(key('e'))
	follow(m, cmd, 12)

	r.Contains(editor.seen[1], "# Not saved: Permission denied: deploy-bot may not do this.\n")
}
