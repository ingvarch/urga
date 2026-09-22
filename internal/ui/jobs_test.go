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

	r.Equal([]string{"web", "web", "service", "production", "running", "3/4", "2h"}, rows[0].cells)
	r.Equal([]string{"cron", "cron", "batch", "default", "dead", "0/0", "-"}, rows[1].cells)
}

func TestJobColor(t *testing.T) {
	r := require.New(t)

	// A service that runs everything it asks for is quiet.
	r.Nil(jobColor(nomad.Job{Type: "service", Status: "running", Running: 3, Desired: 3}))

	// One that is short of allocations is not.
	r.Equal(colorAttention, jobColor(nomad.Job{Type: "service", Status: "running", Running: 2, Desired: 3}))

	r.Equal(colorPending, jobColor(nomad.Job{Type: "service", Status: "pending"}))
	r.Equal(colorDead, jobColor(nomad.Job{Type: "service", Status: "dead"}))
	r.Equal(colorDead, jobColor(nomad.Job{Type: "service", Status: "failed"}))

	// A batch job that ended did its work, it is not a failure.
	r.Equal(colorSpent, jobColor(nomad.Job{Type: "batch", Status: "dead"}))
}
