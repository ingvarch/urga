package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

// plain is what the screen shows without the colors.
func plain(s string) string {
	return ansi.Strip(s)
}

func lines(s string) []string {
	return strings.Split(plain(s), "\n")
}

func TestFrame(t *testing.T) {
	r := require.New(t)

	out := frame("Jobs (production) [2]", "web\ndatabase", 40, 4)
	rows := lines(out)

	// The title rides in the top border, the way a resource list is labelled.
	r.Contains(rows[0], "Jobs (production) [2]")
	r.True(strings.HasPrefix(rows[0], "╭"))
	r.True(strings.HasSuffix(rows[0], "╮"))

	r.Contains(rows[1], "web")
	r.Contains(rows[2], "database")

	// Every line is exactly as wide as asked, a ragged box shifts the columns
	// underneath it.
	for i, row := range rows {
		r.Equal(40, ansi.StringWidth(row), "line %d: %q", i, row)
	}

	// The box is the height it was given: two border lines and the body.
	r.Len(rows, 4)
}

func TestFrame_TitleLongerThanTheBox(t *testing.T) {
	r := require.New(t)

	out := frame("Jobs (a-very-long-namespace-name) [128]", "web", 20, 3)
	rows := lines(out)

	// A title that does not fit is eaten, it never pushes the corner out.
	r.Equal(20, ansi.StringWidth(rows[0]))
	r.True(strings.HasSuffix(rows[0], "╮"))
}

func TestFrame_ShortBodyFillsTheBox(t *testing.T) {
	r := require.New(t)

	out := frame("Jobs", "web", 20, 6)
	rows := lines(out)

	// The box keeps its height with one row in it, so the screen does not
	// jump while the list fills up.
	r.Len(rows, 6)
	r.Equal(20, ansi.StringWidth(rows[4]))
}
