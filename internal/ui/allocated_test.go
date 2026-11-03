package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// allocatedCluster is a client that gives schedulable to jobs, with the
// allocations that count against it and those that do not.
func allocatedCluster(schedulable nomad.Resources) *fakeClient {
	claim := func(status, desired string, cpu, memory, disk int) nomad.Alloc {
		alloc := clientAllocs()[0]
		alloc.Status, alloc.DesiredStatus = status, desired
		alloc.Claimed = nomad.Resources{CPU: cpu, MemoryMB: memory, DiskMB: disk}

		return alloc
	}

	nodes := busyClient()
	nodes[0].Schedulable = schedulable

	return &fakeClient{
		nodes: nodes,
		nodeAllocs: []nomad.Alloc{
			claim("running", "run", 1000, 2048, 300),
			claim("pending", "run", 1000, 2048, 300),
			claim("complete", "run", 2000, 4096, 1000),
			claim("running", "stop", 500, 1024, 100),
		},
		use: map[string]nomad.ResourceUse{"node-1": {}},
	}
}

func allocatedClient(t *testing.T, schedulable nomad.Resources) Model {
	t.Helper()

	m, _ := nodeModelOf(allocatedCluster(schedulable))
	m, cmd := m.update(enter())

	return drain(m, cmd)
}

func TestClient_ShowsWhatItsAllocationsClaimOfWhatItGivesToJobs(t *testing.T) {
	m := allocatedClient(t, nomad.Resources{CPU: 4000, MemoryMB: 8192, DiskMB: 40960})

	require.Contains(t, bodyOf(m), "Allocated CPU 50%, memory 50%, disk 1%")
}

func TestClient_UnitsShowsTheAllocatedNumbers(t *testing.T) {
	r := require.New(t)

	m := allocatedClient(t, nomad.Resources{CPU: 4000, MemoryMB: 8192, DiskMB: 40960})

	m, _ = m.update(key('u'))
	r.Contains(bodyOf(m), "Allocated CPU 2.00 / 4.00 GHz, memory 4.00 / 8.00 GiB, disk 0.59 / 40.00 GiB")

	m, _ = m.update(key('u'))
	r.Contains(bodyOf(m), "Allocated CPU 50%, memory 50%, disk 1%")
}

func TestClient_DoesNotCountClaimsItHasNotRead(t *testing.T) {
	r := require.New(t)

	client := allocatedCluster(nomad.Resources{CPU: 4000, MemoryMB: 8192, DiskMB: 40960})

	m, _ := nodeModelOf(client)
	m, _ = m.update(enter())
	r.IsType(clientPage{}, m.screen.page)

	r.Contains(bodyOf(m), "Allocated n/a")
	r.NotContains(bodyOf(m), "Allocated CPU")

	client.allocsErr = errTest
	m = drain(m, m.fetch())

	r.Contains(bodyOf(m), "Allocated n/a")
	r.NotContains(bodyOf(m), "Allocated CPU")
}

func TestClient_ShowsNothingOfWhatTheClusterDoesNotSend(t *testing.T) {
	r := require.New(t)

	m := allocatedClient(t, nomad.Resources{})
	r.NotContains(bodyOf(m), "Allocated")

	m = allocatedClient(t, nomad.Resources{CPU: 4000, MemoryMB: 8192})
	r.Contains(bodyOf(m), "Allocated CPU 50%, memory 50%")
	r.NotContains(bodyOf(m), "disk")
}

func TestClient_ShowsNothingWhileItsAllocationsAreUnreadWhenTheClusterSendsNoCapacity(t *testing.T) {
	r := require.New(t)

	client := allocatedCluster(nomad.Resources{})

	m, _ := nodeModelOf(client)
	m, _ = m.update(enter())
	r.IsType(clientPage{}, m.screen.page)
	r.NotContains(bodyOf(m), "Allocated")

	client.allocsErr = errTest
	m = drain(m, m.fetch())
	r.NotContains(bodyOf(m), "Allocated")
}

func TestClient_CutsTheAllocatedNumbersAtTheEndOn80Columns(t *testing.T) {
	r := require.New(t)

	client := allocatedCluster(nomad.Resources{CPU: 15600, MemoryMB: 31 * 1024, DiskMB: 450 * 1024})
	for i := range client.nodeAllocs {
		client.nodeAllocs[i].Claimed = nomad.Resources{CPU: 6000, MemoryMB: 12 * 1024, DiskMB: 146 * 1024}
	}

	m, _ := nodeModelOf(client)
	m, _ = m.update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m, cmd := m.update(enter())
	m = drain(m, cmd)
	m, _ = m.update(key('u'))

	body := bodyOf(m)
	r.Contains(body, "Allocated CPU 12.00 / 15.60 GHz, memory 24.00 / 31.00 GiB, disk 292.00 / 4…")
	r.NotContains(body, "450.00 GiB")
}
