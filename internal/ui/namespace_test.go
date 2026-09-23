package ui

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func threeNamespaces() []nomad.Namespace {
	return []nomad.Namespace{{Name: "default"}, {Name: "production"}, {Name: "staging"}}
}

func TestNamespaceKeys_SwitchTheSession(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), namespaces: threeNamespaces()}
	m := newTestModel(client)
	m, _ = m.update(namespacesMsg(threeNamespaces()))

	// The keys are in the header, numbered from one, with all of them on
	// zero.
	head := headerOf(m)
	r.Contains(head, "<0> all")
	r.Contains(head, "<1> default")
	r.Contains(head, "<3> staging")

	m, cmd := m.update(key('3'))
	r.Equal("staging", m.namespace)

	m = drain(m, cmd)
	r.Equal("staging", client.askedNamespace)

	// Zero brings every namespace back, and the key says so.
	m, _ = m.update(key('0'))
	r.Equal(nomad.AllNamespaces, m.namespace)
	r.Contains(renderHeader(m.headerData(), m.width-2*headerPadX), styleKey.Render(pad("all", len("production"))))
}

func TestNamespaceKeys_KeepTheirOrder(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{namespaces: threeNamespaces()})
	m, _ = m.update(namespacesMsg(threeNamespaces()))

	m, _ = m.update(key('3'))
	r.Equal("staging", m.namespace)

	// The cluster answers with the list in another order. The keys do not
	// move: a number means the same namespace as a moment ago.
	m, _ = m.update(namespacesMsg([]nomad.Namespace{{Name: "staging"}, {Name: "default"}, {Name: "production"}}))

	head := headerOf(m)
	r.Contains(head, "<1> default")
	r.Contains(head, "<3> staging")

	m, _ = m.update(key('1'))
	r.Equal("default", m.namespace)
}

func TestNamespaceKeys_MarkTheOneInUse(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{namespaces: threeNamespaces()})
	m, _ = m.update(namespacesMsg(threeNamespaces()))
	m, _ = m.update(key('2'))

	r.Contains(headerOf(m), "<2> production")

	// The one in use is the only one lit up, the rest are quiet.
	head := renderHeader(m.headerData(), m.width-2*headerPadX)
	r.Contains(head, styleKey.Render("production"))
	r.Contains(head, styleMuted.Render(pad("default", len("production"))))
	r.NotContains(head, styleKey.Render(pad("default", len("production"))))
}

func TestNamespaceKeys_OnlyNine(t *testing.T) {
	r := require.New(t)

	many := make([]nomad.Namespace, 0, 12)
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"} {
		many = append(many, nomad.Namespace{Name: name})
	}

	m := newTestModel(&fakeClient{namespaces: many})
	m, _ = m.update(namespacesMsg(many))

	// There are nine keys to give away, the rest of the namespaces are
	// reached through the command line.
	head := headerOf(m)
	r.Contains(head, "<9> i")
	r.NotContains(head, "<10>")
	r.NotContains(head, " j")
}

func TestNamespaceKeys_NothingToSwitchTo(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{})

	// A key that points at no namespace leaves the session where it is.
	m, cmd := m.update(key('5'))

	r.Equal("production", m.namespace)
	r.Nil(cmd)
}

func TestNamespaceKeys_DropTheAnswerAskedBefore(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{jobs: twoJobs(), namespaces: threeNamespaces()})
	m, _ = m.update(namespacesMsg(threeNamespaces()))

	// The jobs of production are on their way when the session moves on.
	late := m.fetch()

	m, _ = m.update(key('3'))
	m, _ = m.update(late())

	// They must not stand under the name of staging.
	r.Empty(m.jobs)
	r.NotContains(plain(m.render()), "cron")
}
