package ui

import (
	"context"
	"fmt"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

const (
	// usageEvery is how often the header reads the load of the cluster. It
	// walks every node and allocation, so it is not asked for often.
	usageEvery = 15 * time.Second

	// rowUsageEvery is how often the readings of the rows on the screen are
	// taken. Every one of them is a request to the machine that runs the
	// work, so they are taken less often than the list itself is refreshed.
	rowUsageEvery = 5 * time.Second
)

// Messages of the readings.
type (
	// usageMsg carries where it was read: numbers of a region or a
	// datacenter the session has left must not stand under the new name.
	usageMsg struct {
		region     string
		datacenter string
		usage      nomad.Usage
	}

	pollUsageMsg struct{}

	// rowUsageMsg is what the rows on the screen take, and what went wrong
	// for the rows that said nothing.
	rowUsageMsg struct {
		readings map[string]nomad.ResourceUse
		missing  int
		reason   error
	}

	pollUsageRow struct{}
)

// usageState is what the cluster and the rows on the screen are busy with,
// and which readings have the next one on its way.
type usageState struct {
	// cluster is what the header shows.
	cluster nomad.Usage

	// clusterDue says the timer for the next reading of the header is on its
	// way. A switch reads at once, and its answer must not start a second
	// timer next to the first.
	clusterDue bool

	// rows is what each row on the screen takes, by its id, and why the
	// rest of them said nothing.
	rows    map[string]nomad.ResourceUse
	missing int
	reason  error

	// rowsDue says the readings of the rows or their timer are on their
	// way. The list is answered on every poll, and none of those answers may
	// start a second chain of readings next to the first.
	rowsDue bool
}

// keepCluster puts up a reading of the header that was taken where the
// session looks, and sets the timer for the next one unless it is already on
// its way.
func (u usageState) keepCluster(use nomad.Usage, here bool) (usageState, tea.Cmd) {
	if here {
		u.cluster = use
	}

	if u.clusterDue {
		return u, nil
	}

	u.clusterDue = true

	return u, tea.Tick(usageEvery, func(time.Time) tea.Msg { return pollUsageMsg{} })
}

// keepRows puts up what the rows take.
func (u usageState) keepRows(msg rowUsageMsg) usageState {
	u.rows, u.missing, u.reason = msg.readings, msg.missing, msg.reason

	return u
}

// forgetRows lets go of the readings of the rows when a screen is put up.
func (u usageState) forgetRows() usageState {
	u.rows, u.missing, u.reason, u.rowsDue = nil, 0, nil, false

	return u
}

// missingNote says why some rows show no reading, and nothing when they all
// have one or nobody said why.
func (u usageState) missingNote() string {
	if u.missing == 0 || u.reason == nil {
		return ""
	}

	return fmt.Sprintf("no readings for %d rows: %s", u.missing, u.reason)
}

// rowRef is one row a screen takes a reading of. The namespace travels with
// it: a client runs the work of every namespace, and a reading is asked for
// where the allocation lives.
type rowRef struct {
	namespace string
	id        string
}

// fetchUsage reads what the rows on the screen take. Rows nobody is looking
// at are not asked about, and a screen with no readings asks nothing.
func (m Model) fetchUsage() tea.Cmd {
	res := m.screen.of()
	if res.readings == nil || res.reading == nil {
		return nil
	}

	refs := res.readings(m)
	if len(refs) == 0 {
		return nil
	}

	client, read := m.client, res.reading

	return askedFor(m.asked, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		out := rowUsageMsg{readings: make(map[string]nomad.ResourceUse, len(refs))}

		for _, ref := range refs {
			use, err := read(client, ctx, ref)

			// A machine that does not answer leaves its row empty, the rest
			// of the list is still worth showing. What it said is kept, a
			// dash in a column explains nothing by itself.
			if err != nil {
				out.missing++

				if out.reason == nil {
					out.reason = err
				}

				continue
			}

			out.readings[ref.id] = use
		}

		return out
	})
}

// usageOnce takes the readings when a screen that has them is filled, and
// only then: the timer keeps them coming after that. They go out with what
// filling the screen asked for.
func usageOnce(m Model, filled tea.Cmd) (Model, tea.Cmd) {
	if m.usage.rowsDue {
		return m, filled
	}

	m, read := m.readRows()

	return m, tea.Batch(filled, read)
}

// readRows asks for the readings and writes down that they are on their way.
// A screen with nothing to read ends the chain, and the next one that has
// readings starts its own.
func (m Model) readRows() (Model, tea.Cmd) {
	read := m.fetchUsage()
	m.usage.rowsDue = read != nil

	return m, read
}

// keepRowUsage puts up what the rows take and sets the timer for the next
// reading. Like the reading, the timer belongs to the screen it was set on.
func (m Model) keepRowUsage(msg rowUsageMsg) (Model, tea.Cmd) {
	m.usage = m.usage.keepRows(msg)
	m.layout()

	return m, askedFor(m.asked, tea.Tick(rowUsageEvery, func(time.Time) tea.Msg { return pollUsageRow{} }))
}

// keepClusterUsage puts up what the header reads.
func (m Model) keepClusterUsage(msg usageMsg) (Model, tea.Cmd) {
	var tick tea.Cmd

	here := msg.region == m.client.Region() && msg.datacenter == m.datacenter
	m.usage, tick = m.usage.keepCluster(msg.usage, here)

	return m, tick
}

// readClusterUsage takes the next reading of the header. Its timer has
// fired, and the answer sets the next one.
func (m Model) readClusterUsage() (Model, tea.Cmd) {
	m.usage.clusterDue = false

	return m, m.fetchClusterUsage()
}

// fetchClusterUsage reads what the datacenter in use is busy with, or the
// whole region, which the header shows. What one row takes is read by
// Model.fetchUsage.
func (m Model) fetchClusterUsage() tea.Cmd {
	client, datacenter := m.client, m.datacenter

	return m.onConnection(request(func(ctx context.Context) (nomad.Usage, error) {
		return client.Usage(ctx, datacenter)
	}, func(usage nomad.Usage) tea.Msg {
		return usageMsg{region: client.Region(), datacenter: datacenter, usage: usage}
	}))
}

// percentCell is what a machine of the cluster is busy with: there is no
// limit to compare it against, the share is what the node reports.
func percentCell(value int, known bool) string {
	if !known {
		return "-"
	}

	return fmt.Sprintf("%d%%", value)
}

// cpuCell and memoryCell are a reading as it reads in a column: a share of
// what was asked for, or the number itself when nothing was asked for.
func cpuCell(use nomad.ResourceUse, known bool) string {
	if !known {
		return "-"
	}

	if use.CPUTicksAllowed > 0 {
		return fmt.Sprintf("%d%%", use.CPUPercent)
	}

	return strconv.Itoa(use.CPUTicks)
}

func memoryCell(use nomad.ResourceUse, known bool) string {
	if !known {
		return "-"
	}

	if use.MemoryMBAllowed > 0 {
		return fmt.Sprintf("%d%%", use.MemoryPercent)
	}

	return fmt.Sprintf("%dM", use.MemoryMB)
}
