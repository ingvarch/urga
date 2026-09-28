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
	{press: "n", label: "New", do: newACL, writes: true},
	{press: "e", label: "Edit", do: editACL, writes: true},
	{press: "ctrl+d", label: "Delete", do: deleteACL, writes: true},
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

// editACL opens the object under the cursor in the editor, as describe
// shows it, and saves it over that object.
func editACL(p aclPage, e env) (aclPage, outcome) {
	o, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	client, kind := e.client, p.kind

	return p, outcome{cmd: openEditor(aclFile(client, kind, o.ID, false, func(ctx context.Context) (string, error) {
		return client.DescribeACL(ctx, kind, o.ID)
	}))}
}

// newACL opens a new object in the editor, from the fields its kind takes. A
// policy is named first: its file does not hold its name.
func newACL(p aclPage, e env) (aclPage, outcome) {
	kind, client := p.kind, e.client
	template := func(context.Context) (string, error) { return nomad.ACLTemplate(kind), nil }

	if kind != nomad.ACLPolicy {
		return p, outcome{cmd: openEditor(aclFile(client, kind, "", true, template))}
	}

	return p, then(lineMsg{
		prefix: "new policy named: ",
		answer: func(m Model, typed string) (Model, tea.Cmd) {
			name := strings.TrimSpace(typed)
			if name == "" {
				return m.warn("A policy needs a name."), nil
			}

			return m, openEditor(aclFile(m.client, kind, name, true, template))
		},
	})
}

// aclFile is an ACL object as a file: what read gives, sent as the object id
// names, a new one when fresh. A policy is its file, the rest JSON. A file
// the cluster refuses opens again with the reason.
func aclFile(client aclClient, kind, id string, fresh bool, read func(ctx context.Context) (string, error)) load {
	extension := "json"
	if kind == nomad.ACLPolicy {
		extension = "hcl"
	}

	return func(ctx context.Context) (file, error) {
		content, err := read(ctx)

		return file{extension: extension, content: content, submit: reopening(extension, func(source string) tea.Cmd {
			return request(func(ctx context.Context) (nomad.ACLWritten, error) {
				return client.SubmitACL(ctx, kind, id, source)
			}, func(w nomad.ACLWritten) tea.Msg { return aclWritten(kind, fresh, w) })
		})}, err
	}
}

// aclWritten says what was written. A new token shows its secret, the one
// time it is drawn, and puts it on the clipboard.
func aclWritten(kind string, fresh bool, w nomad.ACLWritten) tea.Msg {
	noun, name := aclKinds[kind].noun, cmp.Or(w.Name, shortID(w.ID))

	if fresh && w.Secret != "" {
		return tea.BatchMsg{
			func() tea.Msg { return describeMsg{label: noun + " " + name + " created", lines: secretLines(w)} },
			func() tea.Msg { return copyMsg{field: "the secret of the token " + name, value: w.Secret} },
		}
	}

	verb := "saved"
	if fresh {
		verb = "created"
	}

	return doneMsg{said: sprintf("%s %s %s.", noun, name, verb)}
}

// secretLines are the IDs of a new token, and where its secret is kept.
func secretLines(w nomad.ACLWritten) []paintedLine {
	return []paintedLine{
		{text: "Accessor ID  " + w.ID},
		{text: "Secret ID    " + w.Secret, style: &styleValue},
		{},
		{text: "The secret is on the clipboard too. Keep it: describe leaves it out.", style: &styleMuted},
	}
}

// deleteACL deletes the object under the cursor, after the user confirms.
func deleteACL(p aclPage, e env) (aclPage, outcome) {
	o, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	client, kind := e.client, p.kind
	noun, name := aclKinds[kind].noun, cmp.Or(o.Name, shortID(o.ID))

	question := sprintf("Really delete the %s %s?", strings.ToLower(noun), name)
	if kind == nomad.ACLToken {
		question += " Whoever uses it loses access."
	}

	return p, then(askMsg{
		question: question,
		apply: act(sprintf("%s %s deleted.", noun, name), func(ctx context.Context) error {
			return client.DeleteACL(ctx, kind, o.ID)
		}),
	})
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
