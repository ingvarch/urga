package ui

import (
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
	m, _ = m.show(screenVariables)
	m, _ = m.update(variablesMsg(client.variables))

	m, cmd := m.update(enter())

	return drain(m, cmd)
}

func TestVariables_EnterOpensTheValuesHidden(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onVariable(t, client, webVariable())

	r.Equal(screenVariable, m.screen.kind)
	r.Equal("nomad/jobs/web", client.variablePath)
	r.Equal("default", client.askedNamespace)

	out := plain(m.render())
	r.Contains(out, "Variable nomad/jobs/web (default) [3]")

	// By key, and nothing of a value on the screen until it is asked for.
	r.Contains(fileRow(t, m, 0), "CERT")
	r.Contains(fileRow(t, m, 1), "DB_HOST")
	r.Contains(fileRow(t, m, 2), "DB_PASSWORD")
	r.Contains(fileRow(t, m, 2), "••••••••")
	r.NotContains(out, "s3cr3t")
	r.NotContains(out, "10.0.0.5")
	r.NotContains(out, "BEGIN")

	// A value of several lines says so.
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
	r.Equal(screenVariables, m.screen.kind)

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
	m, _ = m.show(screenVariables)
	m, _ = m.update(variablesMsg(client.variables))

	r.Contains(fileRow(t, m, -1), "Lock")
	r.Contains(fileRow(t, m, 0), "874ae5d0")
	r.NotContains(fileRow(t, m, 0), leaderLock)
	r.NotContains(fileRow(t, m, 1), "874ae5d0")
}
