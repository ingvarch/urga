package ui

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestJobsTitle(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{jobs: twoJobs()})
	m, _ = m.update(jobsMsg(twoJobs()))

	// The namespace and the number of rows, so nobody has to count them.
	r.Equal("Jobs (production) [2]", m.title())

	m.namespace = nomad.AllNamespaces
	r.Equal("Jobs (all) [2]", m.title())

	m.namespace = ""
	r.Equal("Jobs (all) [2]", m.title())
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

	// A service that runs all the allocations it wants has no color.
	r.Nil(jobColor(nomad.Job{Type: "service", Status: "running", Running: 3, Desired: 3}))

	// One that runs fewer than it wants gets the attention color.
	r.Equal(colorAttention, jobColor(nomad.Job{Type: "service", Status: "running", Running: 2, Desired: 3}))

	r.Equal(colorPending, jobColor(nomad.Job{Type: "service", Status: "pending"}))
	r.Equal(colorDead, jobColor(nomad.Job{Type: "service", Status: "dead"}))
	r.Equal(colorDead, jobColor(nomad.Job{Type: "service", Status: "failed"}))

	// A batch job that ended did its work, it is not a failure.
	r.Equal(colorSpent, jobColor(nomad.Job{Type: "batch", Status: "dead"}))
}

func TestJobs_TheKeysOfAJob(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{jobs: twoJobs()})
	m, _ = m.update(jobsMsg(twoJobs()))

	keys := []hint{
		{Key: "<enter>", Description: "Allocations"},
		{Key: "<space>", Description: "Mark"},
		{Key: "<ctrl-a>", Description: "Mark All"},
		{Key: "<t>", Description: "Task Groups"},
		{Key: "<d>", Description: "Describe"},
		{Key: "<h>", Description: "Job Spec"},
		{Key: "<ctrl-s>", Description: "Start/Stop"},
		{Key: "<u>", Description: "Revert"},
		{Key: "<v>", Description: "Versions"},
		{Key: "<l>", Description: "Logs"},
		{Key: "<e>", Description: "Edit"},
	}
	r.Equal(keys, m.hints())

	// A job with an allocation waiting to be placed gets the Placement key.
	m, _ = m.update(jobsMsg(waiting()))
	r.Equal(append(keys, hint{Key: "<p>", Description: "Placement"}), m.hints())
}

func TestJobs_WhatIsMarkedIsWhatAnActionTakes(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{jobs: twoJobs()})
	m, _ = m.update(jobsMsg(twoJobs()))

	// web is marked, and the cursor moves on to cron.
	m, _ = m.update(space())
	m, _ = m.update(key('j'))

	marked := markedRows(m)
	r.Len(marked, 1)
	r.Contains(marked[0], "web")

	// The mark is on the job, whatever order the cluster answers in next.
	m, _ = m.update(jobsMsg([]nomad.Job{twoJobs()[1], twoJobs()[0]}))

	m, _ = m.update(ctrlKey('s'))
	r.Contains(plain(m.render()), "Really stop the job web?")
}

func TestJobs_TheListOfTheRegionLeftIsNotShown(t *testing.T) {
	r := require.New(t)

	m, clusters := onDev(t)
	r.Contains(plain(m.render()), "cron")

	// prod does not answer: the jobs of dev must not show under its name.
	clusters.prod.err = errors.New("connection refused")
	m = typeCommand(m, "ctx prod")

	out := plain(m.render())
	r.Contains(out, "Jobs (payments) [0]")
	r.NotContains(out, "cron")
}
