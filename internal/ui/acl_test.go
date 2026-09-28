package ui

import (
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

	r.Equal([]hint{{Key: "<d>", Description: "Describe"}, {Key: "<c>", Description: "Copy Secret"}}, m.hints())

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
	r.Equal([]hint{{Key: "<d>", Description: "Describe"}}, m.hints())
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
