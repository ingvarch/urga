package nomad_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// aclCluster answers every ACL list and one object of each kind.
func aclCluster(t *testing.T) (*nomad.Client, *[]sent) {
	t.Helper()

	return clusterServer(t, map[string]string{
		"/v1/acl/tokens": `[{
			"AccessorID": "a1b2c3d4-0000-0000-0000-000000000000", "Name": "deployer", "Type": "client",
			"Policies": ["deploy", "read"], "Roles": [{"ID": "r1", "Name": ""}], "Global": true,
			"CreateTime": "2026-09-01T10:00:00Z", "ExpirationTime": "2026-10-01T10:00:00Z"
		}]`,
		"/v1/acl/policies":      `[{"Name": "read", "Description": "Read everything"}]`,
		"/v1/acl/roles":         `[{"ID": "r1", "Name": "ops", "Description": "Operators", "Policies": [{"Name": "deploy"}, {"Name": "read"}]}]`,
		"/v1/acl/auth-methods":  `[{"Name": "okta", "Type": "OIDC", "Default": true}]`,
		"/v1/acl/binding-rules": `[{"ID": "b1", "Description": "Engineers", "AuthMethod": "okta"}]`,

		"/v1/acl/token/a1b2c3d4-0000-0000-0000-000000000000": `{"AccessorID": "a1b2c3d4-0000-0000-0000-000000000000", "SecretID": "s3cr3t", "Name": "deployer"}`,
		"/v1/acl/policy/read":                                `{"Name": "read", "Description": "Read everything", "Rules": "namespace \"*\" {\n  policy = \"read\"\n}\n"}`,
		"/v1/acl/role/r1":                                    `{"ID": "r1", "Name": "ops", "Policies": [{"Name": "deploy"}]}`,
	})
}

func TestACLObjects(t *testing.T) {
	r := require.New(t)

	client, _ := aclCluster(t)

	for kind, want := range map[string]nomad.ACLObject{
		nomad.ACLToken: {
			Kind: nomad.ACLToken, ID: "a1b2c3d4-0000-0000-0000-000000000000", Name: "deployer", Type: "client",
			Policies: []string{"deploy", "read"}, Roles: []string{"ops"}, Global: true,
			Created: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC), Expires: time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC),
		},
		nomad.ACLPolicy: {Kind: nomad.ACLPolicy, ID: "read", Name: "read", Description: "Read everything"},
		nomad.ACLRole: {
			Kind: nomad.ACLRole, ID: "r1", Name: "ops", Description: "Operators", Policies: []string{"deploy", "read"},
		},
		nomad.ACLAuthMethod:  {Kind: nomad.ACLAuthMethod, ID: "okta", Name: "okta", Type: "OIDC", Default: true},
		nomad.ACLBindingRule: {Kind: nomad.ACLBindingRule, ID: "b1", Description: "Engineers", AuthMethod: "okta"},
	} {
		objects, err := client.ACLObjects(context.Background(), kind)
		r.NoError(err, kind)
		r.Len(objects, 1, kind)

		got := objects[0]
		got.Created, got.Expires = got.Created.UTC(), got.Expires.UTC()
		r.Equal(want, got, kind)
	}
}

func TestACLObjects_TheRolesOfATokenAreNamed(t *testing.T) {
	r := require.New(t)

	client, asked := aclCluster(t)

	// The list of tokens names a role by its ID only; the list of roles
	// has its name. It is asked for when a token names a role that way.
	tokens, err := client.ACLObjects(context.Background(), nomad.ACLToken)
	r.NoError(err)
	r.Equal([]string{"ops"}, tokens[0].Roles)

	paths := []string{}
	for _, req := range *asked {
		paths = append(paths, req.path)
	}

	r.Equal([]string{"/v1/acl/tokens", "/v1/acl/roles"}, paths)
}

func TestACLObjects_AKindThatIsNot(t *testing.T) {
	r := require.New(t)

	client, _ := aclCluster(t)

	_, err := client.ACLObjects(context.Background(), "sentinel")
	r.ErrorContains(err, "sentinel")
}

func TestDescribeACL(t *testing.T) {
	r := require.New(t)

	client, _ := aclCluster(t)

	out, err := client.DescribeACL(context.Background(), nomad.ACLRole, "r1")
	r.NoError(err)
	r.Contains(out, `"Name": "ops"`)

	// A token is described without its secret: describing is reading, and
	// the screen may be shared.
	out, err = client.DescribeACL(context.Background(), nomad.ACLToken, "a1b2c3d4-0000-0000-0000-000000000000")
	r.NoError(err)
	r.Contains(out, `"Name": "deployer"`)
	r.NotContains(out, "SecretID")
	r.NotContains(out, "s3cr3t")
}

func TestDescribeACL_APolicyIsItsFile(t *testing.T) {
	r := require.New(t)

	client, _ := aclCluster(t)

	out, err := client.DescribeACL(context.Background(), nomad.ACLPolicy, "read")
	r.NoError(err)

	// Its description, then its rules as they are written.
	r.Equal("# Description: Read everything\nnamespace \"*\" {\n  policy = \"read\"\n}\n", out)
}

func TestTokenSecret(t *testing.T) {
	r := require.New(t)

	client, _ := aclCluster(t)

	secret, err := client.TokenSecret(context.Background(), "a1b2c3d4-0000-0000-0000-000000000000")
	r.NoError(err)
	r.Equal("s3cr3t", secret)
}

func TestSubmitACL_ANewToken(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{
		"/v1/acl/token": `{"AccessorID": "a9", "SecretID": "s3cr3t", "Name": "ci"}`,
	})

	written, err := client.SubmitACL(context.Background(), nomad.ACLToken, "", `{"Name": "ci", "Type": "client", "Policies": ["read"]}`)
	r.NoError(err)

	// Created where the kind is created, as typed, and back with its secret.
	r.Equal("/v1/acl/token", (*asked)[0].path)
	r.Equal("ci", (*asked)[0].body["Name"])
	r.Equal(nomad.ACLWritten{ID: "a9", Name: "ci", Secret: "s3cr3t"}, written)
}

func TestSubmitACL_AnObjectChanged(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{
		"/v1/acl/role/r1": `{"ID": "r1", "Name": "ops"}`,
	})

	written, err := client.SubmitACL(context.Background(), nomad.ACLRole, "r1", `{"ID": "r1", "Name": "ops", "Policies": [{"Name": "read"}]}`)
	r.NoError(err)

	r.Equal("/v1/acl/role/r1", (*asked)[0].path)
	r.Equal(nomad.ACLWritten{ID: "r1", Name: "ops"}, written)
}

func TestSubmitACL_APolicyIsItsFile(t *testing.T) {
	r := require.New(t)

	// The cluster answers a policy with nothing.
	client, asked := clusterServer(t, map[string]string{"/v1/acl/policy/read": ``})

	written, err := client.SubmitACL(context.Background(), nomad.ACLPolicy, "read",
		"# Description: Read everything\nnamespace \"*\" {\n  policy = \"read\"\n}\n")
	r.NoError(err)

	r.Equal(map[string]any{
		"Name": "read", "Description": "Read everything", "Rules": "namespace \"*\" {\n  policy = \"read\"\n}\n",
	}, subset((*asked)[0].body, "Name", "Description", "Rules"))
	r.Equal(nomad.ACLWritten{ID: "read", Name: "read"}, written)
}

func TestSubmitACL_APolicyWithoutADescription(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{"/v1/acl/policy/read": ``})

	_, err := client.SubmitACL(context.Background(), nomad.ACLPolicy, "read", "namespace \"*\" {}\n")
	r.NoError(err)

	r.Empty((*asked)[0].body["Description"])
	r.Equal("namespace \"*\" {}\n", (*asked)[0].body["Rules"])
}

func TestSubmitACL_NotJSON(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{})

	_, err := client.SubmitACL(context.Background(), nomad.ACLRole, "r1", `{"Name": "ops",`)
	r.ErrorContains(err, "the role is not valid JSON")
	r.Empty(*asked)
}

func TestDeleteACL(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{}`)

	r.NoError(client.DeleteACL(context.Background(), nomad.ACLBindingRule, "b1"))

	r.Equal(http.MethodDelete, asked.Method)
	r.Equal("/v1/acl/binding-rule/b1", asked.URL.Path)
}

func TestACLTemplate(t *testing.T) {
	r := require.New(t)

	// Every kind starts from the fields it takes; all but a policy as JSON.
	for _, kind := range []string{nomad.ACLToken, nomad.ACLRole, nomad.ACLAuthMethod, nomad.ACLBindingRule} {
		r.True(json.Valid([]byte(nomad.ACLTemplate(kind))), kind)
	}

	r.True(strings.HasPrefix(nomad.ACLTemplate(nomad.ACLPolicy), "# Description: \n"))
}

// subset is what a body holds of these keys.
func subset(body map[string]any, keys ...string) map[string]any {
	out := map[string]any{}
	for _, key := range keys {
		out[key] = body[key]
	}

	return out
}
