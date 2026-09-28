package nomad

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"time"

	"github.com/hashicorp/nomad/api"
)

// The kinds of ACL object.
const (
	ACLToken       = "token"
	ACLPolicy      = "policy"
	ACLRole        = "role"
	ACLAuthMethod  = "auth method"
	ACLBindingRule = "binding rule"
)

// ACLObject is an ACL object as its list shows it. Each kind fills what it
// has.
type ACLObject struct {
	Kind string

	// ID is what the cluster names it by: the accessor of a token, the name
	// of a policy or an auth method, the ID of a role or a binding rule.
	ID string

	Name, Description string

	// Type is client or management for a token, the protocol of an auth
	// method.
	Type string

	// Policies and Roles are what a token or a role grants.
	Policies, Roles []string

	// AuthMethod is what a binding rule binds.
	AuthMethod string

	// Global is a token every region takes; Default the auth method a login
	// uses when it names none.
	Global, Default bool

	Created, Expires time.Time
}

// aclPath is where the cluster keeps a kind: its list, and one of it, by
// the ID that follows.
type aclPath struct{ list, one string }

var aclPaths = map[string]aclPath{
	ACLToken:       {"/v1/acl/tokens", "/v1/acl/token/"},
	ACLPolicy:      {"/v1/acl/policies", "/v1/acl/policy/"},
	ACLRole:        {"/v1/acl/roles", "/v1/acl/role/"},
	ACLAuthMethod:  {"/v1/acl/auth-methods", "/v1/acl/auth-method/"},
	ACLBindingRule: {"/v1/acl/binding-rules", "/v1/acl/binding-rule/"},
}

func pathOf(kind string) (aclPath, error) {
	path, ok := aclPaths[kind]
	if !ok {
		return aclPath{}, fmt.Errorf("no ACL object of kind %q", kind)
	}

	return path, nil
}

// aclEntry is an entry of any ACL list: each kind fills what it has.
type aclEntry struct {
	AccessorID, ID, Name, Description, Type, AuthMethod string

	Policies, Roles links

	Global, Default bool

	CreateTime     time.Time
	ExpirationTime *time.Time
}

// object is the entry as a kind of ACL object. A token is named by its
// accessor, a role and a binding rule by their ID, the rest by their name.
func (e aclEntry) object(kind string) ACLObject {
	object := ACLObject{
		Kind:        kind,
		ID:          cmp.Or(e.AccessorID, e.ID, e.Name),
		Name:        e.Name,
		Description: e.Description,
		Type:        e.Type,
		Policies:    e.Policies.names(),
		Roles:       e.Roles.names(),
		AuthMethod:  e.AuthMethod,
		Global:      e.Global,
		Default:     e.Default,
		Created:     e.CreateTime,
	}

	if e.ExpirationTime != nil {
		object.Expires = *e.ExpirationTime
	}

	return object
}

// link is an ACL object one names. The cluster writes it as its name, or as
// an object with its ID and a name it may leave empty.
type link struct{ ID, Name string }

// links are the objects an entry names: a token lists its policies one way
// and its roles the other.
type links []link

func (l *links) UnmarshalJSON(data []byte) error {
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}

	for _, item := range items {
		var name string
		if json.Unmarshal(item, &name) == nil {
			*l = append(*l, link{Name: name})

			continue
		}

		var named link
		if err := json.Unmarshal(item, &named); err != nil {
			return err
		}

		*l = append(*l, named)
	}

	return nil
}

// names are the names of the links, none for none.
func (l links) names() []string {
	var out []string
	for _, one := range l {
		out = append(out, one.Name)
	}

	return out
}

// nameRoles fills the names of the roles tokens name by their ID only, as
// the list of tokens does, from the list of roles. A role that is gone keeps
// its ID.
func (c *Client) nameRoles(ctx context.Context, entries []aclEntry) error {
	unnamed := slices.ContainsFunc(entries, func(e aclEntry) bool {
		return slices.ContainsFunc(e.Roles, func(role link) bool { return role.Name == "" })
	})
	if !unnamed {
		return nil
	}

	roles, err := c.ACLObjects(ctx, ACLRole)
	if err != nil {
		return err
	}

	names := make(map[string]string, len(roles))
	for _, role := range roles {
		names[role.ID] = role.Name
	}

	for _, e := range entries {
		for i, role := range e.Roles {
			e.Roles[i].Name = cmp.Or(role.Name, names[role.ID], role.ID)
		}
	}

	return nil
}

// ACLObjects lists the ACL objects of a kind.
func (c *Client) ACLObjects(ctx context.Context, kind string) ([]ACLObject, error) {
	path, err := pathOf(kind)
	if err != nil {
		return nil, err
	}

	var entries []aclEntry
	if _, err := c.api.Raw().Query(path.list, &entries, c.query(ctx, "")); err != nil {
		return nil, err
	}

	if err := c.nameRoles(ctx, entries); err != nil {
		return nil, err
	}

	objects := make([]ACLObject, 0, len(entries))
	for _, entry := range entries {
		objects = append(objects, entry.object(kind))
	}

	return objects, nil
}

// DescribeACL is what the cluster knows about an ACL object: a policy as its
// file, the rest as JSON, a token without its secret.
func (c *Client) DescribeACL(ctx context.Context, kind, id string) (string, error) {
	if kind == ACLPolicy {
		policy, _, err := c.api.ACLPolicies().Info(id, c.query(ctx, ""))
		if err != nil {
			return "", err
		}

		return policyFile(policy), nil
	}

	path, err := pathOf(kind)
	if err != nil {
		return "", err
	}

	var object map[string]any
	if _, err := c.api.Raw().Query(path.one+url.PathEscape(id), &object, c.query(ctx, "")); err != nil {
		return "", err
	}

	// Describing is reading, and the screen may be shared.
	delete(object, "SecretID")

	return asJSON(object)
}

// policyFile is a policy as it is written: its description, then its rules.
func policyFile(policy *api.ACLPolicy) string {
	return "# Description: " + policy.Description + "\n" + policy.Rules
}

// TokenSecret is the secret of a token, which is what signs requests with
// it.
func (c *Client) TokenSecret(ctx context.Context, accessorID string) (string, error) {
	token, _, err := c.api.ACLTokens().Info(accessorID, c.query(ctx, ""))
	if err != nil {
		return "", err
	}

	return token.SecretID, nil
}
