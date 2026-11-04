package ui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestNamespaces_NewOpensATemplateAndCreates(t *testing.T) {
	r := require.New(t)

	tempDir(t)

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

	tempDir(t)

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

	// The cursor stays on production, which is now the second row.
	r.True(offers(m, "ctrl-d"))

	// The cluster refuses it anyway.
	m, _ = m.update(key('k'))
	r.False(offers(m, "ctrl-d"))
}

func TestNamespaces_ADeleteOfTheOneOfTheSessionStillMovesItAfterTheScreenIsLeft(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onNamespaces(t, client, &fakeEditor{})

	m, _ = m.update(ctrlKey('d'))
	m, cmd := answerYes(m)

	m, _ = m.update(escape())
	m = playOut(m, cmd)

	r.Equal("production", client.deletedNamespace)
	r.Equal(nomad.DefaultNamespace, m.namespace)
}

func TestNamespaces_DeletingTheOneOfTheSessionGoesToDefaultAfterASwitchOfClusters(t *testing.T) {
	r := require.New(t)

	m, clusters := onDev(t)
	clusters.dev.namespaces = twoNamespaces()

	// The connection is not the first one any more.
	m = typeCommand(m, "ctx prod")
	m = typeCommand(m, "ctx dev")

	m = typeCommand(m, "ns production")
	m, _ = m.update(ctrlKey('d'))
	r.Equal("Really delete the namespace production?", m.confirm.question)

	m, cmd := answerYes(m)
	m = playOut(m, cmd)

	r.Equal("production", clusters.dev.deletedNamespace)
	r.Equal(nomad.DefaultNamespace, m.namespace)
}

// answersOf runs a command and, one level down at a time, every command of
// the batches it returns, and gives the messages.
func answersOf(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}

	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, answersOf(c)...)
		}

		return out
	}

	if msg == nil {
		return nil
	}

	return []tea.Msg{msg}
}

// deleteAnswer deletes the namespace under the cursor and gives the answers
// of the cluster: the done message, and the one that moves the session, which
// is nil when the answer has none.
func deleteAnswer(t *testing.T, m Model) (next Model, move, done tea.Msg) {
	t.Helper()

	next, _ = m.update(ctrlKey('d'))
	next, cmd := answerYes(next)

	for _, msg := range answersOf(cmd) {
		if _, ok := answered(t, msg).(doneMsg); ok {
			require.Nil(t, done, "two done messages")

			done = msg

			continue
		}

		require.Nil(t, move, "two moves")

		move = msg
	}

	require.NotNil(t, done)

	return next, move, done
}

// takeInOrder gives the answers to the model, the move first or the done
// message first, and runs what each starts.
func takeInOrder(m Model, move, done tea.Msg, moveFirst bool) Model {
	order := []tea.Msg{move, done}
	if !moveFirst {
		order = []tea.Msg{done, move}
	}

	for _, msg := range order {
		if msg == nil {
			continue
		}

		var cmd tea.Cmd

		m, cmd = m.update(msg)
		m = playOut(m, cmd)
	}

	return m
}

func TestNamespaces_ADeleteReadsTheSameInEitherOrder(t *testing.T) {
	for name, tc := range map[string]struct {
		keys      string
		namespace string
		want      string
		notWant   string
	}{
		"of the session": {"", nomad.DefaultNamespace, "Namespace production deleted. The session is in default now.", ""},
		"of another one": {"j", "production", "Namespace staging deleted.", "The session is in"},
	} {
		for _, moveFirst := range []bool{true, false} {
			t.Run(sprintf("%s, move first %t", name, moveFirst), func(t *testing.T) {
				r := require.New(t)

				m := onNamespaces(t, &fakeClient{}, &fakeEditor{})
				for _, k := range tc.keys {
					m, _ = m.update(key(k))
				}

				m, move, done := deleteAnswer(t, m)
				m = takeInOrder(m, move, done, moveFirst)

				r.Equal(tc.namespace, m.namespace)
				r.Contains(plain(m.render()), tc.want)

				if tc.notWant != "" {
					r.NotContains(plain(m.render()), tc.notWant)
				}
			})
		}
	}
}

func TestNamespaces_ALateDeleteMovesOnlyASessionStillInIt(t *testing.T) {
	for name, tc := range map[string]struct {
		keys      string
		namespace string
		said      string
		notWant   string
	}{
		"after the session left it":      {"", "staging", "Namespace production deleted.", "The session is in"},
		"after the session went into it": {"j", nomad.DefaultNamespace, "Namespace staging deleted. The session is in default now.", ""},
	} {
		for _, moveFirst := range []bool{true, false} {
			t.Run(sprintf("%s, move first %t", name, moveFirst), func(t *testing.T) {
				r := require.New(t)

				m := onNamespaces(t, &fakeClient{}, &fakeEditor{})
				for _, k := range tc.keys {
					m, _ = m.update(key(k))
				}

				m, move, done := deleteAnswer(t, m)
				m = typeCommand(m, "jobs staging")
				m = takeInOrder(m, move, done, moveFirst)

				r.Equal(tc.namespace, m.namespace)
				r.Contains(plain(m.render()), tc.said)

				if tc.notWant != "" {
					r.NotContains(plain(m.render()), tc.notWant)
				}
			})
		}
	}
}

func TestNamespaces_ALateDeleteAfterASwitchOfClustersLeavesTheSessionWhereItIs(t *testing.T) {
	for _, moveFirst := range []bool{true, false} {
		t.Run(sprintf("move first %t", moveFirst), func(t *testing.T) {
			r := require.New(t)

			m, clusters := onDev(t)
			clusters.dev.namespaces = twoNamespaces()
			clusters.prod.namespaces = twoNamespaces()

			m = typeCommand(m, "ctx prod")
			m = typeCommand(m, "ctx dev")
			m = typeCommand(m, "ns production")

			m, move, done := deleteAnswer(t, m)
			r.Equal("production", clusters.dev.deletedNamespace)

			m = typeCommand(m, "ctx prod")
			m = typeCommand(m, "ns production")
			m = takeInOrder(m, move, done, moveFirst)

			r.Equal("production", m.namespace)
			r.Contains(plain(m.render()), "Namespace production deleted.")
			r.NotContains(plain(m.render()), "The session is in")
		})
	}
}
