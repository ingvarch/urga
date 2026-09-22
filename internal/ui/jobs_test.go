package ui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestJobsTitle(t *testing.T) {
	r := require.New(t)

	// The namespace and the number of rows, so the list says what it holds
	// without counting.
	r.Equal("Jobs (production) [2]", jobsTitle("production", 2))
	r.Equal("Jobs (all) [5]", jobsTitle(nomad.AllNamespaces, 5))
	r.Equal("Jobs (all) [0]", jobsTitle("", 0))
}

func TestJobRows(t *testing.T) {
	r := require.New(t)

	jobs := []nomad.Job{
		{
			ID:         "web",
			Name:       "web",
			Namespace:  "production",
			Type:       "service",
			Status:     "running",
			Running:    3,
			Desired:    4,
			SubmitTime: time.Now().Add(-2 * time.Hour),
		},
		{ID: "cron", Name: "cron", Namespace: "default", Type: "batch", Status: "dead"},
	}

	rows := jobRows(jobs)
	r.Len(rows, 2)

	r.Equal([]string{"web", "web", "service", "production", "running", "3/4", "2h"}, []string(rows[0]))
	r.Equal([]string{"cron", "cron", "batch", "default", "dead", "0/0", "-"}, []string(rows[1]))
}

func TestJobColumns_FillTheWidth(t *testing.T) {
	r := require.New(t)

	for _, width := range []int{80, 120, 200} {
		columns := jobColumns(width)

		// The columns take the whole width, together with the padding the
		// table puts around each cell. A column short of it leaves a gap in
		// the border, one over it wraps the row.
		total := 0
		for _, column := range columns {
			total += column.Width + cellPadding
		}

		r.Equal(width, total, "width %d", width)
	}
}

func TestJobColumns_NarrowTerminal(t *testing.T) {
	r := require.New(t)

	columns := jobColumns(20)

	// Every column keeps a width it can show something in, a negative one
	// drops the column silently.
	for _, column := range columns {
		r.GreaterOrEqual(column.Width, 1, column.Title)
	}
}
