package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	// cellGap is the space between two columns, tableIndent the one before
	// the first.
	cellGap     = 2
	tableIndent = 1

	minColumnWidth = 1
	maxColumnWidth = 48
)

// tableRow is one resource. The color says what state it is in, and marked
// that an action is to take it along with the others.
type tableRow struct {
	cells  []string
	color  color.Color
	marked bool
}

// tableModel is the list every screen shows: a header, rows, a cursor and a
// window that follows it.
type tableModel struct {
	titles []string
	rows   []tableRow

	cursor int
	top    int

	sort sortState

	width  int
	height int
}

func newTableModel(titles []string) tableModel {
	return tableModel{titles: titles, sort: newSortState()}
}

func (t *tableModel) setSize(width, height int) {
	t.width, t.height = width, max(height, 1)
	t.follow()
}

// show puts rows on the table, in the order they are given. The cursor keeps
// its place as far as the new list allows: a cursor past the end scrolls the
// window past the rows and the table looks empty.
func (t *tableModel) show(rows []tableRow, order sortState) {
	t.sort = order
	t.rows = rows
	t.cursor = clamp(t.cursor, 0, len(rows)-1)
	t.follow()
}

// selected is the row the cursor is on.
func (t tableModel) selected() (tableRow, bool) {
	if t.cursor < 0 || t.cursor >= len(t.rows) {
		return tableRow{}, false
	}

	return t.rows[t.cursor], true
}

func (t *tableModel) move(delta int) {
	t.cursor = clamp(t.cursor+delta, 0, len(t.rows)-1)
	t.follow()
}

// follow keeps the cursor inside the window, and the window over the rows: a
// window that starts past what is left shows blank lines under a full list.
func (t *tableModel) follow() {
	if t.cursor < t.top {
		t.top = t.cursor
	}

	if t.cursor >= t.top+t.height {
		t.top = t.cursor - t.height + 1
	}

	t.top = clamp(t.top, 0, max(len(t.rows)-t.height, 0))
}

func (t tableModel) view() string {
	widths := columnWidths(t.titles, t.rows, t.width)

	titles := make([]string, len(t.titles))
	for i, title := range t.titles {
		titles[i] = title + t.sort.marker(i)
	}

	out := []string{styleTableHeader.Render(t.line(titles, widths))}

	for i := t.top; i < len(t.rows) && i < t.top+t.height; i++ {
		line := t.line(t.rows[i].cells, widths)

		if t.rows[i].marked {
			line = markGlyph + line[1:]
		}

		switch {
		case i == t.cursor:
			line = styleSelected.Render(line)
		case t.rows[i].color != nil:
			line = lipgloss.NewStyle().Foreground(t.rows[i].color).Render(line)
		default:
			line = styleText.Render(line)
		}

		out = append(out, line)
	}

	return strings.Join(out, "\n")
}

// line lays the cells out over the columns and pads the result to the whole
// width, so that a color reaches the end of the row.
func (t tableModel) line(cells []string, widths []int) string {
	parts := make([]string, 0, len(widths))
	for i, width := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}

		parts = append(parts, pad(truncate(cell, width), width))
	}

	line := strings.Repeat(" ", tableIndent) + strings.Join(parts, strings.Repeat(" ", cellGap))

	return pad(truncate(line, t.width), t.width)
}

// columnWidths gives every column the width of the widest thing in it, then
// takes width away from the greediest ones until the row fits.
func columnWidths(titles []string, rows []tableRow, width int) []int {
	widths := make([]int, len(titles))
	for i, title := range titles {
		widths[i] = ansi.StringWidth(title)
	}

	for _, row := range rows {
		for i, cell := range row.cells {
			if i < len(widths) {
				widths[i] = max(widths[i], ansi.StringWidth(cell))
			}
		}
	}

	for i := range widths {
		widths[i] = clamp(widths[i], minColumnWidth, maxColumnWidth)
	}

	// The same space is left on both sides, so the last column does not lean
	// on the border.
	available := width - 2*tableIndent - gapsWidth(len(widths))

	shrinkToFit(widths, available)
	spread(widths, available)

	return widths
}

// spread shares what is left over between the columns, so the row reaches the
// right edge instead of huddling on the left. A column that holds more gets
// more of it, which keeps a name column wide and a count column narrow.
func spread(widths []int, available int) {
	left := available - total(widths)
	if left <= 0 || len(widths) == 0 {
		return
	}

	content := total(widths)
	given := 0

	for i := range widths {
		share := left * widths[i] / content
		widths[i] += share
		given += share
	}

	// What does not divide evenly goes to the first columns, which are the
	// ones holding names.
	for i := 0; i < left-given; i++ {
		widths[i%len(widths)]++
	}
}

// shrinkToFit takes from the widest column first, so that the short ones keep
// what they have.
func shrinkToFit(widths []int, available int) {
	for total(widths) > available {
		widest, index := 0, -1
		for i, w := range widths {
			if w > widest {
				widest, index = w, i
			}
		}

		if index < 0 || widths[index] <= minColumnWidth {
			return
		}

		widths[index]--
	}
}

func gapsWidth(columns int) int {
	if columns <= 1 {
		return 0
	}

	return (columns - 1) * cellGap
}

func total(widths []int) int {
	sum := 0
	for _, w := range widths {
		sum += w
	}

	return sum
}

func clamp(v, low, high int) int {
	if high < low {
		return low
	}

	return min(max(v, low), high)
}
