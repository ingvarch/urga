package ui

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// aclObjects are one object of each kind.
func aclObjects() map[string][]nomad.ACLObject {
	return map[string][]nomad.ACLObject{
		nomad.ACLToken: {{
			Kind: nomad.ACLToken, ID: "a1b2c3d4-0000-0000-0000-000000000000", Name: "deployer", Type: "client",
			Policies: []string{"deploy", "read"}, Roles: []string{"ops"}, Global: true,
			Created: time.Now().Add(-72 * time.Hour), Expires: time.Now().Add(50 * time.Hour),
		}},
		nomad.ACLPolicy: {{Kind: nomad.ACLPolicy, ID: "read", Name: "read", Description: "Read everything"}},
		nomad.ACLRole: {{
			Kind: nomad.ACLRole, ID: "r1", Name: "ops", Description: "Operators", Policies: []string{"deploy", "read"},
		}},
		nomad.ACLAuthMethod:  {{Kind: nomad.ACLAuthMethod, ID: "okta", Name: "okta", Type: "OIDC", Default: true}},
		nomad.ACLBindingRule: {{Kind: nomad.ACLBindingRule, ID: "b1c2d3e4-0000-0000-0000-000000000000", Description: "Engineers", AuthMethod: "okta"}},
	}
}

// onACL is the list a command opens.
func onACL(t *testing.T, client *fakeClient, command string) Model {
	t.Helper()

	client.acl = aclObjects()

	return typeCommand(newTestModel(client), command)
}

func TestACL_TheLists(t *testing.T) {
	r := require.New(t)

	for command, want := range map[string]struct {
		title string
		cells []string
	}{
		"tokens":       {"Tokens [1]", []string{"a1b2c3d4", "deployer", "client", "deploy, read", "ops", "yes", "in 2d", "3d"}},
		"policies":     {"Policies [1]", []string{"read", "Read everything"}},
		"roles":        {"Roles [1]", []string{"ops", "Operators", "deploy, read"}},
		"authmethods":  {"Auth Methods [1]", []string{"okta", "OIDC", "yes"}},
		"bindingrules": {"Binding Rules [1]", []string{"b1c2d3e4", "okta", "Engineers"}},
	} {
		m := onACL(t, &fakeClient{}, command)

		r.IsType(aclPage{}, m.screen.page, command)
		r.Equal(want.title, m.title(), command)
		r.Equal(want.cells, m.rows()[0].cells, command)
	}
}

func TestACL_TheirOtherNames(t *testing.T) {
	r := require.New(t)

	for command, kind := range map[string]string{
		"token":       nomad.ACLToken,
		"policy":      nomad.ACLPolicy,
		"pol":         nomad.ACLPolicy,
		"role":        nomad.ACLRole,
		"authmethod":  nomad.ACLAuthMethod,
		"auth":        nomad.ACLAuthMethod,
		"bindingrule": nomad.ACLBindingRule,
		"br":          nomad.ACLBindingRule,
	} {
		m := onACL(t, &fakeClient{}, command)

		page, ok := m.screen.page.(aclPage)
		r.True(ok, command)
		r.Equal(kind, page.kind, command)
	}
}

func TestACL_Describe(t *testing.T) {
	r := require.New(t)

	for command, want := range map[string]struct{ title, asked string }{
		"policies":     {"Policy: read", "policy/read"},
		"bindingrules": {"Binding rule: b1c2d3e4", "binding rule/b1c2d3e4-0000-0000-0000-000000000000"},
	} {
		client := &fakeClient{describe: "{}"}
		m := onACL(t, client, command)

		m, cmd := m.update(key('d'))
		m = playOut(m, cmd)

		r.IsType(describePage{}, m.screen.page, command)
		r.Equal(want.title, m.title(), command)
		r.Equal(want.asked, client.aclOf, command)
	}
}

func TestACL_CopyTheSecretOfAToken(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{secret: "s3cr3t-0000"}
	m := onACL(t, client, "tokens")

	r.Equal([]hint{
		{Key: "<d>", Description: "Describe"},
		{Key: "<c>", Description: "Copy Secret"},
		{Key: "<n>", Description: "New"},
		{Key: "<e>", Description: "Edit"},
		{Key: "<ctrl-d>", Description: "Delete"},
	}, m.hints())

	m, cmd := m.update(key('c'))
	m = playOut(m, cmd)

	// The secret goes to the clipboard, not to the screen.
	r.Equal(nomad.ACLToken+"/a1b2c3d4-0000-0000-0000-000000000000", client.aclOf)
	r.Contains(plain(statusLine(m)), "Copied the secret of the token deployer.")
	r.NotContains(plain(m.render()), "s3cr3t")
}

func TestACL_OnlyATokenHasASecret(t *testing.T) {
	r := require.New(t)

	m := onACL(t, &fakeClient{}, "policies")
	r.Equal([]hint{
		{Key: "<d>", Description: "Describe"},
		{Key: "<n>", Description: "New"},
		{Key: "<e>", Description: "Edit"},
		{Key: "<ctrl-d>", Description: "Delete"},
	}, m.hints())
}

func TestTokenColor(t *testing.T) {
	r := require.New(t)

	now := time.Now()

	// The same measure as the token of the session in the header.
	r.Equal(colorDead, tokenColor(now.Add(-time.Hour), now))
	r.Equal(colorDead, tokenColor(now.Add(2*24*time.Hour), now))
	r.Equal(colorAttention, tokenColor(now.Add(10*24*time.Hour), now))
	r.Equal(colorPending, tokenColor(now.Add(20*24*time.Hour), now))
	r.Nil(tokenColor(now.Add(40*24*time.Hour), now))

	// One that never expires.
	r.Nil(tokenColor(time.Time{}, now))
}

func TestACL_TheObjectsOfAnotherKindAreNotTaken(t *testing.T) {
	r := require.New(t)

	m := onACL(t, &fakeClient{}, "policies")

	m, _ = m.update(aclMsg{kind: nomad.ACLToken, objects: aclObjects()[nomad.ACLToken]})

	r.Equal([]string{"read", "Read everything"}, m.rows()[0].cells)
}

// withEditor is the list a command opens, with an editor that types edits
// in turn.
func withEditor(t *testing.T, client *fakeClient, command string, edits ...string) (Model, *fakeEditor) {
	t.Helper()

	m := onACL(t, client, command)
	editor := &fakeEditor{edits: edits}
	m.opts.Editor = editor

	return m, editor
}

func TestACL_EditAnObject(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{describe: `{"ID": "r1", "Name": "ops"}`, aclWritten: nomad.ACLWritten{ID: "r1", Name: "ops"}}
	m, editor := withEditor(t, client, "roles", `{"ID": "r1", "Name": "ops", "Description": "On call"}`)

	m, cmd := m.update(key('e'))
	m = playOut(m, cmd)

	// The file is the object as describe shows it.
	r.Equal([]string{`{"ID": "r1", "Name": "ops"}`}, editor.seen)
	r.Equal("role/r1", client.submittedACL)
	r.Contains(client.aclSource, "On call")
	r.Contains(plain(m.render()), "Role ops saved.")
}

func TestACL_ANewObject(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{aclWritten: nomad.ACLWritten{ID: "r2", Name: "oncall"}}
	m, editor := withEditor(t, client, "roles", `{"Name": "oncall"}`)

	m, cmd := m.update(key('n'))
	m = playOut(m, cmd)

	// It starts from the fields of its kind, and has no ID until it is
	// written.
	r.Equal([]string{nomad.ACLTemplate(nomad.ACLRole)}, editor.seen)
	r.Equal("role/", client.submittedACL)
	r.Contains(plain(m.render()), "Role oncall created.")
}

func TestACL_ANewPolicyIsNamedFirst(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{aclWritten: nomad.ACLWritten{ID: "deploy", Name: "deploy"}}
	m, editor := withEditor(t, client, "policies", "# Description: Deploy\nnamespace \"default\" {}\n")

	// A policy is named by its name, which its file does not hold.
	m, _ = m.update(key('n'))
	r.Equal(overlayAnswer, m.overlay)
	r.Contains(plain(m.render()), "new policy named:")

	m = typeIn(m, "deploy")
	m, cmd := m.update(enter())
	m = playOut(m, cmd)

	r.Equal([]string{nomad.ACLTemplate(nomad.ACLPolicy)}, editor.seen)
	r.Equal("policy/deploy", client.submittedACL)
	r.Contains(plain(m.render()), "Policy deploy created.")
}

func TestACL_APolicyNeedsAName(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m, editor := withEditor(t, client, "policies")

	m, _ = m.update(key('n'))
	m, cmd := m.update(enter())
	m = playOut(m, cmd)

	r.Empty(editor.seen)
	r.Contains(plain(statusLine(m)), "A policy needs a name.")
}

func TestACL_ANewTokenShowsItsSecretOnce(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{aclWritten: nomad.ACLWritten{ID: "a9b8c7d6-0000-0000-0000-000000000000", Name: "ci", Secret: "s3cr3t-0000"}}
	m, _ := withEditor(t, client, "tokens", `{"Name": "ci", "Type": "client", "Policies": ["read"]}`)

	m, cmd := m.update(key('n'))
	m = playOut(m, cmd)

	// The one time the secret is drawn: it is what the new token is for.
	r.IsType(describePage{}, m.screen.page)
	r.Equal("Token ci created", m.title())

	out := plain(m.render())
	r.Contains(out, "a9b8c7d6-0000-0000-0000-000000000000")
	r.Contains(out, "s3cr3t-0000")
	r.Contains(out, "Copied the secret of the token ci.")
}

func TestACL_ARefusedFileOpensAgain(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		describe:   `{"AccessorID": "a1"}`,
		refusals:   []error{errors.New("ACL token accessor does not match request path")},
		aclWritten: nomad.ACLWritten{ID: "a1", Name: "deployer"},
	}
	m, editor := withEditor(t, client, "tokens", `{"Name": "deployer"}`, `{"AccessorID": "a1", "Name": "deployer"}`)

	m, cmd := m.update(key('e'))
	playOut(m, cmd)

	r.Len(editor.seen, 2)
	r.Contains(editor.seen[1], "# Not saved: ACL token accessor does not match request path.")
	r.Equal([]string{"SubmitACL", "SubmitACL"}, client.writes)
}

func TestACL_Delete(t *testing.T) {
	r := require.New(t)

	for command, want := range map[string]struct{ question, deleted, said string }{
		"policies": {"Really delete the policy read?", "policy/read", "Policy read deleted."},
		"tokens": {
			"Really delete the token deployer? Whoever uses it loses access.",
			"token/a1b2c3d4-0000-0000-0000-000000000000", "Token deployer deleted.",
		},
		"bindingrules": {"Really delete the binding rule b1c2d3e4?", "binding rule/b1c2d3e4-0000-0000-0000-000000000000", "Binding rule b1c2d3e4 deleted."},
	} {
		client := &fakeClient{}
		m := onACL(t, client, command)

		m, _ = m.update(ctrlKey('d'))
		r.Contains(plain(m.render()), want.question, command)

		m, cmd := m.update(key('y'))
		m = playOut(m, cmd)

		r.Equal(want.deleted, client.deletedACL, command)
		r.Contains(plain(m.render()), want.said, command)
	}
}

func TestACL_AChangedTokenDoesNotShowItsSecret(t *testing.T) {
	r := require.New(t)

	// The cluster answers a change of a token with its secret too.
	client := &fakeClient{
		describe:   `{"AccessorID": "a1"}`,
		aclWritten: nomad.ACLWritten{ID: "a1", Name: "deployer", Secret: "s3cr3t-0000"},
	}
	m, _ := withEditor(t, client, "tokens", `{"AccessorID": "a1", "Name": "deployer"}`)

	m, cmd := m.update(key('e'))
	m = playOut(m, cmd)

	r.IsType(aclPage{}, m.screen.page)
	r.NotContains(plain(m.render()), "s3cr3t")
	r.Contains(plain(m.render()), "Token deployer saved.")
}
