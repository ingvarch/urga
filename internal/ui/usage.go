package ui

import (
	"context"
	"fmt"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// usageEvery is how often the readings of the rows on the screen are taken.
// Every one of them is a request to the machine that runs the work, so they
// are taken less often than the list itself is refreshed.
const rowUsageEvery = 5 * time.Second

// Messages of the readings.
type (
	// rowUsageMsg is what the rows on the screen take, and what went wrong
	// for the rows that said nothing.
	rowUsageMsg struct {
		readings map[string]nomad.ResourceUse
		missing  int
		reason   error
	}

	pollUsageRow struct{}
)

// fetchUsage reads what the rows on the screen take. Rows nobody is looking
// at are not asked about, and a screen with no readings asks nothing.
func (m Model) fetchUsage() tea.Cmd {
	res := m.screen.of()
	if res.readings == nil || res.reading == nil {
		return nil
	}

	ids := res.readings(m)
	if len(ids) == 0 {
		return nil
	}

	client, namespace, read := m.client, m.screen.namespace, res.reading

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		out := rowUsageMsg{readings: make(map[string]nomad.ResourceUse, len(ids))}

		for _, id := range ids {
			use, err := read(client, ctx, namespace, id)

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

			out.readings[id] = use
		}

		return out
	}
}

// usageOnce takes the readings when a screen that has them is filled, and
// only then: the timer keeps them coming after that.
func (m Model) usageOnce() tea.Cmd {
	if len(m.rowUsage) > 0 {
		return nil
	}

	return m.fetchUsage()
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
