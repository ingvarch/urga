package ui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/require"
)

// tagged is a text of two lines from two allocations, each tagged with its
// own id in its own colour.
func tagged() (textModel, lipgloss.Style, lipgloss.Style) {
	first := lipgloss.NewStyle().Foreground(colorTitle)
	second := lipgloss.NewStyle().Foreground(colorCanary)

	t := textModel{
		lines: []string{"GET /health 200", "worker started " + strings.Repeat("x", 40)},
		tags: map[int]tag{
			0: {text: "9a1b2c3d │ ", style: first},
			1: {text: "4f2a1c9e │ ", style: second},
		},
	}
	t.setSize(40, 10)

	return t, first, second
}

func TestText_ATagInFrontOfALine(t *testing.T) {
	r := require.New(t)

	text, first, second := tagged()

	// The tag in its colour, the line in the colour of text.
	rows := strings.Split(text.view(), "\n")
	r.True(strings.HasPrefix(rows[0], opening(first)+"9a1b2c3d │ "), rows[0])
	r.Contains(rows[0], opening(styleText)+"GET /health 200")
	r.True(strings.HasPrefix(rows[1], opening(second)+"4f2a1c9e │ "), rows[1])

	// It is read with the line: the filter finds the lines of one
	// allocation, and what is saved says where each line came from.
	text.filter = "4f2a"
	r.NotContains(text.view(), "GET /health")
	r.Contains(text.view(), "worker started")
	r.Equal("4f2a1c9e │ worker started "+strings.Repeat("x", 40), text.rows()[0].text)
}

func TestText_ATagThroughTheWrapAndTheTimes(t *testing.T) {
	r := require.New(t)

	text, _, second := tagged()
	text.wrap = true
	text.times = true
	text.stamps = map[int]time.Time{1: time.Date(2026, 9, 24, 18, 26, 44, 0, time.UTC)}

	rows := text.rows()

	// The time a line arrived comes first, then where it came from; a line
	// laid over several rows says where it came from once.
	r.True(strings.HasPrefix(rows[1].text, "18:26:44  4f2a1c9e │ worker"), rows[1].text)

	view := strings.Split(text.view(), "\n")
	r.Contains(view[1], opening(second)+"4f2a1c9e │ ")
	r.NotContains(view[2], opening(second))
}
