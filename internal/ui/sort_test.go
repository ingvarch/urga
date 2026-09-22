package ui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func cellsOf(tbl tableModel, column int) []string {
	out := []string{}
	for _, row := range tbl.rows {
		out = append(out, row.cells[column])
	}

	return out
}

func TestSort_ByAColumn(t *testing.T) {
	r := require.New(t)

	tbl := testTable(row("web", "b", "running"), row("api", "c", "dead"), row("cron", "a", "pending"))

	tbl.sortBy(0)
	r.Equal([]string{"api", "cron", "web"}, cellsOf(tbl, 0))

	// The same column again turns the list around.
	tbl.sortBy(0)
	r.Equal([]string{"web", "cron", "api"}, cellsOf(tbl, 0))

	// Another column starts over, the right way up.
	tbl.sortBy(1)
	r.Equal([]string{"a", "b", "c"}, cellsOf(tbl, 1))
}

func TestSort_ShowsWhichColumn(t *testing.T) {
	r := require.New(t)

	tbl := testTable(row("web", "b", "running"))
	tbl.sortBy(0)

	header := lines(tbl.view())[0]
	r.Contains(header, "ID ↑")

	tbl.sortBy(0)
	r.Contains(lines(tbl.view())[0], "ID ↓")
}

func TestSort_AgeGoesByTime(t *testing.T) {
	r := require.New(t)

	tbl := newTableModel([]string{"ID", "Age"})
	tbl.setSize(60, 5)
	tbl.setRows([]tableRow{row("a", "5h"), row("b", "2d"), row("c", "45s"), row("d", "10m")})

	tbl.sortBy(1)

	// The youngest first: seconds, minutes, hours, days. Read as text, "2d"
	// would come before "5h".
	r.Equal([]string{"45s", "10m", "5h", "2d"}, cellsOf(tbl, 1))
}

func TestSort_NumbersGoByValue(t *testing.T) {
	r := require.New(t)

	tbl := newTableModel([]string{"ID", "Allocs"})
	tbl.setSize(60, 5)
	tbl.setRows([]tableRow{row("a", "9/9"), row("b", "10/10"), row("c", "2/2")})

	tbl.sortBy(1)

	r.Equal([]string{"2/2", "9/9", "10/10"}, cellsOf(tbl, 1))
}

func TestSort_KeepsTheCursorOnItsRow(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)

	// The cursor is on web, the second row after sorting by name.
	m, _ = m.update(key('N'))

	out := plain(m.render())
	r.True(strings.Index(out, "cron") < strings.Index(out, "web"), out)

	m, _ = m.update(enter())

	// Enter opens what the cursor is on now, not what was there before.
	r.Equal("cron", m.screen.jobID)
}

func TestSort_ByTheFirstLetterOfAColumn(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)

	// Shift and the letter a column starts with, the way k9s does it.
	m, _ = m.update(key('S'))
	r.Equal(4, m.sort.column, "Status")

	m, _ = m.update(key('T'))
	r.Equal(2, m.sort.column, "Type")

	// A letter no column starts with leaves the list alone.
	m, _ = m.update(key('X'))
	r.Equal(2, m.sort.column)
}
