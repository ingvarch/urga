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
	return m.prompt.prefix + m.prompt.line()
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
	r.Equal(":allocations", promptLine(m))

	m, _ = m.update(down())
	r.Equal(":clients", promptLine(m))

	m, _ = m.update(up())
	r.Equal(":allocations", promptLine(m))
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

	r.IsType(servicesPage{}, m.screen.page)
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
	r.IsType(jobsPage{}, m.screen.page)
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
	for word, v := range map[string]*view{
		"no":   nodesView,
		"node": nodesView,
		"np":   nodePoolsView,
		"ns":   namespacesView,
	} {
		m := walked(t, letters(word)...)
		opened, _ := m.commit()

		r.Same(v, opened.screen.view, "%s reads as %q", word, promptLine(m))
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

		line := m.prompt.line()
		opened, _ := m.commit()

		// The line is the promise: whatever it reads as is what enter
		// opens, in every order the keys can be pressed.
		wanted, ok := parseCommand(line)

		switch {
		case !ok:
			r.Equal(flashErr, opened.flash.level, "%s: the line reads %q and enter opened something anyway", name, line)
		default:
			r.NotEqual(flashErr, opened.flash.level, "%s: the line reads %q and enter refused it", name, line)
			r.Same(wanted.view, opened.screen.view, "%s: the line reads %q", name, line)
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
	r.Equal(flashErr, opened.flash.level)
}

func TestPrompt_TheOfferSurvivesTheNamespaceAfterIt(t *testing.T) {
	r := require.New(t)

	m := walked(t, letters("s")...)
	r.Equal(":servers", promptLine(m))

	m = typeIn(m, " staging")

	// The word that was offered stands in the line with the namespace
	// behind it, and enter opens exactly that.
	r.Equal(":servers staging", promptLine(m))

	opened, _ := m.commit()
	r.IsType(serversPage{}, opened.screen.page)
	r.Equal("staging", opened.namespace)
}

func TestPrompt_AWalkedWordKeepsItsPlaceAfterASpace(t *testing.T) {
	r := require.New(t)

	m := walked(t, key('s'), down())
	r.Equal(":services", promptLine(m))

	m = typeIn(m, " staging")

	// Typing where to look does not walk the resource back to the first
	// one that fits.
	r.Equal(":services staging", promptLine(m))
}

func TestPrompt_UppercaseReadsAsOneWord(t *testing.T) {
	r := require.New(t)

	m := walked(t, letters("SE")...)

	// What is offered is a word of the cluster, not of the keyboard: the
	// line reads as that word.
	r.Equal(":servers", promptLine(m))

	m, _ = m.update(tab())
	r.Equal("servers", m.prompt.text)
}

func TestPrompt_LeavingIsNotSomethingToWalkInto(t *testing.T) {
	r := require.New(t)

	m := walked(t)

	// Walking the resources must never land on the way out of urga.
	for range len(commandNames) + 2 {
		m, _ = m.update(down())
		r.NotEqual("quit", m.prompt.choice())
	}

	// Typed out, it still leaves.
	typed := walked(t, letters("quit")...)
	_, cmd := typed.commit()
	r.NotNil(cmd)
}
