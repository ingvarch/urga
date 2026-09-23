package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

func down() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyDown} }
func up() tea.KeyPressMsg   { return tea.KeyPressMsg{Code: tea.KeyUp} }
func tab() tea.KeyPressMsg  { return tea.KeyPressMsg{Code: tea.KeyTab} }

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
	m, _ = m.update(key(':'))
	m, _ = m.update(down())

	// Everything happens in the line: what it offers never pushes the rows
	// down the screen.
	r.Equal(promptHeight, m.promptRows())
	r.Contains(plain(m.render()), "Jobs (production)")
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
