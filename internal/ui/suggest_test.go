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

	// One letter is enough to get a suggestion, without pressing an arrow.
	r.Equal(":servers", promptLine(m))
}

func TestPrompt_TheArrowsWalkWhatFits(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)
	m, _ = m.update(key(':'))
	m = typeIn(m, "s")

	m, _ = m.update(down())
	r.Equal(":services", promptLine(m))

	// Past the end it wraps to the first, and up moves the other way.
	m, _ = m.update(down())
	r.Equal(":servers", promptLine(m))

	m, _ = m.update(up())
	r.Equal(":services", promptLine(m))
}

func TestPrompt_AnEmptyLineOffersNothingUntilTheArrows(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)
	m, _ = m.update(key(':'))

	// An empty line suggests nothing until an arrow is pressed.
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

	// The resource is fixed once there is a second word: nothing is
	// suggested for it, and the arrows do nothing.
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

	// `no` is Nomad's own word for a client. A longer name that starts the
	// same way must not be picked instead.
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

	// The filter accepts any text and suggests no resource.
	r.Equal("/s", promptLine(m))
}

// walked presses keys, one after another, in an open command line.
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

		// Whatever the line reads as is what enter opens, in every order
		// the keys can be pressed.
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

	// The word is taken, and the arrows move on from it rather than from
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

	// The suggested word stays in the line with the namespace after it,
	// and enter opens exactly that.
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

	// Typing a namespace does not move the resource back to the first
	// one that fits.
	r.Equal(":services staging", promptLine(m))
}

func TestPrompt_UppercaseReadsAsOneWord(t *testing.T) {
	r := require.New(t)

	m := walked(t, letters("SE")...)

	// The suggestion keeps the case of the resource name, not of what was
	// typed: the line reads as that word.
	r.Equal(":servers", promptLine(m))

	m, _ = m.update(tab())
	r.Equal("servers", m.prompt.text)
}

func TestPrompt_LeavingIsNotSomethingToWalkInto(t *testing.T) {
	r := require.New(t)

	m := walked(t)

	// Stepping through the resources must never land on quit.
	for range len(commandNames) + 2 {
		m, _ = m.update(down())
		r.NotEqual("quit", m.prompt.choice())
	}

	// Typed in full, quit still exits.
	typed := walked(t, letters("quit")...)
	_, cmd := typed.commit()
	r.NotNil(cmd)
}
