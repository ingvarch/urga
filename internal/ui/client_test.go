package ui

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// errTest is the error of a node that does not answer.
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

// trailOf are the readings the chart of the open client holds.
func trailOf(m Model) []nomad.ResourceUse {
	return m.screen.page.(clientPage).host.trail
}

// reading is what the machine of the open screen answers.
func reading(cpu int) hostUseMsg {
	return hostUseMsg{nodeID: "node-1", use: nomad.ResourceUse{CPUPercent: cpu}}
}

// onClient drills into the one client of the cluster.
func onClient(t *testing.T) (Model, *fakeClient) {
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

	m, client := onClient(t)

	// The node is asked for the allocations it runs.
	r.Equal("node-1", client.askedNodeID)

	out := plain(m.render())
	r.Contains(out, "Client nomad-server-01 [1]")
	r.Contains(out, "pelmeni_buh_bot")
	r.Contains(out, "running")
}

func TestClient_SaysWhatKindOfMachineItIs(t *testing.T) {
	r := require.New(t)

	m, _ := onClient(t)

	out := plain(m.render())
	r.Contains(out, "ready")
	r.Contains(out, "10.0.0.2")
	r.Contains(out, "dc1")
	r.Contains(out, "default")
}

func TestClient_DrawsWhatTheHostIsDoing(t *testing.T) {
	r := require.New(t)

	m, _ := onClient(t)

	// One reading is a mark, and no line turns yet.
	m, _ = m.update(reading(0))
	r.False(strings.ContainsAny(bodyOf(m), chartTurnRunes), bodyOf(m))

	m, _ = m.update(hostUseMsg{nodeID: "node-1", use: nomad.ResourceUse{
		CPUPercent: 29, CPUTicks: 1165,
		MemoryPercent: 22, MemoryMB: 857, MemoryMBAllowed: 3820,
	}})

	out := plain(m.render())

	// What the node uses next to what it has, and a chart of the readings
	// taken since the screen was opened.
	r.Contains(out, "CPU")
	r.Contains(out, "29%")
	r.Contains(out, "1.17 / 4.00 GHz")

	r.Contains(out, "MEM")
	r.Contains(out, "22%")
	r.Contains(out, "0.84 / 3.73 GiB")

	// The line climbs from the first reading to the second.
	r.True(strings.ContainsAny(bodyOf(m), chartTurnRunes), bodyOf(m))
}

// bodyOf is what the box of the screen holds, without its frame.
func bodyOf(m Model) string {
	_, content := m.body(m.width - 2*screenPadX - 2)

	return plain(content)
}

func TestClient_UnitsTogglesBetweenPercentsAndNumbers(t *testing.T) {
	r := require.New(t)

	m, _ := onClient(t)
	m, _ = m.update(hostUseMsg{nodeID: "node-1", use: nomad.ResourceUse{
		CPUPercent: 29, CPUTicks: 1165,
		MemoryPercent: 22, MemoryMB: 857, MemoryMBAllowed: 3820,
	}})

	// The charts read percents first, against the numbers behind them.
	r.Contains(plain(m.render()), "1.17 / 4.00 GHz")
	r.Contains(plain(m.render()), "0.84 / 3.73 GiB")
	r.Contains(keyNames(m), "u Units")

	m, _ = m.update(key('u'))
	out := plain(m.render())

	// One key shows the numbers themselves: gigahertz, gibibytes, and the
	// percents behind them.
	r.Contains(out, "1.17 GHz")
	r.Contains(out, "0.84 GiB")
	r.Contains(out, "2.00 GHz")
	r.Contains(out, "GiB")
	r.Contains(out, "29%")

	// The same key reads percents again.
	m, _ = m.update(key('u'))
	out = plain(m.render())

	r.Contains(out, "1.17 / 4.00 GHz")
	r.NotContains(out, "1.17 GHz")
}

func TestClient_AnIdleMachineDoesNotReadAsDoingNothing(t *testing.T) {
	r := require.New(t)

	m, _ := onClient(t)
	m, _ = m.update(hostUseMsg{nodeID: "node-1", use: nomad.ResourceUse{
		CPUPercent: 1, CPUTicks: 40,
		MemoryPercent: 1, MemoryMB: 40, MemoryMBAllowed: 3820,
	}})

	// The little a machine uses still reads as something, in both units.
	out := plain(m.render())
	r.Contains(out, "0.04 / 4.00 GHz")
	r.Contains(out, "0.04 / 3.73 GiB")

	m, _ = m.update(key('u'))
	out = plain(m.render())
	r.Contains(out, "0.04 GHz")
	r.Contains(out, "0.04 GiB")
}

func TestClient_TheChartsHaveOneLook(t *testing.T) {
	r := require.New(t)

	m, _ := onClient(t)
	m, _ = m.update(hostUseMsg{nodeID: "node-1", use: nomad.ResourceUse{
		CPUPercent: 50, CPUTicks: 2000,
		MemoryPercent: 50, MemoryMB: 1910, MemoryMBAllowed: 3820,
	}})

	shaded := m.render()
	r.Contains(shaded, paintOf(sgrBackground, colorPanel))

	// How the charts are filled is not a choice of the screen: the keys
	// that are not its own change nothing it draws.
	for _, press := range []rune{'f', 's'} {
		pressed, _ := m.update(key(press))
		r.Equal(shaded, pressed.render(), string(press))
	}
}

func TestClient_UnitsFallsBackToPercentsWithoutTotals(t *testing.T) {
	r := require.New(t)

	// The machine reports no totals to read the numbers against.
	client := &fakeClient{
		nodes:      []nomad.Node{{ID: "node-1", Name: "nomad-server-01", Status: "ready", Eligibility: "eligible"}},
		nodeAllocs: clientAllocs(),
	}

	m, _ := nodeModelOf(client)
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	m, _ = m.update(hostUseMsg{nodeID: "node-1", use: nomad.ResourceUse{CPUPercent: 29, MemoryPercent: 22}})

	m, _ = m.update(key('u'))
	out := plain(m.render())

	// Without a limit the numbers read against nothing: percents do.
	r.NotContains(out, "MHz")
	r.NotContains(out, "MiB")
	r.Contains(out, "50%")
}

func TestClient_SmallNumbersReadWithoutScaling(t *testing.T) {
	r := require.New(t)

	// Below a thousand megahertz and a gibibyte, numbers read as they are.
	client := &fakeClient{
		nodes:      []nomad.Node{{ID: "node-1", Name: "nomad-server-01", Status: "ready", Eligibility: "eligible", CPUShares: 500, MemoryMB: 512}},
		nodeAllocs: clientAllocs(),
	}

	m, _ := nodeModelOf(client)
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	m, _ = m.update(hostUseMsg{nodeID: "node-1", use: nomad.ResourceUse{
		CPUPercent: 80, CPUTicks: 400,
		MemoryPercent: 50, MemoryMB: 256, MemoryMBAllowed: 512,
	}})

	out := plain(m.render())
	r.Contains(out, "400 / 500 MHz")
	r.Contains(out, "256 / 512 MiB")

	m, _ = m.update(key('u'))
	out = plain(m.render())

	r.Contains(out, "400 MHz")
	r.Contains(out, "256 MiB")
	r.NotContains(out, "GHz")
	r.NotContains(out, "GiB")
}

func TestClient_ChartsShareOneScaleWidth(t *testing.T) {
	r := require.New(t)

	// Gigahertz take more room than the mebibytes of this machine.
	client := &fakeClient{
		nodes:      []nomad.Node{{ID: "node-1", Name: "nomad-server-01", Status: "ready", Eligibility: "eligible", CPUShares: 20000, MemoryMB: 512}},
		nodeAllocs: clientAllocs(),
	}

	m, _ := nodeModelOf(client)
	m, cmd := m.update(enter())
	m = drain(m, cmd)
	m, _ = m.update(hostUseMsg{nodeID: "node-1", use: nomad.ResourceUse{
		CPUPercent: 6, CPUTicks: 1165, MemoryPercent: 50, MemoryMB: 256, MemoryMBAllowed: 512,
	}})
	m, _ = m.update(key('u'))

	lines := strings.Split(plain(m.render()), "\n")
	head := slices.IndexFunc(lines, func(s string) bool { return strings.Contains(s, "1.17 GHz") })
	r.Positive(head)

	// The first plot row carries no labels: its first bar per half is the
	// axis both scales stand on. Cells, not bytes: the bar takes three of
	// those. The table draws the panel without its margin space.
	half := chartWidth(m.width - 2*screenPadX - 2)
	plot := strings.TrimSuffix(strings.TrimPrefix(lines[head+1], " │ "), " │")
	cells := []rune(plot)
	cpuAxis := slices.Index(cells[:half], '│')
	memAxis := slices.Index(cells[half+columnGap:], '│')
	r.Positive(cpuAxis)
	r.Positive(memAxis)

	// Both axes stand the same columns into their halves: the charts
	// shrink together instead of one shifting sideways.
	r.Equal(cpuAxis, memAxis)
	r.Equal(len("20.00 GHz"), cpuAxis)
}

func TestClient_TheChartIsDroppedOnAShortScreen(t *testing.T) {
	r := require.New(t)

	m, _ := onClient(t)
	m, _ = m.update(reading(29))

	// A box of nine rows, whatever the header above it takes.
	m, _ = m.update(tea.WindowSizeMsg{Width: 120, Height: headerHeight + 11})
	out := plain(m.render())

	// On a screen with no room for both, the chart is dropped for the
	// allocations, and the status of the node still shows.
	r.Contains(out, "pelmeni_buh_bot")
	r.Contains(out, "ready")
	r.NotContains(out, "50%")
}

func TestClient_TheChartKeepsTheReadings(t *testing.T) {
	r := require.New(t)

	m, _ := onClient(t)

	before := len(trailOf(m))

	for _, at := range []int{10, 20, 30} {
		m, _ = m.update(reading(at))
	}

	// Every reading is kept, the newest last: that is what the chart draws
	// from left to right.
	r.Len(trailOf(m), before+3)
	r.Equal(30, trailOf(m)[len(trailOf(m))-1].CPUPercent)
}

func TestClient_AReadingThatFailsKeepsTheChart(t *testing.T) {
	r := require.New(t)

	m, _ := onClient(t)

	m, _ = m.update(reading(10))
	kept := len(trailOf(m))

	m, _ = m.update(hostUseMsg{nodeID: "node-1", err: errTest})

	// When the node does not answer, the error shows, and the readings from
	// before stay on the chart.
	r.Len(trailOf(m), kept)
	r.Contains(plain(m.render()), "no answer")
}

func TestClient_AnotherClientStartsItsOwnChart(t *testing.T) {
	r := require.New(t)

	m, _ := onClient(t)

	m, _ = m.update(reading(10))
	r.Contains(trailOf(m), nomad.ResourceUse{CPUPercent: 10})

	m, _ = m.update(escape())
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	// The readings belong to the machine they were taken on.
	r.NotContains(trailOf(m), nomad.ResourceUse{CPUPercent: 10})
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

	// A client runs allocations of every namespace, and a reading is asked
	// for in the namespace the allocation belongs to.
	r.Equal("staging", client.usageNamespace)
}

func TestClient_OpensTheTasksOfAnAllocation(t *testing.T) {
	r := require.New(t)

	m, _ := onClient(t)

	m, _ = m.update(enter())

	// The allocations of a client answer the same keys as any other
	// allocation list.
	r.IsType(tasksPage{}, m.screen.page)
	r.Contains(plain(m.render()), "bot")
}

func TestClient_TheChartBelongsToTheClientScreenOnly(t *testing.T) {
	r := require.New(t)

	m, _ := onClient(t)
	m, _ = m.update(reading(29))

	r.Contains(plain(m.render()), "Status")

	// A screen opened from the client shows only its own content; the panel
	// of the node stays on the client screen.
	next, cmd := m.update(key('a'))
	next = drain(next, cmd)

	out := plain(next.render())
	r.NotContains(out, "Datacenter")
	r.NotContains(out, "50%")
}

func TestClient_AReadingOfTheMachineYouLeftIsDropped(t *testing.T) {
	r := require.New(t)

	m, _ := onClient(t)

	before := len(trailOf(m))

	m, _ = m.update(hostUseMsg{nodeID: "node-9", use: nomad.ResourceUse{CPUPercent: 99}})

	// A reading asked of another node is dropped: it goes neither on the
	// chart nor on the error line.
	r.Len(trailOf(m), before)

	m, _ = m.update(hostUseMsg{nodeID: "node-9", err: errTest})
	r.NotEqual(flashErr, m.flash.level)
}

func TestClient_TheMachineInThePanelKeepsUp(t *testing.T) {
	r := require.New(t)

	m, _ := onClient(t)
	r.Contains(plain(m.render()), "ready")

	m, _ = m.update(hostMsg(nomad.Node{ID: "node-1", Name: "nomad-server-01", Status: "down", Drain: true}))

	// The machine is polled with everything else on the screen: a client
	// that goes down must not still read as ready.
	out := plain(m.render())
	r.Contains(out, "down")
	r.Contains(out, "draining")

	// An answer about another node is dropped.
	m, _ = m.update(hostMsg(nomad.Node{ID: "node-9", Name: "somewhere-else", Status: "ready", Address: "10.9.9.9"}))

	out = plain(m.render())
	r.NotContains(out, "10.9.9.9")
	r.Contains(out, "draining")
}

func TestClient_ANarrowScreenKeepsTheMachineAndDropsTheCharts(t *testing.T) {
	r := require.New(t)

	m, _ := onClient(t)
	m, _ = m.update(reading(29))

	m, _ = m.update(tea.WindowSizeMsg{Width: 20, Height: 40})

	out := plain(m.render())

	// Half of a narrow screen has no room for a chart with its scale, so
	// the node details keep the room and the charts are dropped instead of
	// spilling out of the box.
	r.Contains(out, "ready")
	r.NotContains(out, "50%")

	for _, line := range strings.Split(out, "\n") {
		r.LessOrEqual(ansi.StringWidth(line), 20, line)
	}
}

// clientOpened is the client screen before its list has been answered.
func clientOpened(t *testing.T, cpu int) (Model, *fakeClient) {
	t.Helper()

	client := &fakeClient{
		nodes:      busyClient(),
		nodeAllocs: clientAllocs(),
		use:        map[string]nomad.ResourceUse{"node-1": {CPUPercent: cpu}},
	}

	m, _ := nodeModelOf(client)
	m, _ = m.update(enter())

	return m, client
}

func TestClient_TheFirstReadingComesWithTheList(t *testing.T) {
	r := require.New(t)

	m, _ := clientOpened(t, 42)

	m, cmd := m.update(allocsMsg(clientAllocs()))
	m = drain(m, cmd)

	// The chart has something to draw as soon as the screen does.
	r.NotEmpty(trailOf(m))
	r.Equal(42, trailOf(m)[len(trailOf(m))-1].CPUPercent)

	// The list is answered on every poll and on every change the cluster
	// reports. None of those answers takes a reading: the chart has a timer
	// of its own, and a second timer next to it would read twice as often.
	before := len(trailOf(m))

	for range 3 {
		m, cmd = m.update(allocsMsg(clientAllocs()))
		m = drain(m, cmd)
	}

	r.Len(trailOf(m), before)
}

func TestClient_APollOfTheListReadsNoHost(t *testing.T) {
	r := require.New(t)

	m, client := onClient(t)
	calls := client.usageCalls

	// A poll comes every thirty seconds while the cluster streams and every
	// two when it does not. Readings taken with it would reach the chart at
	// either pace, while its axis assumes one fixed pace.
	drain(m, m.fetch())

	r.Equal(calls, client.usageCalls)
}

func TestClient_APollOfTheListReadsTheMachine(t *testing.T) {
	r := require.New(t)

	m, client := onClient(t)
	client.nodes[0].Status, client.nodes[0].Drain = "down", true

	// The node is asked for with its allocations, or the panel keeps
	// showing the node as it was when the screen opened.
	m = drain(m, m.fetch())

	out := plain(m.render())
	r.Contains(out, "down")
	r.Contains(out, "draining")
}

func TestClient_EveryReadingSetsTheTimerForTheNext(t *testing.T) {
	r := require.New(t)

	m, _ := onClient(t)

	_, cmd := m.update(reading(10))
	r.NotNil(cmd)

	// A machine that did not answer is asked again all the same, or one
	// timeout stops the chart for as long as the screen is open.
	_, cmd = m.update(hostUseMsg{nodeID: "node-1", err: errTest})
	r.NotNil(cmd)
}

func TestClient_TheTimerTakesAReading(t *testing.T) {
	r := require.New(t)

	m, _ := onClient(t)
	before := len(trailOf(m))

	m, cmd := m.update(pollHostMsg{})
	m = drain(m, cmd)

	r.Len(trailOf(m), before+1)
}

func TestClient_ComingBackReadsAgain(t *testing.T) {
	r := require.New(t)

	m, client := clientOpened(t, 42)
	m, cmd := m.update(allocsMsg(clientAllocs()))
	m = drain(m, cmd)

	m, _ = m.update(enter())
	r.IsType(tasksPage{}, m.screen.page)

	// The timer of the chart stopped when the screen was left. Coming back
	// starts another one, or the chart stops moving under a live list.
	m, _ = m.update(escape())
	r.IsType(clientPage{}, m.screen.page)

	before := len(trailOf(m))

	m, cmd = m.update(allocsMsg(client.nodeAllocs))
	m = drain(m, cmd)

	r.Len(trailOf(m), before+1)
}

// drawnPanel is how many rows the screen draws above its table.
func drawnPanel(m Model) int {
	width := m.width - 2*screenPadX - 2
	_, content := m.body(width)

	return strings.Count(content, "\n") - strings.Count(m.withHint(m.list.table.view(), width), "\n")
}

func TestClient_ThePanelTakesWhatTheBoxHasRoomFor(t *testing.T) {
	r := require.New(t)

	m, _ := onClient(t)
	m, _ = m.update(reading(29))

	// As the box shrinks, the panel goes from tall charts to short ones, to
	// the node alone, and then away. The table gets whatever is left.
	for _, tc := range []struct {
		width, box, panel int
		charts            bool
	}{
		{width: 120, box: 40, panel: 18, charts: true},
		{width: 120, box: 26, panel: 18, charts: true},
		{width: 120, box: 25, panel: 18, charts: true},
		{width: 120, box: 24, panel: 18, charts: true},
		{width: 120, box: 23, panel: 12, charts: true},
		{width: 120, box: 20, panel: 12, charts: true},
		{width: 120, box: 19, panel: 12, charts: true},
		{width: 120, box: 18, panel: 12, charts: true},
		{width: 120, box: 17, panel: 2},
		{width: 120, box: 8, panel: 2},
		{width: 120, box: 7, panel: 0},
		{width: 120, box: 3, panel: 0},
		{width: 20, box: 40, panel: 2},
	} {
		sized, _ := m.update(tea.WindowSizeMsg{Width: tc.width, Height: tc.box + screenPadTop + headerHeight + statusHeight})
		_, content := sized.body(sized.width - 2*screenPadX - 2)
		body := plain(content)

		r.Equal(tc.panel, sized.panelHeight(), "%dx%d", tc.width, tc.box)
		r.Equal(tc.panel, drawnPanel(sized), "%dx%d", tc.width, tc.box)
		r.Equal(max(tc.box-3-tc.panel, 1), sized.list.table.height, "%dx%d", tc.width, tc.box)
		r.Equal(tc.panel > 0, strings.Contains(body, "ready"), "%dx%d", tc.width, tc.box)
		r.Equal(tc.charts, strings.Contains(body, "50%"), "%dx%d", tc.width, tc.box)
	}
}

func TestPanel_TakesTheRowsItDraws(t *testing.T) {
	r := require.New(t)

	client, _ := onClient(t)
	client, _ = client.update(reading(29))

	screens := map[string]Model{
		"client":     client,
		"tasks":      openCanary(t, &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}),
		"deployment": onDeployment(t, &fakeClient{}),
		"variable":   onVariable(t, &fakeClient{}, leaderVariable()),
		"jobs":       newTestModel(&fakeClient{}),
	}

	// The rows kept for a panel are the rows it draws, at every size, and a
	// screen without one keeps none.
	for name, m := range screens {
		for _, width := range []int{20, 60, 120} {
			for height := 1; height <= 60; height++ {
				sized, _ := m.update(tea.WindowSizeMsg{Width: width, Height: height})
				r.Equal(sized.panelHeight(), drawnPanel(sized), "%s at %dx%d", name, width, height)
			}
		}
	}
}

func TestClient_TheChartSaysHowFarBackItReaches(t *testing.T) {
	r := require.New(t)

	m, _ := onClient(t)
	m, _ = m.update(hostUseMsg{nodeID: "node-1", use: nomad.ResourceUse{CPUPercent: 29, CPUTicks: 1165}})
	m, _ = m.update(tea.WindowSizeMsg{Width: 114, Height: 44})

	half := chartWidth(m.width - 2*screenPadX - 2)

	// The left edge is as many readings back as the plot is wide, and the
	// readings come at the pace of their own timer. The plot is what the
	// scale leaves of the chart.
	window := time.Duration(half-len("100%")-1) * hostUseEvery
	r.Equal(4*time.Minute, window)
	r.Contains(bodyOf(m), "4m ago")

	// The numbers take a wider scale, and the plot holds less.
	m, _ = m.update(key('u'))

	window = time.Duration(half-len("4.00 GHz")-1) * hostUseEvery
	r.Equal(3*time.Minute+40*time.Second, window)
	r.Contains(bodyOf(m), "3m ago")
	r.NotContains(bodyOf(m), "4m ago")
}

func TestClient_ChartsTooNarrowForTheirScaleTakeNoRows(t *testing.T) {
	r := require.New(t)

	m, _ := onClient(t)
	m, _ = m.update(hostUseMsg{nodeID: "node-1", use: nomad.ResourceUse{CPUPercent: 29, CPUTicks: 1165}})
	m, _ = m.update(tea.WindowSizeMsg{Width: 26, Height: 44})

	// Half of this screen holds the scale of the percents and a plot.
	r.Contains(bodyOf(m), "50%")

	// The scale of the numbers is wider and leaves no plot: the charts are
	// dropped, and so is the empty row under them.
	m, _ = m.update(key('u'))

	r.Equal(hostPanelRows, m.panelHeight())
	r.Equal(hostPanelRows, drawnPanel(m))
	r.NotContains(bodyOf(m), "now")
}

func TestClient_TheAllocationsOnItAnswerTheirKeys(t *testing.T) {
	r := require.New(t)

	m, client := onClient(t)

	// A client runs allocations of every namespace: an allocation is
	// restarted in its own namespace.
	asked, _ := m.update(key('r'))
	r.Contains(plain(asked.render()), "Really restart the allocation af1f37df?")

	asked, cmd := answerYes(asked)
	drain(asked, cmd)

	r.Equal(1, client.restarted)
	r.Equal("production", client.askedNamespace)
	r.Equal("af1f37df-4528-6395-3a51-1ffcbf3c2ba4", client.askedID)

	// And described there.
	client.describe = "the allocation, as the cluster describes it"

	described, cmd := m.update(key('d'))
	described = drain(described, cmd)

	r.IsType(describePage{}, described.screen.page)
	r.Contains(plain(described.render()), "Allocation: af1f37df")
	r.Equal("production", client.askedNamespace)
}

// twoOnAClient are two allocations of two jobs on the one client.
func twoOnAClient() []nomad.Alloc {
	other := clientAllocs()[0]
	other.ID, other.JobID, other.TaskGroup = "bb2f37df-0000-0000-0000-000000000000", "cron", "nightly"
	other.Namespace = "staging"
	other.Tasks = []nomad.Task{{Name: "job", State: "running"}}

	return append(clientAllocs(), other)
}

func TestClient_MarksTakeTheAllocationsOnIt(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{nodes: busyClient(), nodeAllocs: twoOnAClient()}

	m, _ := nodeModelOf(client)
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	m, _ = m.update(ctrlKey('a'))
	m, _ = m.update(ctrlKey('k'))
	r.Contains(plain(m.render()), "Really stop 2 allocations?")

	m, cmd = answerYes(m)
	drain(m, cmd)

	r.Equal(2, client.stoppedAllocs)
}

func TestClient_TheLogsOfWhatRunsOnIt(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{nodes: busyClient(), nodeAllocs: twoOnAClient()}

	m, _ := nodeModelOf(client)
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	client.askedNodeID = ""

	m, cmd = m.update(key('l'))
	m = drain(m, cmd)

	// The allocations of the node are read again, and the question names it.
	r.Equal("node-1", client.askedNodeID)
	r.IsType(logTasksPage{}, m.screen.page)
	r.Contains(plain(m.render()), "Logs of which task? (Client: nomad-server-01)")
}

func TestClient_TheLogsOfATaskOnItAreReadAgainFromIt(t *testing.T) {
	r := require.New(t)

	cron := "bb2f37df-0000-0000-0000-000000000000"
	client := &fakeClient{nodes: busyClient(), nodeAllocs: twoOnAClient(), logsByAlloc: map[string]*nomad.LogStream{cron: writing()}}

	m, _ := nodeModelOf(client)
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	m, cmd = m.update(key('l'))
	m = drain(m, cmd)

	// Tasks of several jobs are named with their job.
	r.Contains(fileRow(t, m, 0), "cron/nightly/job")
	r.Contains(fileRow(t, m, 1), "pelmeni_buh_bot/pelmenis/bot")

	m, cmd = m.update(enter())
	m = playOut(m, cmd)
	r.Contains(plain(m.render()), "Logs (Job: cron, Task: job) [stdout, 1 allocation]")

	// Read again, the allocations are those of the client.
	client.askedNodeID, client.logsOpened = "", nil

	m, cmd = m.update(key('r'))
	playOut(m, cmd)

	r.Equal("node-1", client.askedNodeID)
	r.Equal([]string{cron}, client.logsOpened)
}

func TestClient_NothingRunsOnIt(t *testing.T) {
	r := require.New(t)

	allocs := clientAllocs()
	allocs[0].Status = "complete"

	client := &fakeClient{nodes: busyClient(), nodeAllocs: allocs}

	m, _ := nodeModelOf(client)
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	m, cmd = m.update(key('l'))
	m = drain(m, cmd)

	r.IsType(clientPage{}, m.screen.page)
	r.Contains(plain(m.render()), "nomad-server-01 has no allocation running")
}

func TestClient_WatchesTheWorkOfEveryNamespace(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{nodes: busyClient(), nodeAllocs: clientAllocs()}

	m, _ := nodeModelOf(client)
	m, _ = m.update(enter())

	m.update(m.watchScreen()())

	r.Equal(nomad.AllNamespaces, client.watchedNamespace)
	r.Equal([]string{nomad.TopicAllocation}, client.watchedTopics)
}

func TestClient_ComingBackShowsItsOwnAllocations(t *testing.T) {
	r := require.New(t)

	// The allocation on this client was replaced by one on another.
	here := clientAllocs()[0]
	here.Next = "c3d4e5f6-0000-0000-0000-000000000000"

	there := nomad.Alloc{
		ID: here.Next, Namespace: "production", JobID: "cron", TaskGroup: "nightly",
		NodeID: "node-2", NodeName: "nomad-client-02", Status: "running",
	}

	client := &fakeClient{nodes: busyClient(), nodeAllocs: []nomad.Alloc{here}, alloc: here}

	m, _ := nodeModelOf(client)
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	// The allocation, the one that replaced it, and the client that one
	// runs on, with allocations of its own.
	m, cmd = m.update(enter())
	m = drain(m, cmd)

	client.alloc = there
	m, cmd = m.update(key('n'))
	m = drain(m, cmd)

	client.nodeAllocs = []nomad.Alloc{there}
	m, cmd = m.update(key('c'))
	m = drain(m, cmd)
	r.Contains(plain(m.render()), "Client nomad-client-02")

	for range 3 {
		m, _ = m.update(escape())
	}

	// Before it is asked again, the first client shows the allocations it
	// had, not those of the second one.
	out := plain(m.render())
	r.Contains(out, "Client nomad-server-01 [1]")
	r.Contains(out, "pelmeni_buh_bot")
	r.NotContains(out, "nightly")
}

func TestClient_ThePanelSaysWhatTheListKnewUntilTheMachineAnswers(t *testing.T) {
	r := require.New(t)

	m, _ := clientOpened(t, 0)

	out := plain(m.render())
	r.Contains(out, "10.0.0.2")
	r.Contains(out, "dc1")
}

func TestClient_TheMachineAnsweringLeavesTheErrorOfTheList(t *testing.T) {
	r := require.New(t)

	m, _ := onClient(t)
	m, _ = m.update(errMsg{err: errors.New("the list would not come")})

	// The node answered, the list did not: the error of the list stays on
	// the screen.
	m, _ = m.update(hostMsg(busyClient()[0]))

	r.Contains(plain(m.render()), "the list would not come")
}

func TestClient_AReadingTakesTheErrorOfTheLastOneOff(t *testing.T) {
	r := require.New(t)

	m, _ := onClient(t)

	m, _ = m.update(hostUseMsg{nodeID: "node-1", err: errTest})
	r.Contains(plain(m.render()), "no answer")

	m, _ = m.update(reading(10))
	r.NotContains(plain(m.render()), "no answer")
}
