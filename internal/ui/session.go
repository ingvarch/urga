package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/config"
	"github.com/ingvarch/urga/internal/nomad"
)

// viewOfName finds a view by the name it is saved under between runs.
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

// restore brings back the namespace, number keys and screen of the last run.
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

// session is the saved session of the cluster in use.
func (m Model) session() *config.Session {
	return m.opts.Config.Of(m.opts.Cluster)
}

// snapshot records the namespace, the number keys and the screen in the
// session and copies the config. It reports false without a config.
func (m Model) snapshot() (*config.Config, config.Snapshot, bool, error) {
	cfg := m.opts.Config
	if cfg == nil {
		return nil, config.Snapshot{}, false, nil
	}

	session := m.session()
	session.UseNamespace(NamespaceOrAll(m.namespace))
	session.Remember(m.namespaceOrder)

	if v := m.screen.view; v != nil && v.stored != "" {
		session.Screen = v.stored
	}

	snapshot, err := cfg.Snapshot()

	return cfg, snapshot, true, err
}

// remember saves the namespace and the screen of this session. It is a
// command, so the file is written outside the code that handles keys, from
// a copy made here.
func (m Model) remember() tea.Cmd {
	cfg, snapshot, ok, err := m.snapshot()
	if !ok {
		return nil
	}

	if err != nil {
		return func() tea.Msg { return errMsg{err: err} }
	}

	return func() tea.Msg {
		if err := cfg.Write(snapshot); err != nil {
			return errMsg{err: err}
		}

		return nil
	}
}

// SaveSession writes the session at once, for when urga quits and a write
// still running would be cut off.
func (m Model) SaveSession() error {
	cfg, snapshot, ok, err := m.snapshot()
	if !ok || err != nil {
		return err
	}

	return cfg.Write(snapshot)
}

// NamespaceOrAll is how a namespace is named when none was chosen: all
// namespaces at once is a choice too, and is saved as one.
func NamespaceOrAll(namespace string) string {
	if namespace == "" {
		return nomad.AllNamespaces
	}

	return namespace
}
