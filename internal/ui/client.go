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
func fetchHost(client Client, nodeID string) tea.Cmd {
	return request(func(ctx context.Context) (nomad.Node, error) {
		return client.Node(ctx, nodeID)
	}, func(node nomad.Node) tea.Msg { return hostMsg(node) })
}

// fetchHostUse reads what the machine itself is doing, which is more than
// its allocations: the chart shows the host, not the sum of the work.
func fetchHostUse(client Client, nodeID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		use, err := client.NodeUsage(ctx, nodeID)

		return hostUseMsg{nodeID: nodeID, use: use, err: err}
	}
}

// hostOnce takes the first reading of the chart when the list of a client is
// filled, and only then: the timer of the chart keeps them coming after that.
func hostOnce(m Model, filled tea.Cmd) (Model, tea.Cmd) {
	if !m.screen.isClient() || m.host.due {
		return m, filled
	}

	m.host.due = true

	return m, tea.Batch(filled, m.readHost())
}

// readHost takes a reading of the machine the screen is open on. Like the
// screen, it belongs to the ask it was made for.
func (m Model) readHost() tea.Cmd {
	return askedFor(m.asked, fetchHostUse(m.client, m.screen.nodeID))
}

// pollHost takes the next reading of the chart. Its timer has gone off, and
// the answer sets the next one.
func (m Model) pollHost() (Model, tea.Cmd) {
	if !m.screen.isClient() {
		return m, nil
	}

	return m, m.readHost()
}

// keepHost keeps what the panel says about the machine up with it.
func (m Model) keepHost(node hostMsg) Model {
	if node.ID != m.screen.nodeID {
		return m
	}

	m.host.node = nomad.Node(node)

	return m
}

// keepHostUse puts a reading on the chart. A machine that does not answer
// says so and keeps what it said before: a chart that empties on one timeout
// reads as a machine that stopped working.
func (m Model) keepHostUse(msg hostUseMsg) (Model, tea.Cmd) {
	if msg.nodeID != m.screen.nodeID {
		return m, nil
	}

	// The next reading is due whatever came of this one: a machine that did
	// not answer once is asked again.
	next := askedFor(m.asked, tea.Tick(hostUseEvery, func(time.Time) tea.Msg { return pollHostMsg{} }))

	if msg.err != nil {
		return m.fail(msg.err), next
	}

	m = m.forget()
	m.host = m.host.keep(msg.use)

	return m, next
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

// chartWidth is what one of the two charts gets: half of the panel, which
// keeps a column for the margin of the table and a gap between them.
func (m Model) chartWidth() int {
	return max((m.width-2*screenPadX-3-columnGap)/2, 1)
}

// chartHeight is how tall the plot of a chart is here. A short screen gets
// the short one, and then none at all: the allocations come first. A narrow
// one gets none either: half of it has no room for a chart with its scale.
func (m Model) chartHeight() int {
	if m.chartWidth() <= chartAxisWidth {
		return 0
	}

	room := m.rowsForPanel() - hostChartRest - hostPanelRest

	switch {
	case room >= hostChartHeight:
		return hostChartHeight
	case room >= hostChartShort:
		return hostChartShort
	}

	return 0
}

// panelHeight is what the screen holds above its rows. A client has one:
// what the machine is doing belongs over its allocations, not over a list of
// its attributes. So do the tasks of an allocation: where it runs and
// listens.
func (m Model) panelHeight() int {
	if m.screen.kind == screenTasks {
		return len(m.tasksPanel(m.width))
	}

	if m.screen.isDeployment() {
		return len(m.deploymentScreenPanel(m.width))
	}

	if !m.screen.isClient() {
		return 0
	}

	if plot := m.chartHeight(); plot > 0 {
		return plot + hostChartRest + hostPanelRest
	}

	if m.rowsForPanel() >= hostPanelRows {
		return hostPanelRows
	}

	return 0
}

// rowsForPanel is what is left of the box once the table has the rows it
// keeps: its header and a few allocations.
func (m Model) rowsForPanel() int {
	return m.bodyHeight() - 3 - rowsKept
}

// panel is what the screen holds above its rows, sized to it.
func (m Model) panel(width int) []string {
	if m.screen.kind == screenTasks {
		return m.tasksPanel(width)
	}

	if m.screen.isDeployment() {
		return m.deploymentScreenPanel(width)
	}

	return m.host.view(width, m.chartWidth(), m.chartHeight(), hostUseEvery)
}

// view is what a client shows above its allocations: what kind of machine
// it is and what it has been doing since the screen was opened. Each chart
// is half wide and plot tall, and a plot of no height leaves them out.
func (h hostModel) view(width, half, plot int, every time.Duration) []string {
	// One column of the panel goes to the margin the table keeps.
	width--

	rows := []string{h.details(width), ""}

	if plot > 0 {
		window := time.Duration(half-chartAxisWidth) * every

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
		out = append(out, float64(clamp(of(use), 0, 100))/100)
	}

	return out
}

func cpuShare(use nomad.ResourceUse) int { return use.CPUPercent }

func memoryShare(use nomad.ResourceUse) int { return use.MemoryPercent }

// openClient drills into the client under the cursor: what it runs, under
// what the machine itself is doing.
func openClient(m Model) (Model, tea.Cmd) {
	node, ok := selectedOf(m, screenNodes, m.nodes)
	if !ok {
		return m, nil
	}

	return m.openNode(node)
}

// openNode opens the screen of one client.
func (m Model) openNode(node nomad.Node) (Model, tea.Cmd) {
	// The readings belong to the machine they were taken on, the chart
	// starts over on every client.
	m.host = hostModel{node: node}

	return m.push(screen{
		kind:      screenAllocations,
		namespace: nomad.AllNamespaces,
		nodeID:    node.ID,
		label:     node.Name,
	})
}
