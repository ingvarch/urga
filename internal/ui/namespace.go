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

	// namespaceOrder is which namespace each number key selects.
	namespaceOrder []string

	// namespaces are kept across a switch of region: the command line and
	// the number keys need them, and the switch does not ask for them again.
	namespaces []nomad.Namespace
}

// rememberNamespaces keeps the order the number keys were given out in. A
// cluster that answers in another order must not renumber keys the user
// knows. The rule lives in config, which writes the order to its file.
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

// namespaceKey switches the session to the namespace a number key selects.
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

// moveTo points the session at a namespace. The lists of the namespace left
// start empty, on the screen and under it, so no rows of the old namespace
// stay under the new title when the cluster does not answer. They start at
// the first row, with no filter or sort, as a list entered after a switch.
func (m Model) moveTo(namespace string) Model {
	if namespace == m.namespace {
		return m
	}

	m.namespace = namespace

	// The place of the open list is saved from the list when a command opens
	// another screen on top of it.
	if m.screen.listsTheNamespace() {
		m.list.filter, m.list.sort = "", newSortState()
		m.list.table.cursor, m.list.table.top = 0, 0
	}

	m.screen = m.screen.unread()

	history := make([]screen, len(m.history))
	for i, s := range m.history {
		history[i] = s.unread()
	}

	m.history = history

	return m
}

// switchNamespace points the session at a namespace and asks the cluster
// again for what is open.
func (m Model) switchNamespace(namespace string) (Model, tea.Cmd) {
	if m.namespace == namespace {
		return m, nil
	}

	// A list opened by name is entered again in the new namespace, with no
	// filter; a screen opened for a job or an allocation stays in its own.
	m = m.moveTo(namespace)
	if m.screen.followsSession() {
		m.list.filter = ""
	}

	next, cmd := m.followSession()

	return next, tea.Batch(cmd, next.remember())
}

// followSession enters the open screen again after the session moved to
// another namespace or datacenter, when it is a list that follows the
// session. A screen opened for one job, allocation or stream stays as it is:
// asking for it again would only restart its stream and lose its window.
func (m Model) followSession() (Model, tea.Cmd) {
	if !m.screen.followsSession() {
		m.layout()

		return m, nil
	}

	return m.enter()
}

// namespaceColumnData is what the header shows for the number keys.
func (s namespaceState) namespaceColumnData() []namespaceKey {
	if len(s.namespaceOrder) == 0 {
		return nil
	}

	keys := []namespaceKey{{
		Key:    "<0>",
		Name:   "all",
		Active: nomad.EveryNamespace(s.namespace),
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

// keepNamespaces keeps the namespaces whatever is on the screen: the command
// line and the number keys need the list to switch between them.
func (m Model) keepNamespaces(msg namespacesMsg) (Model, tea.Cmd) {
	m.namespaces = msg
	m.rememberNamespaces(msg)

	next, cmd := m.applyList(namespacesView)

	return next, tea.Batch(cmd, next.remember())
}

// namespaceDeleted says the cluster deleted a namespace, and takes a session
// that is still in it to default.
func (m Model) namespaceDeleted(msg namespaceDeletedMsg) (Model, tea.Cmd) {
	said := sprintf("Namespace %s deleted.", msg.name)
	if msg.connection != m.connection || m.namespace != msg.name {
		return m.say(said), nil
	}

	next, cmd := m.switchNamespace(nomad.DefaultNamespace)

	return next.say(said + sprintf(" The session is in %s now.", nomad.DefaultNamespace)), cmd
}
