package ui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// allocationsOf walks down to the allocation list of the first job, which is
// where the readings show up.
func allocationsOf(t *testing.T, client *fakeClient) Model {
	t.Helper()

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(client.jobs))
	m, _ = m.update(enter())
	m, _ = m.update(allocsMsg(client.allocs))

	return m
}

func TestUsage_ShownOnTheAllocations(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		jobs:   twoJobs(),
		allocs: twoAllocs(),
		use: map[string]nomad.ResourceUse{
			"af1f37df-7b19-6b1c-da67-5e8f482b5a15": {CPUTicks: 125, CPUTicksAllowed: 500, CPUPercent: 25, MemoryMB: 128, MemoryMBAllowed: 256, MemoryPercent: 50},
		},
	}

	m := allocationsOf(t, client)

	// Before the cluster answers, the columns are there and empty.
	out := plain(m.render())
	r.Contains(out, "CPU")
	r.Contains(out, "MEM")

	m = drain(m, m.fetchUsage())

	out = plain(m.render())
	r.Contains(out, "25%")
	r.Contains(out, "50%")

	// The allocation that said nothing shows nothing rather than zero.
	r.Contains(out, "-")
}

func TestUsage_AskedForWhatIsOnTheScreen(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}

	m := allocationsOf(t, client)

	drain(m, m.fetchUsage())

	// Of the two rows on the screen one runs, and it is the only one asked
	// about. A cluster is not walked allocation by allocation for rows
	// nobody is looking at.
	r.Equal(1, client.usageCalls)
}

func TestUsage_NotAskedForOnOtherScreens(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs()}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	r.Nil(m.fetchUsage())
}

func TestUsage_ShownOnTheNodes(t *testing.T) {
	r := require.New(t)

	nodes := []nomad.Node{{ID: "node-1", Name: "server-01", Status: "ready", Eligibility: "eligible"}}

	client := &fakeClient{
		nodes: nodes,
		use:   map[string]nomad.ResourceUse{"node-1": {CPUPercent: 40, MemoryMB: 4096, MemoryMBAllowed: 8192, MemoryPercent: 50}},
	}

	m := newTestModel(client)
	m, _ = m.update(key(':'))
	m = typeIn(m, "nodes")
	m, _ = m.update(enter())
	m, _ = m.update(nodesMsg(nodes))

	m = drain(m, m.fetchUsage())

	out := plain(m.render())
	r.Contains(out, "40%")
	r.Contains(out, "50%")
}

func TestUsage_Cells(t *testing.T) {
	r := require.New(t)

	// A share of what was asked for.
	r.Equal("25%", cpuCell(nomad.ResourceUse{CPUTicks: 125, CPUTicksAllowed: 500, CPUPercent: 25}, true))
	r.Equal("50%", memoryCell(nomad.ResourceUse{MemoryMB: 128, MemoryMBAllowed: 256, MemoryPercent: 50}, true))

	// Nothing was asked for, so the number itself is what there is to say.
	r.Equal("125", cpuCell(nomad.ResourceUse{CPUTicks: 125}, true))
	r.Equal("128M", memoryCell(nomad.ResourceUse{MemoryMB: 128}, true))

	// The cluster has not answered.
	r.Equal("-", cpuCell(nomad.ResourceUse{}, false))
	r.Equal("-", memoryCell(nomad.ResourceUse{}, false))
}

func TestUsage_OnlyRunningAllocationsAreAsked(t *testing.T) {
	r := require.New(t)

	allocs := []nomad.Alloc{
		{ID: "running-one", Namespace: "production", JobID: "web", TaskGroup: "frontend", Status: "running"},
		{ID: "finished-one", Namespace: "production", JobID: "web", TaskGroup: "frontend", Status: "complete"},
	}

	client := &fakeClient{jobs: twoJobs(), allocs: allocs}

	m := allocationsOf(t, client)

	drain(m, m.fetchUsage())

	// An allocation that has ended reports nothing, so it is not asked. The
	// answer would be an error, and the row would say nothing either way.
	r.Equal(1, client.usageCalls)
}

// allocationsAnswered opens the allocations of the first job and hands the
// list over, with what the answer asked for next.
func allocationsAnswered(t *testing.T, client *fakeClient) (Model, tea.Cmd) {
	t.Helper()

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(client.jobs))
	m, _ = m.update(enter())

	return m.update(allocsMsg(client.allocs))
}

func TestUsage_EveryScreenStartsItsOwnReadings(t *testing.T) {
	r := require.New(t)

	nodes := []nomad.Node{{ID: "node-1", Name: "server-01", Status: "ready", Eligibility: "eligible"}}

	client := &fakeClient{
		jobs:   twoJobs(),
		allocs: twoAllocs(),
		nodes:  nodes,
		use: map[string]nomad.ResourceUse{
			"af1f37df-7b19-6b1c-da67-5e8f482b5a15": {CPUTicks: 125, CPUTicksAllowed: 500, CPUPercent: 25},
			"node-1":                               {CPUPercent: 40, MemoryMB: 4096, MemoryMBAllowed: 8192, MemoryPercent: 50},
		},
	}

	m, cmd := allocationsAnswered(t, client)
	m = drain(m, cmd)
	r.Contains(plain(m.render()), "25%")

	// The jobs have nothing to read; the clients do.
	m, _ = m.update(escape())
	m, _ = m.update(jobsMsg(client.jobs))

	m, _ = m.update(key(':'))
	m = typeIn(m, "nodes")
	m, _ = m.update(enter())

	m, cmd = m.update(nodesMsg(nodes))
	m = drain(m, cmd)

	// What was read of the allocations does not keep the clients from being
	// read.
	out := plain(m.render())
	r.Contains(out, "40%")
	r.Contains(out, "50%")
}

func TestUsage_FailedReadingsStartNoSecondTimer(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), usageErr: errors.New("No path to node")}

	m, cmd := allocationsAnswered(t, client)
	m = drain(m, cmd)
	r.Equal(1, client.usageCalls)

	// Every poll answers the list again. Readings that all failed still have
	// their timer on its way, and no answer starts another next to it.
	for range 3 {
		m, cmd = m.update(allocsMsg(client.allocs))
		m = drain(m, cmd)
	}

	r.Equal(1, client.usageCalls)
}

func TestUsage_ReadingsOnTheirWayAreNotAskedAgain(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}

	// The first reading is still out when the list is answered again.
	m, _ := allocationsAnswered(t, client)

	m, cmd := m.update(allocsMsg(client.allocs))
	drain(m, cmd)

	r.Zero(client.usageCalls)
}

func TestUsage_TheReasonStaysWithItsScreen(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), usageErr: errors.New("No path to node")}

	m, cmd := allocationsAnswered(t, client)
	m = drain(m, cmd)
	r.Contains(plain(m.render()), "no readings")

	m, _ = m.update(escape())
	m, _ = m.update(jobsMsg(client.jobs))

	// The jobs were never read, so nothing on them is missing.
	r.NotContains(plain(m.render()), "no readings")
}

func TestUsage_AReadingOfAScreenThatWasLeftIsDropped(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), usageErr: errors.New("No path to node")}

	m := allocationsOf(t, client)
	late := m.fetchUsage()

	m, _ = m.update(escape())
	m, _ = m.update(jobsMsg(client.jobs))

	// The answer of the allocations comes back after they were left.
	m = drain(m, late)

	r.NotContains(plain(m.render()), "no readings")
}

func TestUsage_SaysWhyThereAreNoReadings(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), usageErr: errors.New("Unexpected response code: 500 (rpc error: No path to node)")}

	m := allocationsOf(t, client)

	m = drain(m, m.fetchUsage())

	// A dash in the column is not an explanation. What the cluster said is
	// on the screen.
	out := plain(m.render())
	r.Contains(out, "no readings")
	r.Contains(out, "No path to node")
}

func TestUsage_OnlyWhatTheFilterLeavesIsAsked(t *testing.T) {
	r := require.New(t)

	allocs := twoAllocs()
	allocs[1].Status = "running"

	client := &fakeClient{jobs: twoJobs(), allocs: allocs}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(client.jobs))
	m, _ = m.update(enter())
	m, _ = m.update(allocsMsg(allocs))

	// Both run, and the filter leaves one of them on the screen.
	m, _ = m.update(key('/'))
	m = typeIn(m, "backend")
	m, _ = m.update(enter())

	drain(m, m.fetchUsage())

	r.Equal(1, client.usageCalls)
}
