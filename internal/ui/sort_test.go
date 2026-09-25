package ui

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// sortTable orders the rows of a table the way the model does: one sort, one
// hand-over.
func sortTable(tbl *tableModel, column int) {
	order := tbl.sort.by(column)

	rows, _ := sortRows(tbl.rows, nil, order, tbl.titles)
	tbl.show(rows, order)
}

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

	sortTable(&tbl, 0)
	r.Equal([]string{"api", "cron", "web"}, cellsOf(tbl, 0))

	// The same column again turns the list around.
	sortTable(&tbl, 0)
	r.Equal([]string{"web", "cron", "api"}, cellsOf(tbl, 0))

	// Another column starts over, the right way up.
	sortTable(&tbl, 1)
	r.Equal([]string{"a", "b", "c"}, cellsOf(tbl, 1))
}

func TestSort_ShowsWhichColumn(t *testing.T) {
	r := require.New(t)

	tbl := testTable(row("web", "b", "running"))
	sortTable(&tbl, 0)

	header := lines(tbl.view())[0]
	r.Contains(header, "ID ↑")

	sortTable(&tbl, 0)
	r.Contains(lines(tbl.view())[0], "ID ↓")
}

// agedRow is a row whose second cell says how long ago it was, the way the
// screens build one.
func agedRow(id string, ago time.Duration) tableRow {
	moment := time.Now().Add(-ago)

	return tableRow{cells: []string{id, ageOf(moment)}, ages: moments{1: moment}}
}

func TestSort_AgeGoesByTime(t *testing.T) {
	r := require.New(t)

	tbl := newTableModel([]string{"ID", "Age"})
	tbl.setSize(60, 5)
	tbl.show([]tableRow{
		agedRow("a", 5*time.Hour), agedRow("b", 48*time.Hour), agedRow("c", 45*time.Second), agedRow("d", 10*time.Minute),
	}, newSortState())

	sortTable(&tbl, 1)

	// The youngest first: seconds, minutes, hours, days. Read as text, "2d"
	// would come before "5h".
	r.Equal([]string{"45s", "10m", "5h", "2d"}, cellsOf(tbl, 1))
}

// Every list that shows how long ago something was orders that column by
// time, whatever the column is called.
func TestSort_EveryAgeColumnGoesByTime(t *testing.T) {
	newer, older := time.Now().Add(-30*time.Minute), time.Now().Add(-time.Hour)

	lists := []struct {
		name   string
		titles []string
		column string
		rows   func(at time.Time) []tableRow
	}{
		{"jobs", jobTitles, "Age", func(at time.Time) []tableRow {
			return jobRows([]nomad.Job{{SubmitTime: at}})
		}},
		{"allocations", allocTitles, "Age", func(at time.Time) []tableRow {
			return allocRows([]nomad.Alloc{{Created: at}}, nil)
		}},
		{"allocations of a deployment", deploymentAllocTitles, "Age", func(at time.Time) []tableRow {
			return deploymentAllocRows([]nomad.Alloc{{Created: at}}, nil)
		}},
		{"tasks", taskTitles, "Started", func(at time.Time) []tableRow {
			return taskRows([]nomad.Task{{Started: at}})
		}},
		{"task events", taskEventTitles, "Age", func(at time.Time) []tableRow {
			return taskEventRows([]nomad.TaskEvent{{Time: at}})
		}},
		{"client events", nodeEventTitles, "Age", func(at time.Time) []tableRow {
			return nodeEventRows([]nomad.NodeEvent{{Time: at}})
		}},
		{"drivers", driverTitles, "Updated", func(at time.Time) []tableRow {
			return driverRows([]nomad.Driver{{Updated: at}})
		}},
		{"files", fileTitles, "Modified", func(at time.Time) []tableRow {
			return fileRows([]nomad.File{{Name: "stdout", Modified: at}})
		}},
		{"evaluations", evaluationTitles, "Age", func(at time.Time) []tableRow {
			return evaluationRows([]nomad.Evaluation{{Created: at}})
		}},
		{"variables by age", variableTitles, "Age", func(at time.Time) []tableRow {
			return variableRows([]nomad.Variable{{Created: at}})
		}},
		{"variables by change", variableTitles, "Modified", func(at time.Time) []tableRow {
			return variableRows([]nomad.Variable{{Modified: at}})
		}},
		{"versions", versionTitles, "Age", func(at time.Time) []tableRow {
			return versionRows([]nomad.JobVersion{{Submitted: at}})
		}},
	}

	for _, list := range lists {
		t.Run(list.name, func(t *testing.T) {
			column := slices.Index(list.titles, list.column)
			require.GreaterOrEqual(t, column, 0)

			tbl := newTableModel(list.titles)
			tbl.show(append(list.rows(newer), list.rows(older)...), newSortState())

			sortTable(&tbl, column)

			// Read as text, "1h" comes before "30m" and is older.
			require.Equal(t, []string{"30m", "1h"}, cellsOf(tbl, column))
		})
	}
}

// What has not happened yet reads "-" and goes first, with the youngest.
func TestSort_AnAgeThatIsNotThereCountsAsNew(t *testing.T) {
	r := require.New(t)

	tbl := newTableModel(taskTitles)
	tbl.show(taskRows([]nomad.Task{{Name: "web", Started: time.Now().Add(-time.Hour)}, {Name: "sidecar"}}), newSortState())

	sortTable(&tbl, slices.Index(taskTitles, "Started"))

	r.Equal([]string{"-", "1h"}, cellsOf(tbl, slices.Index(taskTitles, "Started")))
}

func TestSort_NumbersGoByValue(t *testing.T) {
	r := require.New(t)

	tbl := newTableModel([]string{"ID", "Allocs"})
	tbl.setSize(60, 5)
	tbl.show([]tableRow{row("a", "9/9"), row("b", "10/10"), row("c", "2/2")}, newSortState())

	sortTable(&tbl, 1)

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
	r.Contains(plain(m.render()), "Allocations (Job: cron)")
}

func TestSort_ByTheFirstLetterOfAColumn(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)

	// Shift and the letter a column starts with, the way k9s does it.
	m, _ = m.update(key('S'))
	r.Equal(4, m.list.sort.column, "Status")

	m, _ = m.update(key('T'))
	r.Equal(2, m.list.sort.column, "Type")

	// A letter no column starts with leaves the list alone.
	m, _ = m.update(key('X'))
	r.Equal(2, m.list.sort.column)
}
