package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

const (
	// hostChartHeight is how tall the plot of a chart is when the screen has
	// room for it, and hostChartShort what it falls back to. Both divide by
	// four, so that every quarter of the scale falls between two rows and
	// can be drawn where it is. hostChartRest is what a chart takes besides
	// its plot: the reading, the top of the scale, the axis and the times.
	hostChartHeight = 8
	hostChartShort  = 4
	hostChartRest   = 4

	// hostPanelRest is the panel around the charts: the details of the
	// machine, a line under them and a line of air above the allocations.
	hostPanelRest = 3

	// hostPanelRows is the panel without a chart, which is what a screen
	// with no room for one still shows.
	hostPanelRows = 2

	// rowsKept are the rows a table under a panel shows whatever else is on
	// the screen: the allocations of a client, the tasks of an allocation.
	// The panel gives way to them, not the other way around.
	rowsKept = 3

	// hostTrailMax is how many readings a chart keeps. At one reading every
	// hostUseEvery that is the last twenty minutes of the machine.
	hostTrailMax = 240

	// hostUseEvery is how often the chart takes a reading: as often as the
	// rows under it, on a timer of its own.
	hostUseEvery = rowUsageEvery
)

// Messages of the machine a client screen is open on. Both carry the id of
// the machine they were asked of: an answer that arrives after the screen
// moved on belongs to another client.
type (
	hostUseMsg struct {
		nodeID string
		use    nomad.ResourceUse
		err    error
	}

	hostMsg nomad.Node

	// pollHostMsg is the timer of the chart going off.
	pollHostMsg struct{}
)

// hostModel is the machine a client screen is open on, and the readings
// taken of it since it was opened.
type hostModel struct {
	node  nomad.Node
	trail []nomad.ResourceUse

	// due says the next reading or its timer is on its way. The list is
	// answered on every poll, and none of those answers may start a second
	// chain of readings next to the first.
	due bool
}

// fetchHost reads the machine itself, so that what the panel says about it
// keeps up with the rest of the screen.
func fetchHost(client nodesClient, nodeID string) tea.Cmd {
	return request(func(ctx context.Context) (nomad.Node, error) {
		return client.Node(ctx, nodeID)
	}, func(node nomad.Node) tea.Msg { return hostMsg(node) })
}

// fetchHostUse reads what the machine itself is doing, which is more than
// its allocations: the chart shows the host, not the sum of the work.
func fetchHostUse(client nodesClient, nodeID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		use, err := client.NodeUsage(ctx, nodeID)

		return hostUseMsg{nodeID: nodeID, use: use, err: err}
	}
}

// clientPage is one client: the allocations on it, from every namespace,
// under what the machine itself is doing.
type clientPage struct {
	nodeID, name string

	// host is the machine as the page last read it, and the readings taken
	// of it since the page was opened.
	host hostModel

	allocs []nomad.Alloc
}

// clientOf is a client with what is known of the machine so far.
func clientOf(node nomad.Node) clientPage {
	// The readings belong to the machine they were taken on, the chart
	// starts over on every client.
	return clientPage{nodeID: node.ID, name: node.Name, host: hostModel{node: node}}
}

func (p clientPage) title(_ env, count int) string {
	return sprintf("Client %s [%d]", p.name, count)
}

func (clientPage) titles() []string { return allocTitles }
func (clientPage) topics() []string { return []string{nomad.TopicAllocation} }

// where: a client runs the work of every namespace, which is what its stream
// watches.
func (clientPage) where(env) string { return nomad.AllNamespaces }

// fetch reads the work of the machine, and the machine itself: what the
// panel says about it keeps up with the rest of the screen. What the host is
// doing is read on the timer of the chart, not with the list.
func (p clientPage) fetch(e env) tea.Cmd {
	client, nodeID := e.client, p.nodeID

	return tea.Batch(
		fetchList(func(ctx context.Context) ([]nomad.Alloc, error) {
			return client.NodeAllocations(ctx, nodeID)
		}, func(items []nomad.Alloc) tea.Msg { return allocsMsg(items) }),
		fetchHost(client, nodeID),
	)
}

func (p clientPage) take(msg tea.Msg, e env) (page, outcome, bool) {
	switch msg := msg.(type) {
	case allocsMsg:
		p.allocs = msg

		return p.hostOnce(e)

	case hostMsg:
		if msg.ID != p.nodeID {
			return p, outcome{}, false
		}

		// What the panel says about the machine keeps up with it. The
		// machine is not the list: its answer leaves what went wrong with
		// the list on the status line, and the poll where it is.
		p.host.node = nomad.Node(msg)

		return p, outcome{reading: true}, true

	case hostUseMsg:
		if msg.nodeID != p.nodeID {
			return p, outcome{}, false
		}

		return p.keepHostUse(msg)

	case pollHostMsg:
		// The timer of the chart has gone off, and the answer sets the next
		// one.
		return p, outcome{cmd: fetchHostUse(e.client, p.nodeID), reading: true}, true
	}

	return p, outcome{}, false
}

// hostOnce takes the first reading of the chart when the list of the client
// is filled, and only then: the timer of the chart keeps them coming after
// that.
func (p clientPage) hostOnce(e env) (page, outcome, bool) {
	if p.host.due {
		return p, outcome{}, true
	}

	p.host.due = true

	return p, outcome{cmd: fetchHostUse(e.client, p.nodeID)}, true
}

// keepHostUse puts a reading on the chart. A machine that does not answer
// says so and keeps what it said before: a chart that empties on one timeout
// reads as a machine that stopped working.
func (p clientPage) keepHostUse(msg hostUseMsg) (page, outcome, bool) {
	// The next reading is due whatever came of this one: a machine that did
	// not answer once is asked again.
	next := tea.Tick(hostUseEvery, func(time.Time) tea.Msg { return pollHostMsg{} })

	if msg.err != nil {
		return p, outcome{now: []tea.Msg{failMsg{err: msg.err}}, cmd: next, reading: true}, true
	}

	p.host = p.host.keep(msg.use)

	return p, outcome{now: []tea.Msg{forgetMsg{}}, cmd: next, reading: true}, true
}

// restart lets go of the reading of the chart the page thinks is on its
// way: it belonged to an ask that is over.
func (p clientPage) restart() page {
	p.host.due = false

	return p
}

func (p clientPage) rows(e env) []tableRow { return allocRows(p.allocs, e.usage) }

func (p clientPage) visible(env) []nomad.Alloc { return p.allocs }

func (p clientPage) ids(env) []string { return names(p.allocs, allocMark) }

func (p clientPage) readings(e env) []rowRef { return runningRefs(e.index, p.allocs) }

func (clientPage) reading(ctx context.Context, client Client, ref rowRef) (nomad.ResourceUse, error) {
	return allocReading(ctx, client, ref)
}

// logsOf: the logs are read of what runs on the machine.
func (p clientPage) logsOf(env) logScope {
	return logScope{namespace: nomad.AllNamespaces, nodeID: p.nodeID, label: p.name}
}

// clientKeys are the keys of the machine, and after them the keys of the
// allocations on it, which answer here as on any other list of allocations.
var clientKeys = append([]pageKey[clientPage]{
	{press: "e", label: "Events", do: nodeScreen(func(d machine) page { return nodeEventsPage{machine: d} })},
	// Draining is a key of the list of clients; on the screen of one client
	// the same key opens what it can run.
	{press: "ctrl+d", label: "Drivers", do: nodeScreen(func(d machine) page { return nodeDriversPage{machine: d} })},
	{press: "ctrl+h", label: "Host Volumes", do: nodeScreen(func(d machine) page { return nodeVolumesPage{machine: d} })},
	{press: "a", label: "Attributes", do: nodeScreen(func(d machine) page { return nodeAttributesPage{machine: d} })},
	{press: "m", label: "Meta", do: nodeScreen(func(d machine) page { return nodeMetaPage{nodeID: d.nodeID, name: d.name} })},
}, allocKeys[clientPage]()...)

func (p clientPage) keys(e env) []keyHint { return hintsOf(p, e, clientKeys) }

func (p clientPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, clientKeys, k)
}

// keep puts a reading at the end of the chart, which holds the last few
// minutes of them.
func (h hostModel) keep(use nomad.ResourceUse) hostModel {
	h.trail = append(h.trail, use)

	if len(h.trail) > hostTrailMax {
		h.trail = h.trail[len(h.trail)-hostTrailMax:]
	}

	return h
}

// chartWidth is what one of the two charts gets of a panel this wide: half
// of it, which keeps a column for the margin of the table and a gap between
// them.
func chartWidth(width int) int {
	return max((width-1-columnGap)/2, 1)
}

// chartHeight is how tall the plot of a chart is in a panel of room rows. A
// short screen gets the short one, and then none at all: the allocations
// come first. A narrow one gets none either: half of it has no room for a
// chart with its scale.
func chartHeight(half, room int) int {
	if half <= chartAxisWidth {
		return 0
	}

	room -= hostChartRest + hostPanelRest

	switch {
	case room >= hostChartHeight:
		return hostChartHeight
	case room >= hostChartShort:
		return hostChartShort
	}

	return 0
}

// panelHeight is how many rows the panel takes from the table, at the width
// it is drawn at.
func (m Model) panelHeight() int {
	return len(m.panel(m.width - 2*screenPadX - 2))
}

// rowsForPanel is what is left of the box once the table has the rows it
// keeps: its header and a few allocations.
func (m Model) rowsForPanel() int {
	return m.bodyHeight() - 3 - rowsKept
}

// panel is what the screen holds above its rows, sized to it. A client has
// one: what the machine is doing belongs over its allocations, not over a
// list of its attributes. So do the tasks of an allocation: where it runs and
// listens. A page may hold one of its own, like a variable held as a lock
// saying who holds it.
func (m Model) panel(width int) []string {
	if p, ok := m.screen.page.(panelled); ok {
		return p.panel(m.env(), width, m.rowsForPanel())
	}

	return nil
}

// panel is what the machine is doing, above its allocations.
func (p clientPage) panel(_ env, width, room int) []string {
	// A box with no room for even the machine leaves it to the
	// allocations.
	if room < hostPanelRows {
		return nil
	}

	half := chartWidth(width)

	return p.host.view(width, half, chartHeight(half, room))
}

// view is what a client shows above its allocations: what kind of machine
// it is and what it has been doing since the screen was opened. Each chart
// is half wide and plot tall, and a plot of no height leaves them out.
func (h hostModel) view(width, half, plot int) []string {
	// One column of the panel goes to the margin the table keeps.
	width--

	rows := []string{h.details(width), ""}

	if plot > 0 {
		window := time.Duration(half-chartAxisWidth) * hostUseEvery

		cpu := chart{
			name:    "CPU",
			reading: h.cpuReading(),
			detail:  h.cpuDetail(),
			values:  shares(h.trail, cpuShare),
			window:  window,
			color:   colorAccent,
		}.render(half, plot)

		memory := chart{
			name:    "MEM",
			reading: h.memoryReading(),
			detail:  h.memoryDetail(),
			values:  shares(h.trail, memoryShare),
			window:  window,
			color:   colorTitle,
		}.render(half, plot)

		for i := range cpu {
			rows = append(rows, pad(cpu[i], half)+strings.Repeat(" ", columnGap)+memory[i])
		}

		rows = append(rows, "")
	}

	// The panel starts where the columns of the table do.
	for i, row := range rows {
		rows[i] = " " + row
	}

	return rows
}

// details is the machine itself, in the fields the Nomad interface shows.
func (h hostModel) details(width int) string {
	node := h.node

	return fieldLine([]field{
		{"Status", node.Status},
		{"Address", node.Address},
		{"Datacenter", node.Datacenter},
		{"Pool", node.NodePool},
		{"Version", node.Version},
		{"Scheduling", eligibilityOf(node)},
	}, width)
}

// eligibilityOf says whether the machine is taking work.
func eligibilityOf(node nomad.Node) string {
	if node.Drain {
		return "draining"
	}

	return node.Eligibility
}

// The last reading in words: the share of the machine, and the numbers it
// comes from when they are known.
func (h hostModel) cpuReading() string {
	use, ok := h.last()
	if !ok {
		return unknown
	}

	return fmt.Sprintf("%d%%", use.CPUPercent)
}

func (h hostModel) cpuDetail() string {
	use, ok := h.last()
	if !ok || h.node.CPUShares == 0 {
		return ""
	}

	return fmt.Sprintf("%d / %d MHz", use.CPUTicks, h.node.CPUShares)
}

func (h hostModel) memoryReading() string {
	use, ok := h.last()
	if !ok {
		return unknown
	}

	return fmt.Sprintf("%d%%", use.MemoryPercent)
}

func (h hostModel) memoryDetail() string {
	use, ok := h.last()
	if !ok {
		return ""
	}

	total := use.MemoryMBAllowed
	if total == 0 {
		total = h.node.MemoryMB
	}

	if total == 0 {
		return ""
	}

	return fmt.Sprintf("%d / %d MiB", use.MemoryMB, total)
}

func (h hostModel) last() (nomad.ResourceUse, bool) {
	if len(h.trail) == 0 {
		return nomad.ResourceUse{}, false
	}

	return h.trail[len(h.trail)-1], true
}

// shares turns the readings into the 0 to 1 the chart draws.
func shares(trail []nomad.ResourceUse, of func(nomad.ResourceUse) int) []float64 {
	out := make([]float64, 0, len(trail))
	for _, use := range trail {
		out = append(out, float64(max(0, min(of(use), 100)))/100)
	}

	return out
}

func cpuShare(use nomad.ResourceUse) int { return use.CPUPercent }

func memoryShare(use nomad.ResourceUse) int { return use.MemoryPercent }
