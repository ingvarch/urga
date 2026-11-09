package ui

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestQueue_ListsQueuedJobsOfEveryPool(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		nodePools: []nomad.NodePool{
			{Name: "batch-1", Queue: nomad.QueueFifo},
			{Name: "batch-2", Queue: nomad.QueueFifo},
			{Name: "default", Queue: ""},
		},
		queueWorkloads: map[string][]nomad.QueueWorkload{
			"batch-1": {
				{JobID: "job-b", Namespace: "prod", Pool: "batch-1", QueueType: nomad.QueueFifo, Status: "queued", Position: 1, CreatedAt: time.Now().Add(-10 * time.Minute)},
			},
			"batch-2": {
				{JobID: "job-a", Namespace: "default", Pool: "batch-2", QueueType: nomad.QueueFifo, Status: "placing", Position: 0, CreatedAt: time.Now().Add(-20 * time.Minute)},
			},
		},
	}

	m, _ := sessionModel(t, client)
	m = typeCommand(m, "queue")

	out := plain(m.render())
	r.Contains(out, "Queue [2]")
	r.Contains(out, "Pos")
	r.Contains(out, "Pool")
	r.Contains(out, "job-a")
	r.Contains(out, "placing")
	r.Contains(out, "job-b")
	r.Contains(out, "queued")

	// Order is by Position: job-a (Pos 1) before job-b (Pos 2).
	r.Contains(fileRow(t, m, 0), "job-a")
	r.Contains(fileRow(t, m, 1), "job-b")
}

func TestQueue_DynamicPriorityColumns(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		queueWorkloads: map[string][]nomad.QueueWorkload{
			"gpu": {
				{
					JobID:               "dyn-1",
					Namespace:           "default",
					Pool:                "gpu",
					QueueType:           nomad.QueueDynamic,
					Status:              "queued",
					Position:            0,
					AdjustedPriority:    105,
					BasePriority:        50,
					FairshareAdjustment: 10,
					AgeAdjustment:       45,
					CPUAdjustment:       5,
					MemoryAdjustment:    -5,
					CreatedAt:           time.Now().Add(-5 * time.Minute),
				},
			},
		},
	}

	m, _ := sessionModel(t, client)
	m, _ = m.update(openMsg{queuePage{pool: "gpu", queueType: nomad.QueueDynamic}})
	m = playOut(m, m.fetch())

	out := plain(m.render())
	r.Contains(out, "Queue (Pool: gpu) [1]")
	r.Contains(out, "Priority")
	r.Contains(out, "Base")
	r.Contains(out, "Fairshare")
	r.Contains(out, "Age Adj")
	r.Contains(out, "CPU Adj")
	r.Contains(out, "Mem Adj")
	r.Contains(out, "dyn-1")
	r.Contains(out, "105")
	r.Contains(out, "50")
	r.Contains(out, "+10")
	r.Contains(out, "+45")
	r.Contains(out, "+5")
	r.Contains(out, "-5")
}

func TestQueue_DynamicFromThePoolOrItsWorkloads(t *testing.T) {
	r := require.New(t)

	opened := func(kind nomad.QueueKind) queuePage { return queuePage{pool: "gpu", queueType: kind} }
	read := func(kind nomad.QueueKind) queuePage {
		return queuePage{pool: "gpu", items: []nomad.QueueWorkload{{JobID: "job-1", QueueType: kind}}}
	}

	r.True(opened(nomad.QueueDynamic).dynamic())
	r.True(read(nomad.QueueDynamic).dynamic())
	r.False(opened(nomad.QueueFifo).dynamic())
	r.False(read(nomad.QueueFifo).dynamic())
}

func TestQueue_TenantsPanel(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		queueWorkloads: map[string][]nomad.QueueWorkload{
			"gpu": {
				{JobID: "job-1", Namespace: "default", Pool: "gpu", QueueType: nomad.QueueDynamic, Status: "placing", Position: 0},
			},
		},
		queueStatus: map[string][]nomad.QueueTenant{
			"gpu": {
				{
					ID:             "team-alpha",
					PercentageUsed: 65,
					Fairshare:      map[string]float64{"cpu": 600, "memory": 256},
				},
			},
		},
	}

	m, _ := sessionModel(t, client)
	m, _ = m.update(openMsg{queuePage{pool: "gpu", queueType: nomad.QueueDynamic}})
	m = playOut(m, m.fetch())

	out := plain(m.render())
	r.Contains(out, "Tenant")
	r.Contains(out, "team-alpha")
	r.Contains(out, "65%")
	r.Contains(out, "600 MHz")
	r.Contains(out, "256 MB")
}

func TestQueue_FailedTenantsReadShowsTheError(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		queueWorkloads: map[string][]nomad.QueueWorkload{
			"gpu": {
				{JobID: "job-1", Namespace: "default", Pool: "gpu", QueueType: nomad.QueueDynamic, Status: "placing", Position: 0},
			},
		},
		queueStatusErr: errors.New("tenants read failed"),
		changes:        newChanges(),
	}

	m, _ := sessionModel(t, client)
	m, _ = m.update(openMsg{queuePage{pool: "gpu", queueType: nomad.QueueDynamic}})
	m = playOut(m, m.fetch())

	r.Contains(plain(m.render()), "job-1")
	r.Contains(plain(statusLine(m)), "tenants read failed")
}

func TestQueue_TenantsPanelOmittedWithoutOperatorRead(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		queueWorkloads: map[string][]nomad.QueueWorkload{
			"gpu": {
				{JobID: "job-1", Namespace: "default", Pool: "gpu", QueueType: nomad.QueueDynamic, Status: "placing", Position: 0},
			},
		},
		// queueStatus is nil (no operator:read permission / 403)
	}

	m, _ := sessionModel(t, client)
	m, _ = m.update(openMsg{queuePage{pool: "gpu", queueType: nomad.QueueDynamic}})
	m = playOut(m, m.fetch())

	out := plain(m.render())
	r.NotContains(out, "team-alpha")
	r.NotContains(out, "Share")
	r.Contains(out, "job-1")
	r.Contains(out, "placing")
}

func TestQueue_UnsupportedOnOlderClusters(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		queueErr: nomad.ErrQueueUnsupported,
	}

	m, _ := sessionModel(t, client)
	m = typeCommand(m, "queue")

	out := plain(m.render())
	r.Contains(out, "The batch job queue needs Nomad 2.1.")
}

// The pool "default" is asked only to tell a cluster without queues from one
// where no pool has a queue.
func TestQueue_AsksTheDefaultPoolOnlyWhenNoPoolHasAQueue(t *testing.T) {
	for name, tc := range map[string]struct {
		pools []nomad.NodePool
		asked []string
	}{
		"a pool with a queue":  {[]nomad.NodePool{{Name: "batch", Queue: nomad.QueueFifo}, {Name: "default"}}, []string{"batch"}},
		"no pool with a queue": {[]nomad.NodePool{{Name: "default"}}, []string{"default"}},
	} {
		t.Run(name, func(t *testing.T) {
			client := &fakeClient{nodePools: tc.pools}

			fetchAllQueues(client)()

			require.Equal(t, tc.asked, client.queuePools)
		})
	}
}

func TestQueue_EnterOpensJobAllocations(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		nodePools: []nomad.NodePool{{Name: "batch", Queue: nomad.QueueFifo}},
		queueWorkloads: map[string][]nomad.QueueWorkload{
			"batch": {
				{JobID: "batch-run", Namespace: "production", Pool: "batch", Status: "queued", Position: 0},
			},
		},
		allocs: []nomad.Alloc{
			{ID: "alloc-1", JobID: "batch-run", Namespace: "production", Status: "pending"},
		},
	}

	m, _ := sessionModel(t, client)
	m = typeCommand(m, "queue")

	m, cmd := m.update(enter())
	m = playOut(m, cmd)

	r.IsType(allocationsPage{}, m.screen.page)
	r.Contains(plain(m.render()), "Allocations (Job: batch-run)")
}

func TestQueue_KeepsTheReadIndexOfThePoolsAndTheQueue(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		nodePools: []nomad.NodePool{{Name: "batch-1", Queue: nomad.QueueFifo}},
		queueWorkloads: map[string][]nomad.QueueWorkload{
			"batch-1": {{JobID: "job-b", Namespace: "prod", Pool: "batch-1", QueueType: nomad.QueueFifo, Status: "queued"}},
		},
		readIndex: map[string]uint64{nomad.TopicNodePool: 9, nomad.TopicJob: 4},
	}

	m, _ := sessionModel(t, client)
	m = typeCommand(m, "queue")

	r.Contains(plain(m.render()), "job-b")
	r.Equal(map[string]uint64{nomad.TopicNodePool: 9, nomad.TopicJob: 4}, m.watch.read)
}

func TestQueue_APoolThatFailedLeavesNoReadIndexOfTheJobs(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		changes:   newChanges(),
		nodePools: []nomad.NodePool{{Name: "batch-1", Queue: nomad.QueueFifo}, {Name: "batch-2", Queue: nomad.QueueFifo}},
		queueWorkloads: map[string][]nomad.QueueWorkload{
			"batch-1": {{JobID: "job-b", Namespace: "prod", Pool: "batch-1", QueueType: nomad.QueueFifo, Status: "queued"}},
		},
		queueErrs: map[string]error{"batch-2": errors.New("timeout")},
		readIndex: map[string]uint64{nomad.TopicNodePool: 9, nomad.TopicJob: 4},
	}

	m, _ := sessionModel(t, client)
	m = typeCommand(m, "queue")

	r.Contains(plain(m.render()), "job-b")
	r.NotContains(m.watch.read, nomad.TopicJob)

	// The queue of batch-2 is not shown: a change of a job reads it again.
	m, _ = m.update(changeMsg{id: m.watch.id, change: nomad.Change{Topic: nomad.TopicJob, Index: 4}})
	r.True(m.watch.settling)
}

func TestQueue_KeepsTheReadIndexOfTheQueueOfAPool(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		queueWorkloads: map[string][]nomad.QueueWorkload{
			"batch-1": {{JobID: "job-b", Namespace: "prod", Pool: "batch-1", QueueType: nomad.QueueFifo, Status: "queued"}},
		},
		readIndex: map[string]uint64{nomad.TopicJob: 4},
	}

	m, _ := sessionModel(t, client)
	m, _ = m.update(openMsg{queuePage{pool: "batch-1", queueType: nomad.QueueFifo}})
	m = playOut(m, m.fetch())

	r.Contains(plain(m.render()), "job-b")
	r.Equal(map[string]uint64{nomad.TopicJob: 4}, m.watch.read)
}
