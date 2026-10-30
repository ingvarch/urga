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

	rows := jobRows(jobs, nil, nil)
	r.Len(rows, 2)

	r.Equal([]string{"web", "web", "service", "production", "running", "3/4", "2h"}, rows[0].cells)
	r.Equal([]string{"cron", "cron", "batch", "default", "dead", "0/0", "-"}, rows[1].cells)
}

func TestJobColor(t *testing.T) {
	r := require.New(t)

	// A service that runs all the allocations it wants has no color.
	r.Nil(jobColor(nomad.Job{Type: "service", Status: "running", Running: 3, Desired: 3}))

	// One that runs fewer than it wants is not ready yet: red.
	r.Equal(colorDead, jobColor(nomad.Job{Type: "service", Status: "running", Running: 2, Desired: 3}))

	r.Equal(colorPending, jobColor(nomad.Job{Type: "service", Status: "pending"}))
	r.Equal(colorDead, jobColor(nomad.Job{Type: "service", Status: "dead"}))
	r.Equal(colorDead, jobColor(nomad.Job{Type: "service", Status: "failed"}))

	// A batch job that ended did its work, it is not a failure.
	r.Equal(colorSpent, jobColor(nomad.Job{Type: "batch", Status: "dead"}))

	// One that ended with a group that failed is.
	r.Equal(colorDead, jobColor(nomad.Job{Type: "batch", Status: "dead", Failed: true}))
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
		{Key: "<m>", Description: "Meta"},
		{Key: "<ctrl-s>", Description: "Start/Stop"},
		{Key: "<ctrl-e>", Description: "Evaluate"},
		{Key: "<ctrl-r>", Description: "Reschedule Failed"},
		{Key: "<u>", Description: "Revert"},
		{Key: "<v>", Description: "Versions"},
		{Key: "<l>", Description: "Logs"},
		{Key: "<w>", Description: "Watch"},
		{Key: "<o>", Description: "Events"},
		{Key: "<e>", Description: "Edit"},
		{Key: "<n>", Description: "New"},
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

func TestJobs_BatchQueuePositionInRows(t *testing.T) {
	r := require.New(t)

	jobs := []nomad.Job{
		{ID: "batch-placing", Namespace: "default", Type: "batch", Status: "pending"},
		{ID: "batch-queued-0", Namespace: "default", Type: "batch", Status: "pending"},
		{ID: "batch-queued-1", Namespace: "default", Type: "batch", Status: "pending"},
		{ID: "batch-unknown", Namespace: "default", Type: "batch", Status: "pending"},
		{ID: "service-pending", Namespace: "default", Type: "service", Status: "pending"},
		{ID: "batch-running", Namespace: "default", Type: "batch", Status: "running"},
	}

	queue := map[string]nomad.QueueWorkload{
		"default/batch-placing":   {JobID: "batch-placing", Namespace: "default", Status: "placing", Position: 0},
		"default/batch-queued-0":  {JobID: "batch-queued-0", Namespace: "default", Status: "queued", Position: 0},
		"default/batch-queued-1":  {JobID: "batch-queued-1", Namespace: "default", Status: "queued", Position: 1},
		"default/service-pending": {JobID: "service-pending", Namespace: "default", Status: "queued", Position: 2},
		"default/batch-running":   {JobID: "batch-running", Namespace: "default", Status: "queued", Position: 3},
	}

	rows := jobRows(jobs, nil, queue)
	r.Len(rows, 6)

	r.Equal("placing", rows[0].cells[4])
	r.Equal("queued 1", rows[1].cells[4])
	r.Equal("queued 2", rows[2].cells[4])
	r.Equal("pending", rows[3].cells[4])
	r.Equal("pending", rows[4].cells[4])
	r.Equal("running", rows[5].cells[4])
}

func TestJobs_BatchQueuePositionEndToEnd(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		nodePools: []nomad.NodePool{{Name: "batch", Queue: "fifo"}},
		queueWorkloads: map[string][]nomad.QueueWorkload{
			"batch": {
				{JobID: "batch-1", Namespace: "default", Status: "placing", Position: 0},
				{JobID: "batch-2", Namespace: "default", Status: "queued", Position: 1},
			},
		},
		jobs: []nomad.Job{
			{ID: "batch-1", Name: "batch-1", Namespace: "default", Type: "batch", Status: "pending"},
			{ID: "batch-2", Name: "batch-2", Namespace: "default", Type: "batch", Status: "pending"},
		},
	}

	m := newTestModel(client)
	m = follow(m, m.fetch(), 3)

	out := plain(m.render())
	r.Contains(out, "batch-1")
	r.Contains(out, "placing")
	r.Contains(out, "batch-2")
	r.Contains(out, "queued 2")
}

func TestJobs_BatchQueuePositionInPoolPage(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		queueWorkloads: map[string][]nomad.QueueWorkload{
			"gpu": {
				{JobID: "gpu-batch", Namespace: "default", Status: "queued", Position: 0},
			},
		},
		poolJobs: []nomad.Job{
			{ID: "gpu-batch", Name: "gpu-batch", Namespace: "default", Type: "batch", Status: "pending"},
		},
	}

	m := newTestModel(client)
	m, _ = m.update(openMsg{jobsPage{pool: "gpu"}})
	m = follow(m, m.fetch(), 3)

	out := plain(m.render())
	r.Contains(out, "gpu-batch")
	r.Contains(out, "queued 1")
}

func TestJobs_NoPendingBatchDoesNotFetchQueue(t *testing.T) {
	r := require.New(t)

	called := false
	client := &fakeClient{
		jobs: []nomad.Job{
			{ID: "svc-1", Name: "svc-1", Namespace: "default", Type: "service", Status: "pending"},
			{ID: "batch-done", Name: "batch-done", Namespace: "default", Type: "batch", Status: "dead"},
		},
	}

	m := newTestModel(client)
	m = follow(m, m.fetch(), 3)

	r.False(called)
	out := plain(m.render())
	r.Contains(out, "svc-1")
	r.Contains(out, "pending")
	r.Contains(out, "batch-done")
	r.Contains(out, "dead")
}
