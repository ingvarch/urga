package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/config"
	"github.com/ingvarch/urga/internal/nomad"
)

// viewOfName is how a view is written down between runs, and back.
var viewOfName = storedIndex()

func storedIndex() map[string]*view {
	names := map[string]*view{}

	for _, v := range views {
		if v.stored != "" {
			names[v.stored] = v
		}
	}

	return names
}

// restore picks the session up where it was left.
func (m Model) restore() Model {
	if m.opts.Config == nil {
		return m
	}

	cfg := m.session()

	if cfg.Namespace != nil && !m.opts.NamespaceGiven {
		m.namespace = *cfg.Namespace
	}

	m.namespaceOrder = cfg.Namespaces

	if v, ok := viewOfName[cfg.Screen]; ok {
		m.screen = v.opened()
		m.list.table = newTableModel(m.screen.page.titles())
	}

	return m
}

// session is what the cluster in use was left looking at.
func (m Model) session() *config.Session {
	return m.opts.Config.Of(m.opts.Cluster)
}

// remember writes down what this session is looking at. It is a command, so
// the file is written off the path that answers keys.
func (m Model) remember() tea.Cmd {
	cfg := m.opts.Config
	if cfg == nil {
		return nil
	}

	session := m.session()
	session.UseNamespace(NamespaceOrAll(m.namespace))
	session.Remember(m.namespaceOrder)

	if v := m.screen.view; v != nil && v.stored != "" {
		session.Screen = v.stored
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
