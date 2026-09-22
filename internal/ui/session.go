package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// screenOfName is how a screen is written down between runs.
var screenOfName = map[string]screenKind{
	"jobs":        screenJobs,
	"deployments": screenDeployments,
	"namespaces":  screenNamespaces,
	"services":    screenServices,
	"evaluations": screenEvaluations,
	"nodes":       screenNodes,
	"variables":   screenVariables,
	"nodepools":   screenNodePools,
}

var nameOfScreen = func() map[screenKind]string {
	out := make(map[screenKind]string, len(screenOfName))
	for name, kind := range screenOfName {
		out[kind] = name
	}

	return out
}()

// restore picks the session up where it was left.
func (m Model) restore() Model {
	if m.opts.Config == nil {
		return m
	}

	cfg := m.opts.Config

	if cfg.Namespace != nil {
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

	cfg.UseNamespace(namespaceOrAll(m.namespace))
	cfg.Remember(m.namespaceOrder)

	if name, ok := nameOfScreen[m.screen.kind]; ok {
		cfg.Screen = name
	}

	return func() tea.Msg {
		if err := cfg.Save(); err != nil {
			return errMsg{err: err}
		}

		return nil
	}
}

// namespaceOrAll is how the namespace is written down: every namespace at
// once is a choice, and it is written as one.
func namespaceOrAll(namespace string) string {
	if namespace == "" {
		return nomad.AllNamespaces
	}

	return namespace
}
