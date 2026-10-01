package ui

import (
	"context"
	"fmt"
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

const (
	// hostChartHeight is how tall the plot of a chart is when the screen has
	// room for it, and hostChartShort what it falls back to. The tall one
	// is a row more than divides by four and the short one a row more than
	// divides by two, so that every quarter of the scale, or its half,
	// falls on a row and is marked where it is. hostChartRest is what a
	// chart takes besides its plot: the reading and the times.
	hostChartHeight = 13
	hostChartShort  = 7
	hostChartRest   = 2

	// hostPanelRest is the panel around the charts: the details of the
	// machine, a line under them and an empty line above the allocations.
	hostPanelRest = 3

	// hostPanelRows is the panel without a chart, which is what a screen
	// with no room for one still shows.
	hostPanelRows = 2

	// rowsKept are the rows a table under a panel shows whatever else is on
	// the screen: the allocations of a client, the tasks of an allocation.
	// The panel shrinks to leave them room.
	rowsKept = 3

	// hostTrailMax is how many readings a chart keeps. At one reading every
	// hostUseEvery that is the last twenty minutes of the machine.
	hostTrailMax = 240

	// hostUseEvery is how often the chart takes a reading: as often as the
	// rows under it, on a timer of its own.
	hostUseEvery = rowUsageEvery
)

// Messages of the machine a client screen is open on. Both carry the id of
// the machine they were requested for: an answer that arrives after the
// screen switched to another client is about the wrong machine.
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

	// due is true while the next reading or its timer is pending. The list
	// arrives again on every poll, and none of those answers may start a
	// second chain of readings next to the first.
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

	// absolute reads the charts in megahertz and mebibytes; without it
	// they read percents.
	absolute bool

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

		// The panel shows the machine as last read. This answer is not the
		// list: it does not clear an error of the list from the status line,
		// and it does not reschedule the poll.
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

// keepHostUse adds a reading to the chart. When the machine does not answer,
// the error is shown and the chart keeps its readings: a chart that empties
// on one timeout looks like a machine that stopped working.
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

// restart clears the flag of a chart reading the page thinks is pending:
// that reading was for a request that is over.
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
// allocations on it, which work here as on any other list of allocations.
var clientKeys = append([]pageKey[clientPage]{
	{press: "e", label: "Events", do: nodeScreen(func(d machine) page { return nodeEventsPage{machine: d} })},
	// Draining is a key of the list of clients; on the screen of one client
	// the same key opens its drivers.
	{press: "ctrl+d", label: "Drivers", do: nodeScreen(func(d machine) page { return nodeDriversPage{machine: d} })},
	{press: "ctrl+h", label: "Host Volumes", do: nodeScreen(func(d machine) page { return nodeVolumesPage{machine: d} })},
	{press: "a", label: "Attributes", do: nodeScreen(func(d machine) page { return nodeAttributesPage{machine: d} })},
	{press: "m", label: "Meta", do: nodeScreen(func(d machine) page { return nodeMetaPage{nodeID: d.nodeID, name: d.name} })},
	{press: "u", label: "Units", do: toggleUnits},
}, allocKeys[clientPage]()...)

func (p clientPage) keys(e env) []keyHint { return hintsOf(p, e, clientKeys) }

func (p clientPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, clientKeys, k)
}

// toggleUnits reads the charts in numbers and back in percents: megahertz
// for CPU, mebibytes for memory.
func toggleUnits(p clientPage, _ env) (clientPage, outcome) {
	p.absolute = !p.absolute

	return p, outcome{}
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
// come first.
func chartHeight(room int) int {
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
	// A box with no room even for the details of the machine gives all its
	// rows to the allocations.
	if room < hostPanelRows {
		return nil
	}

	half := chartWidth(width)

	return p.host.view(width, half, chartHeight(room), p.absolute)
}

// view is what a client shows above its allocations: what kind of machine
// it is and what it has been doing since the screen was opened. Each chart
// is half wide and plot tall, and a plot of no height leaves them out.
func (h hostModel) view(width, half, plot int, absolute bool) []string {
	// One column of the panel goes to the margin the table keeps.
	width--

	rows := []string{h.details(width), ""}

	if plot > 0 {
		cpu, memory := alignLabels(h.chart(cpuMeasure, absolute), h.chart(memoryMeasure, absolute))
		cpuLines := cpu.render(half, plot)
		memoryLines := memory.render(half, plot)

		for i := range cpuLines {
			rows = append(rows, pad(cpuLines[i], half)+strings.Repeat(" ", columnGap)+memoryLines[i])
		}

		// A narrow screen gets no charts: half of it has no room for one
		// with its scale. Then there is nothing to keep apart from the
		// allocations either.
		if len(cpuLines) > 0 {
			rows = append(rows, "")
		}
	}

	// The panel starts where the columns of the table do.
	for i, row := range rows {
		rows[i] = " " + row
	}

	return rows
}

// measure is one thing the charts read of a machine: its CPU or its memory.
type measure struct {
	name  string
	color color.Color

	// share is how much of a hundred a reading takes, used the number
	// behind it.
	share, used func(nomad.ResourceUse) int

	// has is what the machine has of it, nothing when it does not say.
	has func(hostModel) int

	// small is the unit the numbers come in, and big the one they read in
	// once the total holds per of the small one: gigahertz past a thousand
	// megahertz, gibibytes past a gibibyte.
	small, big string
	per        float64
}

var (
	cpuMeasure = measure{
		name:  "CPU",
		color: colorAccent,
		share: func(use nomad.ResourceUse) int { return use.CPUPercent },
		used:  func(use nomad.ResourceUse) int { return use.CPUTicks },
		has:   func(h hostModel) int { return h.node.CPUShares },
		small: mhzUnit, big: ghzUnit, per: 1000,
	}

	memoryMeasure = measure{
		name:  "MEM",
		color: colorTitle,
		share: func(use nomad.ResourceUse) int { return use.MemoryPercent },
		used:  func(use nomad.ResourceUse) int { return use.MemoryMB },
		has:   memoryOf,
		small: mibUnit, big: gibUnit, per: 1024,
	}
)

// memoryOf is the memory the last reading says the work may take, or what
// the machine has when the reading does not say.
func memoryOf(h hostModel) int {
	if use, ok := h.last(); ok && use.MemoryMBAllowed > 0 {
		return use.MemoryMBAllowed
	}

	return h.node.MemoryMB
}

// unit is the unit the numbers read in against a total, and how many of the
// small one make it.
func (m measure) unit(total float64) (string, float64) {
	if total >= m.per {
		return m.big, m.per
	}

	return m.small, 1
}

// chart is one measure of the machine since the screen was opened: a share
// of a hundred, or the numbers against what the machine has.
func (h hostModel) chart(of measure, absolute bool) chart {
	c := chart{name: of.name, color: of.color, every: hostUseEvery}

	use, read := h.last()
	has := float64(of.has(h))

	// The last reading in words: the share of the machine, and the numbers
	// it comes from when they are known.
	share := unknown
	if read {
		share = fmt.Sprintf("%d%%", of.share(use))
	}

	// The numbers read against what the machine has, or the most it used
	// since the screen was opened. With neither, the percents are drawn.
	total := has
	if total == 0 {
		total = maxOf(counts(h.trail, of.used, 1))
	}

	if absolute && total > 0 {
		unit, per := of.unit(total)

		c.values, c.max, c.unit = counts(h.trail, of.used, per), total/per, unit
		c.reading, c.detail = unknown, share

		if read {
			c.reading = amount(float64(of.used(use))/per, unit)
		}

		return c
	}

	c.values, c.max, c.unit = counts(h.trail, of.share, 1), 100, percentUnit
	c.reading = share

	if read && has > 0 {
		unit, per := of.unit(has)
		c.detail = pair(float64(of.used(use))/per, has/per, unit)
	}

	return c
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

func (h hostModel) last() (nomad.ResourceUse, bool) {
	if len(h.trail) == 0 {
		return nomad.ResourceUse{}, false
	}

	return h.trail[len(h.trail)-1], true
}

// counts turns the readings into the numbers the plot counts, per of them
// to one of its unit. A reading past the top of the scale is drawn on it.
func counts(trail []nomad.ResourceUse, of func(nomad.ResourceUse) int, per float64) []float64 {
	out := make([]float64, 0, len(trail))
	for _, use := range trail {
		out = append(out, float64(max(of(use), 0))/per)
	}

	return out
}

// maxOf is the most of the numbers, or nothing when there are none.
func maxOf(values []float64) float64 {
	top := 0.0
	for _, value := range values {
		top = max(top, value)
	}

	return top
}
