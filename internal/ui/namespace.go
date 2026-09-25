package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/config"
	"github.com/ingvarch/urga/internal/nomad"
)

// namespaceState is the namespace the session looks at, the keys that switch
// it, and the namespaces the cluster has.
type namespaceState struct {
	namespace string

	// namespaceOrder is which namespace each number key stands for.
	namespaceOrder []string

	// namespaces are kept across a switch of region: the command line and
	// the number keys need them, and the switch does not ask for them again.
	namespaces []nomad.Namespace
}

// rememberNamespaces keeps the order the keys were handed out in. A cluster
// that answers in another order must not renumber them under the fingers,
// and the rule for that lives with the file it is written to.
func (s *namespaceState) rememberNamespaces(list []nomad.Namespace) {
	seen := make([]string, 0, len(list))
	for _, ns := range list {
		seen = append(seen, ns.Name)
	}

	s.namespaceOrder = config.Ordered(s.namespaceOrder, seen)
}

// knowsNamespace says whether the cluster has a namespace by that name.
func (s namespaceState) knowsNamespace(namespace string) bool {
	for _, known := range s.namespaces {
		if known.Name == namespace {
			return true
		}
	}

	return false
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

	// A list opened by name follows the session there when it is entered; a
	// screen opened for a job or an allocation stays where that one lives.
	m.namespace = namespace
	m.list.filter = ""

	next, cmd := m.enter()

	return next, tea.Batch(cmd, next.remember())
}

// namespaceColumnData is what the header shows for the number keys.
func (s namespaceState) namespaceColumnData() []namespaceKey {
	if len(s.namespaceOrder) == 0 {
		return nil
	}

	keys := []namespaceKey{{
		Key:    "<0>",
		Name:   "all",
		Active: s.namespace == "" || s.namespace == nomad.AllNamespaces,
	}}

	for i, name := range s.namespaceOrder {
		keys = append(keys, namespaceKey{
			Key:    fmt.Sprintf("<%d>", i+1),
			Name:   name,
			Active: s.namespace == name,
		})
	}

	return keys
}
