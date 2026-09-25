package ui

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

// randomTable is a table of up to eight columns whose titles and cells are
// between empty and wider than a column may be.
func randomTable(rng *rand.Rand) ([]string, []tableRow) {
	text := func() string { return strings.Repeat("x", rng.IntN(maxColumnWidth+10)) }

	titles := make([]string, 1+rng.IntN(8))
	for i := range titles {
		titles[i] = text()
	}

	rows := make([]tableRow, rng.IntN(6))
	for i := range rows {
		cells := make([]string, len(titles))
		for j := range cells {
			cells[j] = text()
		}

		rows[i] = tableRow{cells: cells}
	}

	return titles, rows
}

// natural is how wide a column would like to be: its widest text, within the
// bounds a column keeps.
func natural(titles []string, rows []tableRow) []int {
	widths := make([]int, len(titles))
	for i, title := range titles {
		widths[i] = ansi.StringWidth(title)

		for _, row := range rows {
			widths[i] = max(widths[i], ansi.StringWidth(row.cells[i]))
		}

		widths[i] = max(minColumnWidth, min(widths[i], maxColumnWidth))
	}

	return widths
}

// Whatever the width of the terminal, the columns are laid out the same way:
// the list fills the line when it fits, and gives way from the widest column
// when it does not.
func TestColumnWidths_AtEveryWidth(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 11))

	for range 300 {
		titles, rows := randomTable(rng)
		want := natural(titles, rows)

		for width := 0; width <= 200; width++ {
			widths := columnWidths(titles, rows, width)
			available := width - 2*tableIndent - gapsWidth(len(titles))

			require.Len(t, widths, len(titles))

			for i, w := range widths {
				require.GreaterOrEqual(t, w, minColumnWidth, "width %d, column %d", width, i)
			}

			if total(want) <= available {
				// Room for everything: every column gets what it holds, and the
				// row reaches the right edge.
				require.Equal(t, available, total(widths), "width %d", width)

				// What is left over is shared by what each column holds, a
				// long value counting no more than a column may be wide.
				left := available - total(want)

				for i := range widths {
					share := left * want[i] / total(want)
					require.InDelta(t, share, widths[i]-want[i], 1, "width %d, column %d", width, i)
				}

				continue
			}

			// Not enough room: the row fits, unless every column is already as
			// narrow as a column gets.
			require.LessOrEqual(t, total(widths), max(available, len(titles)*minColumnWidth), "width %d", width)

			// The same table at the same width is laid out the same way.
			require.Equal(t, widths, columnWidths(titles, rows, width))
		}
	}
}
