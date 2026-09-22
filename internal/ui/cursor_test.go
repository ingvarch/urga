package ui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func manyJobs(n int) []nomad.Job {
	jobs := make([]nomad.Job, 0, n)
	for i := range n {
		id := fmt.Sprintf("job-%02d", i)
		jobs = append(jobs, nomad.Job{ID: id, Name: id, Namespace: "production", Type: "service", Status: "running", Running: 1, Desired: 1})
	}

	return jobs
}

func TestCursor_StaysInTheListWhenItShrinks(t *testing.T) {
	r := require.New(t)

	jobs := manyJobs(20)

	m := newTestModel(&fakeClient{jobs: jobs})
	m, _ = m.update(tea.WindowSizeMsg{Width: 120, Height: 14})
	m, _ = m.update(jobsMsg(jobs))

	// The cursor is on the last row of a list longer than the window.
	m, _ = m.update(key('G'))
	r.Equal(19, m.table.cursor)

	// The list shrinks under it, which is what a filter, a namespace switch
	// or a cluster with fewer jobs does.
	m, _ = m.update(key('/'))
	m = typeIn(m, "job-03")

	// The row that is left is on the screen. A cursor past the end scrolls
	// the window past it and the table looks empty.
	out := plain(m.render())
	r.Contains(out, "job-03")
	r.Less(m.table.cursor, len(m.table.rows))
}

func TestCursor_ShrinkingClusterKeepsTheRowsVisible(t *testing.T) {
	r := require.New(t)

	jobs := manyJobs(20)

	m := newTestModel(&fakeClient{jobs: jobs})
	m, _ = m.update(tea.WindowSizeMsg{Width: 120, Height: 14})
	m, _ = m.update(jobsMsg(jobs))
	m, _ = m.update(key('G'))

	// The cluster answers with a shorter list than the one on the screen.
	m, _ = m.update(jobsMsg(jobs[:2]))

	out := plain(m.render())
	r.Contains(out, "job-00")
	r.Contains(out, "job-01")
}
