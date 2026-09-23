package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

func down() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyDown} }
func up() tea.KeyPressMsg   { return tea.KeyPressMsg{Code: tea.KeyUp} }
func tab() tea.KeyPressMsg  { return tea.KeyPressMsg{Code: tea.KeyTab} }

func backspace() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyBackspace} }

// promptLine is what the command line reads as, with what it offers.
func promptLine(m Model) string {
	return m.prompt.prefix + m.prompt.text + m.prompt.suggestion()
}

func TestPrompt_ALetterOffersTheFirstResourceThatFitsIt(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)
	m, _ = m.update(key(':'))
	m = typeIn(m, "s")

	// One letter is enough to be offered something, without walking to it.
	r.Equal(":servers", promptLine(m))
}

func TestPrompt_TheArrowsWalkWhatFits(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)
	m, _ = m.update(key(':'))
	m = typeIn(m, "s")

	m, _ = m.update(down())
	r.Equal(":services", promptLine(m))

	// Past the end it comes back to the first, and up walks the other way.
	m, _ = m.update(down())
	r.Equal(":servers", promptLine(m))

	m, _ = m.update(up())
	r.Equal(":services", promptLine(m))
}

func TestPrompt_AnEmptyLineOffersNothingUntilTheArrows(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)
	m, _ = m.update(key(':'))

	// An open line waits: it says nothing until it is asked.
	r.Equal(":", promptLine(m))

	m, _ = m.update(down())
	r.Equal(":"+commandNames[0], promptLine(m))

	m, _ = m.update(down())
	r.Equal(":"+commandNames[1], promptLine(m))

	m, _ = m.update(up())
	r.Equal(":"+commandNames[0], promptLine(m))
}

func TestPrompt_TabTakesWhatIsOffered(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)
	m, _ = m.update(key(':'))
	m = typeIn(m, "s")
	m, _ = m.update(down())
	m, _ = m.update(tab())

	// The word is in the line now, and the line is still open: a namespace
	// can follow it.
	r.Equal("services", m.prompt.text)
	r.Equal(overlayPrompt, m.overlay)
	r.Equal(":services", promptLine(m))
}

func TestPrompt_EnterOpensWhatIsOffered(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)
	m, _ = m.update(key(':'))
	m = typeIn(m, "s")
	m, _ = m.update(down())
	m, _ = m.update(enter())

	r.Equal(screenServices, m.screen.kind)
	r.Equal(overlayNone, m.overlay)
}

func TestPrompt_ANamespaceAfterTheWordIsNotAResource(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)
	m, _ = m.update(namespacesMsg(twoNamespaces()))
	m, _ = m.update(key(':'))
	m = typeIn(m, "jo")
	m, _ = m.update(tab())
	m = typeIn(m, " staging")

	// The resource is settled once there is a second word: nothing is
	// offered for it, and the arrows have nothing to walk.
	r.Equal(":jobs staging", promptLine(m))

	m, _ = m.update(down())
	r.Equal(":jobs staging", promptLine(m))

	m, _ = m.update(enter())
	r.Equal(screenJobs, m.screen.kind)
	r.Equal("staging", m.namespace)
}

func TestPrompt_TakesNoRoomOfItsOwn(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)
	rows := len(strings.Split(plain(m.render()), "\n"))

	m, _ = m.update(key(':'))
	m, _ = m.update(down())

	// Everything happens in the line: what it offers never pushes the rows
	// down the screen or takes one away.
	r.Equal(rows, len(strings.Split(plain(m.render()), "\n")))
	r.Contains(plain(m.render()), "Jobs (production)")
}

func TestPrompt_AShortFormOpensWhatItAlwaysDid(t *testing.T) {
	r := require.New(t)

	// `no` is Nomad's own word for a client. A name that merely starts the
	// same way must not take the line over.
	for word, kind := range map[string]screenKind{
		"no":   screenNodes,
		"node": screenNodes,
		"np":   screenNodePools,
		"ns":   screenNamespaces,
	} {
		m := walked(t, letters(word)...)
		opened, _ := m.commit()

		r.Equal(kind, opened.screen.kind, "%s reads as %q", word, promptLine(m))
	}
}

func TestFilter_OffersNothing(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)
	m, _ = m.update(key('/'))
	m = typeIn(m, "s")

	m, _ = m.update(down())

	// The filter takes any text there is; a resource has nothing to do
	// with it.
	r.Equal("/s", promptLine(m))
}

// walked runs a line of keys into an open command line.
func walked(t *testing.T, keys ...tea.KeyPressMsg) Model {
	t.Helper()

	m := loadedModel(t)
	m, _ = m.update(namespacesMsg(twoNamespaces()))
	m, _ = m.update(key(':'))

	for _, press := range keys {
		m, _ = m.update(press)
	}

	return m
}

// letters are one key press per character.
func letters(text string) []tea.KeyPressMsg {
	keys := make([]tea.KeyPressMsg, 0, len(text))
	for _, r := range text {
		keys = append(keys, key(r))
	}

	return keys
}

func TestPrompt_WhatTheLineReadsIsWhatEnterOpens(t *testing.T) {
	r := require.New(t)

	walks := map[string][]tea.KeyPressMsg{
		"a letter":               {key('s')},
		"a letter and a step":    {key('s'), down()},
		"a step back":            {key('s'), up()},
		"taken and stepped":      {key('s'), tab(), down()},
		"taken twice":            {key('j'), tab(), tab()},
		"walked from nothing":    {down(), down()},
		"walked and typed":       {down(), key('s')},
		"a space and a step":     {key(' '), down()},
		"typed past every name":  {key('z'), key('z'), down()},
		"backspaced to nothing":  {key('s'), backspace(), down()},
		"a word and a namespace": append([]tea.KeyPressMsg{key('j'), tab()}, append(letters(" staging"), down())...),
	}

	for name, keys := range walks {
		m := walked(t, keys...)

		line := m.prompt.text + m.prompt.suggestion()
		opened, _ := m.commit()

		// The line is the promise: whatever it reads as is what enter
		// opens, in every order the keys can be pressed.
		wanted, ok := parseCommand(line)

		switch {
		case !ok:
			r.NotNil(opened.err, "%s: the line reads %q and enter opened something anyway", name, line)
		default:
			r.Nil(opened.err, "%s: the line reads %q and enter refused it", name, line)
			r.Equal(wanted.kind, opened.screen.kind, "%s: the line reads %q", name, line)
		}
	}
}

func TestPrompt_TakingAWordOffersTheNextOneAfterIt(t *testing.T) {
	r := require.New(t)

	m := walked(t, key('s'), tab())

	// The word is settled, and the arrows walk on from it rather than from
	// what was typed before it.
	r.Equal("servers", m.prompt.text)

	m, _ = m.update(down())
	r.Equal(":servers", promptLine(m))
}

func TestPrompt_ASpaceIsNotAResource(t *testing.T) {
	r := require.New(t)

	m := walked(t, key(' '))

	// A line that holds nothing but a space names nothing, and must not
	// open the first resource there is.
	r.Empty(m.prompt.choice())

	opened, _ := m.commit()
	r.NotNil(opened.err)
}
