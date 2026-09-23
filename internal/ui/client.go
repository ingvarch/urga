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

	// hostRowsKept are the allocations a client shows whatever else is on
	// the screen. The charts give way to them, not the other way around.
	hostRowsKept = 3

	// hostTrailMax is how many readings a chart keeps. At one reading per
	// poll that is the last few minutes of the machine.
	hostTrailMax = 240
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
)

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

// keepHostUse puts a reading on the chart. A machine that does not answer
// says so and keeps what it said before: a chart that empties on one timeout
// reads as a machine that stopped working.
func (m Model) keepHostUse(msg hostUseMsg) Model {
	if msg.nodeID != m.screen.nodeID {
		return m
	}

	if msg.err != nil {
		m.err = msg.err

		return m
	}

	m.err = nil
	m.hostTrail = append(m.hostTrail, msg.use)

	if len(m.hostTrail) > hostTrailMax {
		m.hostTrail = m.hostTrail[len(m.hostTrail)-hostTrailMax:]
	}

	return m
}

// chartHeight is how tall the plot of a chart is here. A short screen gets
// the short one, and then none at all: the allocations come first.
func (m Model) chartHeight() int {
	room := m.rowsForPanel() - hostChartRest - hostPanelRest

	switch {
	case room >= hostChartHeight:
		return hostChartHeight
	case room >= hostChartShort:
		return hostChartShort
	}

	return 0
}

// panelHeight is what the screen holds above its rows. Only the screen of a
// client has one: what the machine is doing belongs over its allocations,
// not over a list of its attributes.
func (m Model) panelHeight() int {
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
	return m.bodyHeight() - 3 - hostRowsKept
}

// hostPanel is what a client shows above its allocations: what kind of
// machine it is and what it has been doing since the screen was opened.
func (m Model) hostPanel(width int) []string {
	// One column of the panel goes to the margin the table keeps.
	width--

	rows := []string{m.hostDetails(width), ""}

	if plot := m.chartHeight(); plot > 0 {
		half := max((width-columnGap)/2, 1)
		window := time.Duration(max(half-chartAxisWidth, 1)) * m.opts.PollEvery

		cpu := chart{
			name:    "CPU",
			reading: m.cpuReading(),
			detail:  m.cpuDetail(),
			values:  shares(m.hostTrail, cpuShare),
			window:  window,
			color:   colorAccent,
		}.render(half, plot)

		memory := chart{
			name:    "MEM",
			reading: m.memoryReading(),
			detail:  m.memoryDetail(),
			values:  shares(m.hostTrail, memoryShare),
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

// hostDetails is the machine itself, in the fields the Nomad interface shows.
func (m Model) hostDetails(width int) string {
	node := m.host

	fields := []struct{ label, value string }{
		{"Status", node.Status},
		{"Address", node.Address},
		{"Datacenter", node.Datacenter},
		{"Pool", node.NodePool},
		{"Version", node.Version},
		{"Scheduling", eligibilityOf(node)},
	}

	cells := make([]string, 0, len(fields))
	for _, field := range fields {
		if field.value == "" {
			continue
		}

		cells = append(cells, styleLabel.Render(field.label)+" "+styleValue.Render(field.value))
	}

	return truncate(strings.Join(cells, strings.Repeat(" ", columnGap+1)), width)
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
func (m Model) cpuReading() string {
	use, ok := m.lastHostUse()
	if !ok {
		return unknown
	}

	return fmt.Sprintf("%d%%", use.CPUPercent)
}

func (m Model) cpuDetail() string {
	use, ok := m.lastHostUse()
	if !ok || m.host.CPUShares == 0 {
		return ""
	}

	return fmt.Sprintf("%d / %d MHz", use.CPUTicks, m.host.CPUShares)
}

func (m Model) memoryReading() string {
	use, ok := m.lastHostUse()
	if !ok {
		return unknown
	}

	return fmt.Sprintf("%d%%", use.MemoryPercent)
}

func (m Model) memoryDetail() string {
	use, ok := m.lastHostUse()
	if !ok {
		return ""
	}

	total := use.MemoryMBAllowed
	if total == 0 {
		total = m.host.MemoryMB
	}

	if total == 0 {
		return ""
	}

	return fmt.Sprintf("%d / %d MiB", use.MemoryMB, total)
}

func (m Model) lastHostUse() (nomad.ResourceUse, bool) {
	if len(m.hostTrail) == 0 {
		return nomad.ResourceUse{}, false
	}

	return m.hostTrail[len(m.hostTrail)-1], true
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
