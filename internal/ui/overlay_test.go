package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

func typeIn(m Model, text string) Model {
	for _, r := range text {
		m, _ = m.update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	return m
}

func key(name rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: name, Text: string(name)}
}

func loadedModel(t *testing.T) Model {
	t.Helper()

	m := newTestModel(&fakeClient{jobs: twoJobs(), allocs: twoAllocs()})
	m, _ = m.update(jobsMsg(twoJobs()))

	return m
}

func TestPrompt_OpensAndCloses(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)

	m, _ = m.update(key(':'))
	r.Equal(overlayPrompt, m.overlay)

	m = typeIn(m, "jo")

	// What was typed is on the screen, with the rest of the word dim behind
	// it, the way a shell suggests.
	out := plain(m.render())
	r.Contains(out, ":jo")
	r.Contains(out, "jobs")

	m, _ = m.update(escape())

	r.Equal(overlayNone, m.overlay)
	r.NotContains(plain(m.render()), ":jo")
}

func TestPrompt_TabTakesTheSuggestion(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)
	m, _ = m.update(key(':'))
	m = typeIn(m, "jo")

	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyTab})

	r.Equal("jobs", m.prompt.text)
}

func TestPrompt_SwitchesResource(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)
	m, _ = m.update(key(':'))
	m = typeIn(m, "dp")

	m, cmd := m.update(enter())

	// The prompt closes and the resource is on the screen.
	r.Equal(overlayNone, m.overlay)
	r.IsType(deploymentsPage{}, m.screen.page)
	r.NotNil(cmd)
}

func TestPrompt_SwitchesNamespaceToo(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), namespaces: twoNamespaces()}
	m := newTestModel(client)
	m, _ = m.update(namespacesMsg(twoNamespaces()))

	m, _ = m.update(key(':'))
	m = typeIn(m, "jobs staging")
	m, cmd := m.update(enter())

	r.Equal("staging", m.namespace)
	r.IsType(jobsPage{}, m.screen.page)

	m = drain(m, cmd)
	r.Equal("staging", client.askedNamespace)

	// The header says which namespace the session looks at.
	r.Contains(headerOf(m), "staging")
}

func TestPrompt_UnknownResourceSays(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)
	m, _ = m.update(key(':'))
	m = typeIn(m, "zzz")
	m, _ = m.update(enter())

	r.Equal(overlayNone, m.overlay)
	r.Contains(plain(m.render()), "no such resource: zzz")

	// The list that was there stays there.
	r.IsType(jobsPage{}, m.screen.page)
}

func TestPrompt_UnknownNamespaceSays(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{jobs: twoJobs(), namespaces: twoNamespaces()})
	m, _ = m.update(namespacesMsg(twoNamespaces()))

	m, _ = m.update(key(':'))
	m = typeIn(m, "jobs nowhere")
	m, _ = m.update(enter())

	r.Contains(plain(m.render()), "no such namespace: nowhere")
	r.Equal("production", m.namespace)
}

func TestPrompt_Quits(t *testing.T) {
	r := require.New(t)

	for _, word := range []string{"q", "q!", "quit"} {
		m := loadedModel(t)
		m, _ = m.update(key(':'))
		m = typeIn(m, word)

		_, cmd := m.update(enter())
		r.NotNil(cmd, word)
		r.IsType(tea.QuitMsg{}, cmd(), word)
	}
}

func TestPrompt_TakesTheKeysFromTheScreen(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)
	m, _ = m.update(key(':'))

	// Keys of the screen are text while the prompt is open: q does not quit
	// and j does not move the cursor.
	m, cmd := m.update(key('q'))
	r.Nil(cmd)

	m = typeIn(m, "j")
	r.Equal("qj", m.prompt.text)
	r.Zero(m.list.table.cursor)
}

func TestPrompt_Backspace(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)
	m, _ = m.update(key(':'))
	m = typeIn(m, "job")

	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	r.Equal("jo", m.prompt.text)

	m, _ = m.update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	r.Empty(m.prompt.text)
}

func TestPrompt_BackspaceThatSendsCtrlH(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)
	m, _ = m.update(key(':'))
	m = typeIn(m, "job")

	// A terminal set to send ^H for its backspace key hands the key over as
	// ctrl+h. On a line being typed that can only mean one thing.
	m, _ = m.update(tea.KeyPressMsg{Code: 'h', Mod: tea.ModCtrl})
	r.Equal("jo", m.prompt.text)
}

func TestPrompt_BackspaceErasesACharacter(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)
	m, _ = m.update(key('/'))
	m = typeIn(m, "жук")

	// A letter can take more than one byte. Cutting one byte leaves half a
	// letter on the line, and a filter nothing matches.
	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	r.Equal("жу", m.prompt.text)
}

func TestFilter_TakesAPaste(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)
	m, _ = m.update(key('/'))

	// A name copied out of a log or a chat is pasted, not typed. Copying a
	// whole line brings its line break along.
	m, _ = m.update(tea.PasteMsg{Content: "cron\n"})

	r.Equal("cron", m.prompt.text)

	out := plain(m.render())
	r.Contains(out, "cron")
	r.NotContains(out, "web")
}

func TestPrompt_APasteStaysOnOneLine(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)
	m, _ = m.update(key(':'))

	// The line is one line. Breaks and tabs inside a paste are spaces, and
	// what would move the terminal around is left out.
	m, _ = m.update(tea.PasteMsg{Content: "jobs\tproduction\r\nnow\x1b[2J"})

	r.Equal("jobs production now[2J", m.prompt.text)
}

func TestPaste_WithNoLineOpenChangesNothing(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)

	// Nothing on a list takes text. A paste is not a string of keys, and
	// must not press any of them.
	next, cmd := m.update(tea.PasteMsg{Content: "q"})

	r.Nil(cmd)
	r.Equal(overlayNone, next.overlay)
	r.IsType(m.screen.page, next.screen.page)
	r.Empty(next.list.filter)
}

func TestFilter_NarrowsTheList(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)

	m, _ = m.update(key('/'))
	r.Equal(overlayFilter, m.overlay)

	m = typeIn(m, "cron")

	out := plain(m.render())
	r.Contains(out, "cron")
	r.NotContains(out, "web")

	// The count in the title follows what is on the screen.
	r.Contains(out, "Jobs (production) [1]")

	// Escape drops the filter and the rows come back.
	m, _ = m.update(escape())
	r.Contains(plain(m.render()), "web")
	r.Equal(overlayNone, m.overlay)
}

func TestFilter_CursorFollowsTheFilteredRow(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)

	m, _ = m.update(key('/'))
	m = typeIn(m, "cron")
	m, _ = m.update(enter())

	// Enter closes the filter and keeps it on, the cursor is on the row that
	// is left.
	r.Equal(overlayNone, m.overlay)

	m, _ = m.update(enter())
	r.IsType(allocationsPage{}, m.screen.page)

	// The allocations of the job that was filtered to, not of the first row
	// of the unfiltered list.
	r.Contains(plain(m.render()), "Allocations (Job: cron)")
}

func TestHelp_OpensAndCloses(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)

	m, _ = m.update(key('?'))
	r.Equal(overlayHelp, m.overlay)

	out := plain(m.render())
	r.Contains(out, "RESOURCE")
	r.Contains(out, "GENERAL")
	r.Contains(out, "NAVIGATION")

	// The keys of the open screen are in there, next to the ones that work
	// everywhere.
	r.Contains(out, "Allocations")
	r.Contains(out, "Command")
	r.Contains(out, "Filter")

	m, _ = m.update(escape())
	r.Equal(overlayNone, m.overlay)
	r.NotContains(plain(m.render()), "NAVIGATION")
}

func TestHelp_TakesTheKeys(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)
	m, _ = m.update(key('?'))

	// The list does not move under an open help window.
	m, _ = m.update(key('j'))
	r.Zero(m.list.table.cursor)

	// Any of the three ways out closes it.
	for _, out := range []tea.KeyPressMsg{escape(), enter(), key('?')} {
		m, _ := m.update(out)
		r.Equal(overlayNone, m.overlay, out.String())
	}
}

func TestHelp_HasNoRoomForTheList(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)
	m, _ = m.update(key('?'))

	// Help covers the body, the rows are not behind it.
	r.NotContains(plain(m.render()), strings.Repeat("│", 1)+" web")
}

func TestPrompt_SwitchesBothAtOnce(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), namespaces: twoNamespaces()}
	m := newTestModel(client)
	m, _ = m.update(namespacesMsg(twoNamespaces()))

	m, _ = m.update(key(':'))
	m = typeIn(m, "dp staging")
	m, cmd := m.update(enter())

	// A command that names a resource and a namespace does both, not one of
	// the two.
	r.IsType(deploymentsPage{}, m.screen.page)
	r.Equal("staging", m.namespace)

	m = drain(m, cmd)
	r.Equal("staging", client.askedNamespace)
}
