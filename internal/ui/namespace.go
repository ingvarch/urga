package ui

import (
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// namespaceKeys is how many namespaces the number keys reach. The rest are
// reached through the command line.
const namespaceKeys = 9

// rememberNamespaces keeps the order the keys were handed out in. A cluster
// that answers in another order must not renumber them under the fingers.
func (m *Model) rememberNamespaces(list []nomad.Namespace) {
	for _, ns := range list {
		if len(m.namespaceOrder) >= namespaceKeys {
			return
		}

		if !slices.Contains(m.namespaceOrder, ns.Name) {
			m.namespaceOrder = append(m.namespaceOrder, ns.Name)
		}
	}
}

// namespaceKey switches the session to the namespace a number stands for.
// Zero is every namespace at once.
func (m Model) namespaceKey(digit int) (Model, tea.Cmd) {
	if digit == 0 {
		return m.switchNamespace(nomad.AllNamespaces)
	}

	if digit > len(m.namespaceOrder) {
		return m, nil
	}

	return m.switchNamespace(m.namespaceOrder[digit-1])
}

// switchNamespace points the session at a namespace and asks the cluster
// again for what is open.
func (m Model) switchNamespace(namespace string) (Model, tea.Cmd) {
	if m.namespace == namespace {
		return m, nil
	}

	m.namespace = namespace
	m.screen.namespace = namespace
	m.filter = ""

	next, cmd := m.enter()

	return next, tea.Batch(cmd, next.remember())
}

// namespaceColumnData is what the header shows for the number keys.
func (m Model) namespaceColumnData() []namespaceKey {
	if len(m.namespaceOrder) == 0 {
		return nil
	}

	keys := []namespaceKey{{
		Key:    "<0>",
		Name:   "all",
		Active: m.namespace == "" || m.namespace == nomad.AllNamespaces,
	}}

	for i, name := range m.namespaceOrder {
		keys = append(keys, namespaceKey{
			Key:    fmt.Sprintf("<%d>", i+1),
			Name:   name,
			Active: m.namespace == name,
		})
	}

	return keys
}
