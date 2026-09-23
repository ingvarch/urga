package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// errTest is what a machine that does not answer says.
var errTest = errors.New("no answer from the client")

func busyClient() []nomad.Node {
	return []nomad.Node{{
		ID:          "node-1",
		Name:        "nomad-server-01",
		Datacenter:  "dc1",
		NodePool:    "default",
		Version:     "1.11.1",
		Status:      "ready",
		Eligibility: "eligible",
		Address:     "10.0.0.2",
		CPUShares:   4000,
		MemoryMB:    3820,
	}}
}

func clientAllocs() []nomad.Alloc {
	return []nomad.Alloc{{
		ID:            "af1f37df-4528-6395-3a51-1ffcbf3c2ba4",
		Name:          "pelmeni_buh_bot.pelmenis[0]",
		Namespace:     "production",
		JobID:         "pelmeni_buh_bot",
		TaskGroup:     "pelmenis",
		NodeID:        "node-1",
		NodeName:      "nomad-server-01",
		Status:        "running",
		DesiredStatus: "run",
		Tasks:         []nomad.Task{{Name: "bot", State: "running"}},
	}}
}

// clientScreen drills into the one client of the cluster.
func clientScreen(t *testing.T) (Model, *fakeClient) {
	t.Helper()

	client := &fakeClient{nodes: busyClient(), nodeAllocs: clientAllocs()}

	m, _ := nodeModelOf(client)
	m, cmd := m.update(enter())

	return drain(m, cmd), client
}

func TestClient_OpensWhatItRuns(t *testing.T) {
	r := require.New(t)

	m, client := clientScreen(t)

	// The machine is asked for its own work.
	r.Equal("node-1", client.askedNodeID)

	out := plain(m.render())
	r.Contains(out, "Client nomad-server-01 [1]")
	r.Contains(out, "pelmeni_buh_bot")
	r.Contains(out, "running")
}

func TestClient_SaysWhatKindOfMachineItIs(t *testing.T) {
	r := require.New(t)

	m, _ := clientScreen(t)

	out := plain(m.render())
	r.Contains(out, "ready")
	r.Contains(out, "10.0.0.2")
	r.Contains(out, "dc1")
	r.Contains(out, "default")
}

func TestClient_DrawsWhatTheHostIsDoing(t *testing.T) {
	r := require.New(t)

	m, _ := clientScreen(t)

	m, _ = m.update(hostUseMsg{use: nomad.ResourceUse{
		CPUPercent: 29, CPUTicks: 1165,
		MemoryPercent: 22, MemoryMB: 857, MemoryMBAllowed: 3820,
	}})

	out := plain(m.render())

	// What the machine takes, next to what it has, and a chart of the
	// readings taken since the screen was opened.
	r.Contains(out, "CPU")
	r.Contains(out, "29%")
	r.Contains(out, "1165 / 4000 MHz")

	r.Contains(out, "MEM")
	r.Contains(out, "22%")
	r.Contains(out, "857 / 3820 MiB")

	r.True(strings.ContainsAny(out, "▁▂▃▄▅▆▇█"), out)
}

func TestClient_TheChartIsDroppedOnAShortScreen(t *testing.T) {
	r := require.New(t)

	m, _ := clientScreen(t)
	m, _ = m.update(hostUseMsg{use: nomad.ResourceUse{CPUPercent: 29}})

	m, _ = m.update(tea.WindowSizeMsg{Width: 120, Height: 16})
	out := plain(m.render())

	// On a screen with no room for both, the allocations win and the
	// machine still says what it is.
	r.Contains(out, "pelmeni_buh_bot")
	r.Contains(out, "ready")
	r.NotContains(out, "100%")
}

func TestClient_TheChartKeepsTheReadings(t *testing.T) {
	r := require.New(t)

	m, _ := clientScreen(t)

	for _, at := range []int{10, 20, 30} {
		m, _ = m.update(hostUseMsg{use: nomad.ResourceUse{CPUPercent: at}})
	}

	r.Len(m.hostTrail, 3)
	r.Equal(30, m.hostTrail[2].CPUPercent)
}

func TestClient_AReadingThatFailsKeepsTheChart(t *testing.T) {
	r := require.New(t)

	m, _ := clientScreen(t)

	m, _ = m.update(hostUseMsg{use: nomad.ResourceUse{CPUPercent: 10}})
	m, _ = m.update(hostUseMsg{err: errTest})

	// A machine that does not answer says so, and what it said before stays
	// on the chart.
	r.Len(m.hostTrail, 1)
	r.Contains(plain(m.render()), "no answer")
}

func TestClient_AnotherClientStartsItsOwnChart(t *testing.T) {
	r := require.New(t)

	m, _ := clientScreen(t)

	m, _ = m.update(hostUseMsg{use: nomad.ResourceUse{CPUPercent: 10}})
	r.Len(m.hostTrail, 1)

	m, _ = m.update(escape())
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	// The readings belong to the machine they were taken on.
	r.Empty(m.hostTrail)
}

func TestClient_ReadsAnAllocationInItsOwnNamespace(t *testing.T) {
	r := require.New(t)

	allocs := clientAllocs()
	allocs[0].Namespace = "staging"

	client := &fakeClient{nodes: busyClient(), nodeAllocs: allocs}

	m, _ := nodeModelOf(client)
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	m.fetchUsage()()

	// A client runs the work of every namespace, and a reading is asked for
	// in the namespace the allocation belongs to.
	r.Equal("staging", client.usageNamespace)
}

func TestClient_OpensTheTasksOfAnAllocation(t *testing.T) {
	r := require.New(t)

	m, _ := clientScreen(t)

	m, _ = m.update(enter())

	// The allocations of a client answer the same keys as any other
	// allocation list.
	r.Equal(screenTasks, m.screen.kind)
	r.Contains(plain(m.render()), "bot")
}
