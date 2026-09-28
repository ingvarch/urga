package ui

import (
	"cmp"
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// aclMsg is the ACL objects of a kind.
type aclMsg struct {
	kind    string
	objects []nomad.ACLObject
}

// aclKind is how the objects of a kind read: the title of their list, what
// one of them is called, their columns, and how an object fills its row.
type aclKind struct {
	title, noun string
	titles      []string
	add         func(row *tableRow, o nomad.ACLObject)
}

// aclKinds are the kinds of ACL object, with only what their list answers.
var aclKinds = map[string]aclKind{
	nomad.ACLToken: {
		title: "Tokens", noun: "Token",
		titles: []string{"Accessor", "Name", "Type", "Policies", "Roles", "Global", "Expires", "Age"},
		add: func(row *tableRow, o nomad.ACLObject) {
			row.add(shortID(o.ID), o.Name, o.Type, strings.Join(o.Policies, ", "), strings.Join(o.Roles, ", "),
				yesNo(o.Global), expiresIn(o.Expires))
			row.addAge(o.Created)
		},
	},
	nomad.ACLPolicy: {
		title: "Policies", noun: "Policy",
		titles: []string{"Name", "Description"},
		add:    func(row *tableRow, o nomad.ACLObject) { row.add(o.Name, o.Description) },
	},
	nomad.ACLRole: {
		title: "Roles", noun: "Role",
		titles: []string{"Name", "Description", "Policies"},
		add: func(row *tableRow, o nomad.ACLObject) {
			row.add(o.Name, o.Description, strings.Join(o.Policies, ", "))
		},
	},
	nomad.ACLAuthMethod: {
		title: "Auth Methods", noun: "Auth method",
		titles: []string{"Name", "Type", "Default"},
		add:    func(row *tableRow, o nomad.ACLObject) { row.add(o.Name, o.Type, yesNo(o.Default)) },
	},
	nomad.ACLBindingRule: {
		title: "Binding Rules", noun: "Binding rule",
		titles: []string{"ID", "Auth Method", "Description"},
		add:    func(row *tableRow, o nomad.ACLObject) { row.add(shortID(o.ID), o.AuthMethod, o.Description) },
	},
}

// aclPage is the ACL objects of one kind. They belong to the cluster, not to
// a namespace.
type aclPage struct {
	ofTheSession

	kind    string
	objects []nomad.ACLObject
}

func (p aclPage) title(_ env, count int) string {
	return sprintf("%s [%d]", aclKinds[p.kind].title, count)
}

func (p aclPage) titles() []string { return aclKinds[p.kind].titles }

// topics: none. The cluster streams changes of ACL objects to a management
// token only; the list is polled.
func (aclPage) topics() []string { return nil }

func (p aclPage) fetch(e env) tea.Cmd {
	client, kind := e.client, p.kind

	return fetchList(func(ctx context.Context) ([]nomad.ACLObject, error) {
		return client.ACLObjects(ctx, kind)
	}, func(items []nomad.ACLObject) tea.Msg { return aclMsg{kind: kind, objects: items} })
}

func (p aclPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	answer, ok := msg.(aclMsg)
	if !ok || answer.kind != p.kind {
		return p, outcome{}, false
	}

	p.objects = answer.objects

	return p, outcome{}, true
}

func (p aclPage) rows(env) []tableRow {
	kind, now := aclKinds[p.kind], time.Now()
	rows := make([]tableRow, 0, len(p.objects))

	for _, o := range p.objects {
		row := tableRow{color: tokenColor(o.Expires, now)}
		kind.add(&row, o)

		rows = append(rows, row)
	}

	return rows
}

// picked is the object under the cursor.
func (p aclPage) picked(e env) (nomad.ACLObject, bool) { return pickedFrom(e, p.objects) }

var aclKeys = []pageKey[aclPage]{
	{press: "d", label: "Describe", do: describeACL},
	{press: "c", label: "Copy Secret", do: copySecret, offered: func(p aclPage, _ env) bool { return p.kind == nomad.ACLToken }},
}

func (p aclPage) keys(e env) []keyHint { return hintsOf(p, e, aclKeys) }

func (p aclPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, aclKeys, k)
}

// describeACL shows the object under the cursor as the cluster has it. One
// without a name is named by its ID.
func describeACL(p aclPage, e env) (aclPage, outcome) {
	o, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	client, label := e.client, aclKinds[p.kind].noun+": "+cmp.Or(o.Name, shortID(o.ID))

	return p, outcome{cmd: describe(label, func(ctx context.Context) (string, error) {
		return client.DescribeACL(ctx, o.Kind, o.ID)
	})}
}

// copySecret puts the secret of the token under the cursor on the
// clipboard. It is never drawn: the screen may be shared.
func copySecret(p aclPage, e env) (aclPage, outcome) {
	o, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	client := e.client

	return p, outcome{cmd: request(func(ctx context.Context) (string, error) {
		return client.TokenSecret(ctx, o.ID)
	}, func(secret string) tea.Msg {
		return copyMsg{field: "the secret of the token " + o.Name, value: secret}
	})}
}

// expiresIn is when a token stops working: a dash for never.
func expiresIn(expires time.Time) string {
	if expires.IsZero() {
		return "-"
	}

	left := time.Until(expires)
	if left <= 0 {
		return "expired"
	}

	return "in " + age(left)
}
