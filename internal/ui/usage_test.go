package ui

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestUsage_ShownOnTheAllocations(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		jobs:   twoJobs(),
		allocs: twoAllocs(),
		use: map[string]nomad.ResourceUse{
			"af1f37df-7b19-6b1c-da67-5e8f482b5a15": {CPUTicks: 125, CPUTicksAllowed: 500, CPUPercent: 25, MemoryMB: 128, MemoryMBAllowed: 256, MemoryPercent: 50},
		},
	}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(enter())
	m, _ = m.update(allocsMsg(twoAllocs()))

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

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(enter())
	m, _ = m.update(allocsMsg(twoAllocs()))

	drain(m, m.fetchUsage())

	// Two rows on the screen, two readings asked for. A cluster is not walked
	// allocation by allocation for rows nobody is looking at.
	r.Equal(2, client.usageCalls)
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
