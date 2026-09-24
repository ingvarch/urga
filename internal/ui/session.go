package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// screenOfName is how a screen is written down between runs, and back.
var screenOfName map[string]screenKind

func storedIndex() map[string]screenKind {
	names := map[string]screenKind{}

	for kind, res := range resources {
		if res.stored != "" {
			names[res.stored] = kind
		}
	}

	return names
}

// restore picks the session up where it was left.
func (m Model) restore() Model {
	if m.opts.Config == nil {
		return m
	}

	cfg := m.opts.Config

	if cfg.Namespace != nil && !m.opts.NamespaceGiven {
		m.namespace = *cfg.Namespace
		m.screen.namespace = m.namespace
	}

	m.namespaceOrder = cfg.Namespaces

	if kind, ok := screenOfName[cfg.Screen]; ok {
		m.screen.kind = kind
		m.table = newTableModel(m.screen.titles())
	}

	return m
}

// remember writes down what this session is looking at. It is a command, so
// the file is written off the path that answers keys.
func (m Model) remember() tea.Cmd {
	cfg := m.opts.Config
	if cfg == nil {
		return nil
	}

	cfg.UseNamespace(NamespaceOrAll(m.namespace))
	cfg.Remember(m.namespaceOrder)

	if stored := m.screen.of().stored; stored != "" {
		cfg.Screen = stored
	}

	return func() tea.Msg {
		if err := cfg.Save(); err != nil {
			return errMsg{err: err}
		}

		return nil
	}
}

// NamespaceOrAll is how a namespace is named when none was chosen: every
// namespace at once is a choice of its own and is written as one.
func NamespaceOrAll(namespace string) string {
	if namespace == "" {
		return nomad.AllNamespaces
	}

	return namespace
}
