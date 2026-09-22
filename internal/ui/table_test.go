package ui

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func testTable(rows ...tableRow) tableModel {
	t := newTableModel([]string{"ID", "Name", "Status"})
	t.setSize(60, 3)
	t.setRows(rows)

	return t
}

func row(cells ...string) tableRow {
	return tableRow{cells: cells}
}

// colorCode and styleCode are the escape sequences a color or a style puts in
// front of the text, which is how a test sees what a line is painted with.
func colorCode(c color.Color) string {
	return styleCode(lipgloss.NewStyle().Foreground(c))
}

func styleCode(s lipgloss.Style) string {
	rendered := s.Render("x")

	return rendered[:strings.Index(rendered, "x")]
}

func TestTable_ColumnsFollowTheContent(t *testing.T) {
	r := require.New(t)

	rows := []tableRow{row("api", "api", "running"), row("nightly", "nightly", "dead")}

	widths := columnWidths([]string{"ID", "Name", "Status"}, rows, 200)

	// A column is never narrower than what it holds.
	for i, want := range []int{7, 7, 7} {
		r.GreaterOrEqual(widths[i], want)
	}

	// What is left over is shared out, so the row reaches the right edge
	// instead of huddling on the left.
	r.Equal(200, total(widths)+gapsWidth(3)+2*tableIndent)

	for i := 1; i < len(widths); i++ {
		r.InDelta(widths[0], widths[i], 1, "columns that hold the same get the same")
	}
}

func TestTable_WideColumnsGetMoreOfTheSpace(t *testing.T) {
	r := require.New(t)

	rows := []tableRow{row("hcloud-csi-controller", "1/1")}

	widths := columnWidths([]string{"ID", "Allocs"}, rows, 120)

	// The column with the names keeps more of the room than the one with a
	// count in it.
	r.Greater(widths[0], widths[1])
	r.Equal(120, total(widths)+gapsWidth(2)+2*tableIndent)
}

func TestTable_ColumnsShrinkToFit(t *testing.T) {
	r := require.New(t)

	long := strings.Repeat("x", 80)

	widths := columnWidths([]string{"ID", "Name", "Status"}, []tableRow{row(long, long, "running")}, 40)

	total := 0
	for _, w := range widths {
		r.GreaterOrEqual(w, minColumnWidth)
		total += w
	}

	// Everything fits in the width given, gaps included.
	r.LessOrEqual(total+gapsWidth(3)+2*tableIndent, 40)
}

func TestTable_LongValueIsEaten(t *testing.T) {
	r := require.New(t)

	tbl := testTable(row(strings.Repeat("hcloud-csi-controller-", 4), "x", "running"))

	out := plain(tbl.view())

	r.Contains(out, "…")
	for _, line := range strings.Split(out, "\n") {
		r.LessOrEqual(ansi.StringWidth(line), 60)
	}
}

func TestTable_HeaderAndRows(t *testing.T) {
	r := require.New(t)

	tbl := testTable(row("api", "api", "running"), row("cron", "cron", "dead"))

	out := lines(tbl.view())

	// The header spreads with the columns.
	r.Contains(out[0], "ID")
	r.Contains(out[0], "Status")
	r.Contains(out[1], "api")
	r.Contains(out[2], "cron")
}

func TestTable_CursorMoves(t *testing.T) {
	r := require.New(t)

	tbl := testTable(row("a"), row("b"), row("c"))

	r.Equal(0, tbl.cursor)

	tbl.move(1)
	r.Equal(1, tbl.cursor)

	tbl.move(-5)
	r.Equal(0, tbl.cursor, "the cursor stops at the first row")

	tbl.move(9)
	r.Equal(2, tbl.cursor, "and at the last one")
}

func TestTable_CursorStaysOnTheScreen(t *testing.T) {
	r := require.New(t)

	rows := make([]tableRow, 0, 20)
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		rows = append(rows, row(name, name, "running"))
	}

	tbl := testTable(rows...)

	tbl.move(9)
	out := lines(tbl.view())

	// Three rows fit, the window follows the cursor to the last one.
	r.Len(out, 4)
	r.Contains(out[3], "j")
	r.NotContains(plain(tbl.view()), " a ")
}

func TestTable_SelectedRowReads(t *testing.T) {
	r := require.New(t)

	tbl := testTable(row("api", "api", "running"), row("cron", "cron", "dead"))
	tbl.rows[1].color = colorDead

	// The row under the cursor is drawn in the selection colors from end to
	// end. A row that keeps its own color under the highlight is unreadable.
	tbl.move(1)
	selected := strings.Split(tbl.view(), "\n")[2]

	r.Contains(selected, styleCode(styleSelected))
	r.NotContains(selected, colorCode(colorDead))
}

func TestTable_RowKeepsItsColor(t *testing.T) {
	r := require.New(t)

	tbl := testTable(row("api", "api", "running"), row("cron", "cron", "dead"))
	tbl.rows[1].color = colorDead

	// A row that is not under the cursor says what state it is in.
	out := strings.Split(tbl.view(), "\n")[2]
	r.Contains(out, colorCode(colorDead))
}

func TestTable_SelectedRow(t *testing.T) {
	r := require.New(t)

	tbl := testTable(row("api"), row("cron"))
	tbl.move(1)

	selected, ok := tbl.selected()
	r.True(ok)
	r.Equal("cron", selected.cells[0])

	empty := testTable()
	_, ok = empty.selected()
	r.False(ok, "an empty list has nothing under the cursor")
}
