package ui

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// troubledAllocs are allocations of web: one that restarted ten minutes ago,
// one the kernel killed for its memory five minutes ago, one that restarted
// hours ago, one killed for its memory hours ago, and one that failed.
func troubledAllocs() []nomad.Alloc {
	now := time.Now()

	alloc := func(id, status string, version uint64, tasks ...nomad.Task) nomad.Alloc {
		return nomad.Alloc{
			ID: id, Namespace: "production", JobID: "web", TaskGroup: "frontend",
			Status: status, JobVersion: version, Tasks: tasks,
		}
	}
	oomAt := func(at time.Time) []nomad.TaskEvent {
		return []nomad.TaskEvent{{Time: at, Type: "Terminated", OOM: true}}
	}

	return []nomad.Alloc{
		alloc("aaaa1111", statusRunning, 3,
			nomad.Task{Name: "server", Restarts: 2, LastRestart: now.Add(-10 * time.Minute)},
			nomad.Task{Name: "proxy", Restarts: 1, LastRestart: now.Add(-3 * time.Hour)}),
		alloc("bbbb2222", statusRunning, 2,
			nomad.Task{Name: "server", Restarts: 1, LastRestart: now.Add(-6 * time.Hour), Events: oomAt(now.Add(-5 * time.Minute))}),
		alloc("cccc3333", statusRunning, 3, nomad.Task{Name: "server", Restarts: 4, LastRestart: now.Add(-3 * time.Hour)}),
		alloc("dddd4444", statusRunning, 3, nomad.Task{Name: "server", Events: oomAt(now.Add(-2 * time.Hour))}),
		alloc("eeee5555", statusFailed, 3, nomad.Task{Name: "server", Restarts: 1, LastRestart: now.Add(-time.Minute)}),
	}
}

// cellOf is the cell of a row under a column.
func cellOf(titles []string, row tableRow, title string) string {
	return row.cells[slices.Index(titles, title)]
}

func TestAllocations_ShowTheVersionRestartsAndOOM(t *testing.T) {
	r := require.New(t)

	r.Equal([]string{
		"ID", "TaskGroup", "JobID", "Ver", "Namespace", "Node", "Status", "Desired", "Rst", "OOM", "CPU", "MEM", "Age",
	}, allocTitles)

	rows := allocRows(troubledAllocs(), nil)

	r.Equal("3", cellOf(allocTitles, rows[0], "Ver"))
	r.Equal("2", cellOf(allocTitles, rows[1], "Ver"))

	// The restarts of every task of the allocation.
	r.Equal("3", cellOf(allocTitles, rows[0], "Rst"))
	r.Equal("0", cellOf(allocTitles, rows[3], "Rst"))

	r.Empty(cellOf(allocTitles, rows[0], "OOM"))
	r.Equal("yes", cellOf(allocTitles, rows[1], "OOM"))
	r.Equal("yes", cellOf(allocTitles, rows[3], "OOM"))
}

func TestAllocations_ARecentRestartOrOOMNeedsAttention(t *testing.T) {
	r := require.New(t)

	rows := allocRows(troubledAllocs(), nil)

	// A restart or an OOM kill of the last hour is a task in trouble now.
	r.Equal(colorAttention, rows[0].color)
	r.Equal(colorAttention, rows[1].color)

	// One of hours ago is history.
	r.Nil(rows[2].color)
	r.Nil(rows[3].color)

	// A failed allocation stays red.
	r.Equal(colorDead, rows[4].color)

	// So the rows that need attention keep them.
	m, _ := onAllocations(t)
	m, _ = m.update(allocsMsg(troubledAllocs()))
	m, _ = m.update(key('!'))

	r.Len(m.list.table.rows, 3)
}

func TestDeployment_AllocationsShowRestartsAndOOM(t *testing.T) {
	r := require.New(t)

	rows := deploymentAllocRows(troubledAllocs(), nil)

	r.Equal("3", cellOf(deploymentAllocTitles, rows[0], "Rst"))
	r.Equal("yes", cellOf(deploymentAllocTitles, rows[1], "OOM"))
	r.Equal(colorAttention, rows[0].color)
}
