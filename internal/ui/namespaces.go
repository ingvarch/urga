package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// namespacesPage is the namespaces of the cluster. The session keeps them
// whatever is open, for the number keys and the command line; the page
// shows what the session has.
type namespacesPage struct{ ofTheSession }

var namespaceTitles = []string{"Name", "Quota", "Description"}

func (namespacesPage) title(_ env, count int) string             { return sprintf("Namespaces [%d]", count) }
func (namespacesPage) titles() []string                          { return namespaceTitles }
func (namespacesPage) topics() []string                          { return nil }
func (p namespacesPage) take(tea.Msg, env) (page, outcome, bool) { return p, outcome{}, false }

func (namespacesPage) fetch(e env) tea.Cmd {
	return fetchList(e.client.Namespaces, func(items []nomad.Namespace) tea.Msg { return namespacesMsg(items) })
}

func (namespacesPage) rows(e env) []tableRow {
	rows := make([]tableRow, 0, len(e.namespaces))

	for _, n := range e.namespaces {
		rows = append(rows, tableRow{cells: []string{n.Name, n.Quota, n.Description}})
	}

	return rows
}

var namespaceKeys = []pageKey[namespacesPage]{
	{press: "n", label: "New", writes: true, do: newNamespace},
	{press: "e", label: "Edit", writes: true, do: func(p namespacesPage, e env) (namespacesPage, outcome) {
		namespace, ok := pickedFrom(e, e.namespaces)
		if !ok {
			return p, outcome{}
		}

		return p, outcome{cmd: openEditor(namespaceFile(e.client, namespace.Name))}
	}},
	// The cluster keeps its default namespace.
	{press: "ctrl+d", label: "Delete", writes: true, do: deleteNamespace, offered: func(_ namespacesPage, e env) bool {
		namespace, ok := pickedFrom(e, e.namespaces)

		return ok && namespace.Name != nomad.DefaultNamespace
	}},
}

// namespaceTemplate is the file of a new namespace: the fields it is made
// of.
const namespaceTemplate = "{\n  \"Name\": \"\",\n  \"Description\": \"\",\n  \"Meta\": {}\n}\n"

// newNamespace opens a new namespace in the editor. One of a name that exists
// is refused, and the file opens again with the reason.
func newNamespace(p namespacesPage, e env) (namespacesPage, outcome) {
	client := e.client

	return p, outcome{cmd: openEditor(func(context.Context) (file, error) {
		return file{extension: "json", content: namespaceTemplate, submit: reopening("json", func(source string) tea.Cmd {
			return request(func(ctx context.Context) (string, error) {
				return client.CreateNamespace(ctx, source)
			}, func(name string) tea.Msg { return doneMsg{said: sprintf("Namespace %s created.", name)} })
		})}, nil
	})}
}

// deleteNamespace deletes the namespace under the cursor, after the user
// confirms. A session in it goes to default: every list of the one deleted
// would be empty.
func deleteNamespace(p namespacesPage, e env) (namespacesPage, outcome) {
	namespace, ok := pickedFrom(e, e.namespaces)
	if !ok {
		return p, outcome{}
	}

	client, name, session := e.client, namespace.Name, e.namespace

	return p, then(askMsg{
		question: sprintf("Really delete the namespace %s?", name),
		apply: request(func(ctx context.Context) (string, error) {
			return name, client.DeleteNamespace(ctx, name)
		}, func(string) tea.Msg {
			if name != session {
				return doneMsg{said: sprintf("Namespace %s deleted.", name)}
			}

			return tea.BatchMsg{
				func() tea.Msg { return switchNamespaceMsg(nomad.DefaultNamespace) },
				func() tea.Msg {
					return doneMsg{said: sprintf("Namespace %s deleted. The session is in %s now.", name, nomad.DefaultNamespace)}
				},
			}
		}),
	})
}

func (p namespacesPage) keys(e env) []keyHint { return hintsOf(p, e, namespaceKeys) }

func (p namespacesPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, namespaceKeys, k)
}
