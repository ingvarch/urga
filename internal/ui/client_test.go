package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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

// reading is what the machine of the open screen answers.
func reading(cpu int) hostUseMsg {
	return hostUseMsg{nodeID: "node-1", use: nomad.ResourceUse{CPUPercent: cpu}}
}

// clientScreen drills into the one client of the cluster.
func clientScreen(t *testing.T) (Model, *fakeClient) {
	t.Helper()

	client := &fakeClient{
		nodes:      busyClient(),
		nodeAllocs: clientAllocs(),
		use:        map[string]nomad.ResourceUse{"node-1": {}},
	}

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

	m, _ = m.update(hostUseMsg{nodeID: "node-1", use: nomad.ResourceUse{
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
	m, _ = m.update(reading(29))

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

	before := len(m.hostTrail)

	for _, at := range []int{10, 20, 30} {
		m, _ = m.update(reading(at))
	}

	// Every reading is kept, the newest last: that is what the chart draws
	// from left to right.
	r.Len(m.hostTrail, before+3)
	r.Equal(30, m.hostTrail[len(m.hostTrail)-1].CPUPercent)
}

func TestClient_AReadingThatFailsKeepsTheChart(t *testing.T) {
	r := require.New(t)

	m, _ := clientScreen(t)

	m, _ = m.update(reading(10))
	kept := len(m.hostTrail)

	m, _ = m.update(hostUseMsg{nodeID: "node-1", err: errTest})

	// A machine that does not answer says so, and what it said before stays
	// on the chart.
	r.Len(m.hostTrail, kept)
	r.Contains(plain(m.render()), "no answer")
}

func TestClient_AnotherClientStartsItsOwnChart(t *testing.T) {
	r := require.New(t)

	m, _ := clientScreen(t)

	m, _ = m.update(reading(10))
	r.Contains(m.hostTrail, nomad.ResourceUse{CPUPercent: 10})

	m, _ = m.update(escape())
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	// The readings belong to the machine they were taken on.
	r.NotContains(m.hostTrail, nomad.ResourceUse{CPUPercent: 10})
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

func TestClient_TheChartBelongsToTheClientScreenOnly(t *testing.T) {
	r := require.New(t)

	m, _ := clientScreen(t)
	m, _ = m.update(reading(29))

	r.Contains(plain(m.render()), "Status")

	// A screen opened from the client is about one thing of its own; what
	// the machine is doing stays behind on the screen it belongs to.
	next, cmd := m.update(key('a'))
	next = drain(next, cmd)

	out := plain(next.render())
	r.NotContains(out, "Datacenter")
	r.NotContains(out, "100%")
}

func TestClient_AReadingOfTheMachineYouLeftIsDropped(t *testing.T) {
	r := require.New(t)

	m, _ := clientScreen(t)

	before := len(m.hostTrail)

	m, _ = m.update(hostUseMsg{nodeID: "node-9", use: nomad.ResourceUse{CPUPercent: 99}})

	// A reading that was asked of another machine says nothing about this
	// one, on the chart or in the error line.
	r.Len(m.hostTrail, before)

	m, _ = m.update(hostUseMsg{nodeID: "node-9", err: errTest})
	r.Nil(m.err)
}

func TestClient_TheMachineInThePanelKeepsUp(t *testing.T) {
	r := require.New(t)

	m, _ := clientScreen(t)
	r.Contains(plain(m.render()), "ready")

	m, _ = m.update(hostMsg(nomad.Node{ID: "node-1", Name: "nomad-server-01", Status: "down", Drain: true}))

	// The machine is polled with everything else on the screen: a client
	// that goes down must not still read as ready.
	out := plain(m.render())
	r.Contains(out, "down")
	r.Contains(out, "draining")

	// And the answer of another machine is not this one.
	m, _ = m.update(hostMsg(nomad.Node{ID: "node-9", Name: "somewhere-else", Status: "ready"}))
	r.NotContains(plain(m.render()), "somewhere-else")
}

func TestClient_ANarrowScreenKeepsTheMachineAndDropsTheCharts(t *testing.T) {
	r := require.New(t)

	m, _ := clientScreen(t)
	m, _ = m.update(reading(29))

	m, _ = m.update(tea.WindowSizeMsg{Width: 20, Height: 40})

	out := plain(m.render())

	// Half of a narrow screen has no room for a chart with its scale, so
	// the machine keeps the room and the charts give it up rather than
	// spill out of the box.
	r.Contains(out, "ready")
	r.NotContains(out, "100%")

	for _, line := range strings.Split(out, "\n") {
		r.LessOrEqual(ansi.StringWidth(line), 20, line)
	}
}
