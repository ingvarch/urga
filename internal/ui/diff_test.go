package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// opening is how a style starts on the screen, before the text it paints.
func opening(style lipgloss.Style) string {
	return strings.Split(style.Render("\x00"), "\x00")[0]
}

// texts are the words of painted lines.
func texts(lines []paintedLine) []string {
	words := make([]string, 0, len(lines))
	for _, line := range lines {
		words = append(words, line.text)
	}

	return words
}

func TestDiffLines(t *testing.T) {
	r := require.New(t)

	lines := diffLines([]nomad.DiffLine{
		{Kind: nomad.DiffContext, Text: `group "web" {`},
		{Kind: nomad.DiffDeleted, Indent: 1, Text: "count = 1"},
		{Kind: nomad.DiffAdded, Indent: 1, Text: "count = 3"},
		{},
		{Kind: nomad.DiffContext, Text: "}"},
	})

	// As git diff reads: a column for what happened to the line, then the
	// line as deep in the job as it sits.
	r.Equal([]string{
		`  group "web" {`,
		`-   count = 1`,
		`+   count = 3`,
		``,
		`  }`,
	}, texts(lines))

	// Added lines green, deleted red, and what leads to them steps back.
	r.Equal(opening(styleMuted), opening(*lines[0].style))
	r.Equal(opening(styleDeleted), opening(*lines[1].style))
	r.Equal(opening(styleAdded), opening(*lines[2].style))
	r.Nil(lines[3].style)
}

func TestText_PaintsALineWholeThroughTheFilterAndTheWrap(t *testing.T) {
	r := require.New(t)

	text := paintedText([]paintedLine{
		{text: "plain"},
		{text: "+ count = 3 " + strings.Repeat("x", 40), style: &styleAdded},
	})
	text.setSize(20, 10)

	// A painted line keeps its colour on every row it wraps onto, and the
	// filter reads its words, not its colour.
	text.wrap = true
	text.filter = "count"

	view := text.view()
	r.NotContains(view, "plain")
	r.Contains(view, opening(styleAdded)+"+ ")

	rows := strings.Split(view, "\n")
	r.Len(rows, 3)

	for _, row := range rows {
		r.True(strings.HasPrefix(row, opening(styleAdded)), row)
	}

	// What is saved is the words.
	for _, row := range text.rows() {
		r.NotContains(row.text, "\x1b")
	}
}
