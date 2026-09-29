package ui

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestNamespaces_NewOpensATemplateAndCreates(t *testing.T) {
	r := require.New(t)

	t.Setenv("TMPDIR", t.TempDir())

	source := `{"Name": "ml", "Description": "machine learning"}`
	client := &fakeClient{}
	editor := &fakeEditor{edits: []string{source}}
	m := onNamespaces(t, client, editor)

	m, cmd := m.update(key('n'))
	m = follow(m, cmd, 8)

	// The fields a namespace is made of, with no name yet.
	r.Len(editor.seen, 1)
	r.Contains(editor.seen[0], `"Name": ""`)
	r.Contains(editor.seen[0], `"Description": ""`)

	r.Equal([]string{"CreateNamespace"}, client.writes)
	r.Equal(source, client.createdNamespace)
	r.Contains(plain(m.render()), "Namespace ml created.")
}

func TestNamespaces_ANameThatExistsOpensAgain(t *testing.T) {
	r := require.New(t)

	t.Setenv("TMPDIR", t.TempDir())

	source := `{"Name": "staging"}`
	client := &fakeClient{refusals: []error{errors.New("namespace staging already exists")}}
	editor := &fakeEditor{edits: []string{source}}
	m := onNamespaces(t, client, editor)

	m, cmd := m.update(key('n'))
	follow(m, cmd, 12)

	r.Len(editor.seen, 2)
	r.Equal(reasonFor("namespace staging already exists")+source, editor.seen[1])
}

func TestNamespaces_DeleteAsks(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onNamespaces(t, client, &fakeEditor{})
	m, _ = m.update(key('j'))

	m, _ = m.update(ctrlKey('d'))
	r.Equal("Really delete the namespace staging?", m.confirm.question)

	m, cmd := answerYes(m)
	m = drain(m, cmd)

	r.Equal("staging", client.deletedNamespace)
	r.Contains(plain(m.render()), "Namespace staging deleted.")

	// Another namespace was deleted: the session stays where it is.
	r.Equal("production", m.namespace)
}

func TestNamespaces_DeletingTheOneOfTheSessionGoesToDefault(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onNamespaces(t, client, &fakeEditor{})

	m, _ = m.update(ctrlKey('d'))
	m, cmd := answerYes(m)
	m = playOut(m, cmd)

	r.Equal("production", client.deletedNamespace)
	r.Equal(nomad.DefaultNamespace, m.namespace)
	r.Contains(plain(m.render()), "Namespace production deleted. The session is in default now.")
}

func TestNamespaces_TheDefaultOneIsNotOfferedForDelete(t *testing.T) {
	r := require.New(t)

	m := onNamespaces(t, &fakeClient{}, &fakeEditor{})
	m, _ = m.update(namespacesMsg([]nomad.Namespace{{Name: nomad.DefaultNamespace}, {Name: "production"}}))

	// The cluster refuses it anyway.
	r.False(offers(m, "ctrl-d"))

	m, _ = m.update(key('j'))
	r.True(offers(m, "ctrl-d"))
}
