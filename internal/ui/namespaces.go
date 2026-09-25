package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// namespacesPage is the namespaces of the cluster. The session keeps them
// whatever is open, for the number keys and the command line; the page
// shows what the session has.
type namespacesPage struct{}

var namespaceTitles = []string{"Name", "Quota", "Description"}

func (namespacesPage) title(_ env, count int) string { return sprintf("Namespaces [%d]", count) }
func (namespacesPage) titles() []string              { return namespaceTitles }
func (namespacesPage) topics() []string              { return nil }
func (p namespacesPage) take(tea.Msg) (page, bool)   { return p, false }

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
	{press: "e", label: "Edit", writes: true, do: func(p namespacesPage, e env) (namespacesPage, outcome) {
		namespace, ok := pickedFrom(e, e.namespaces)
		if !ok {
			return p, outcome{}
		}

		return p, outcome{cmd: openEditor(namespaceFile(e.client, namespace.Name))}
	}},
}

func (p namespacesPage) keys(e env) []keyHint { return hintsOf(p, e, namespaceKeys) }

func (p namespacesPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, namespaceKeys, k)
}
