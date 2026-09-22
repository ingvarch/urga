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
	rowUsageMsg  map[string]nomad.ResourceUse
	pollUsageRow struct{}
)

// fetchUsage reads what the rows on the screen take. Rows nobody is looking
// at are not asked about.
func (m Model) fetchUsage() tea.Cmd {
	ids := m.visibleIDs()
	if len(ids) == 0 {
		return nil
	}

	client, kind, namespace := m.client, m.screen.kind, m.screen.namespace

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		readings := make(map[string]nomad.ResourceUse, len(ids))

		for _, id := range ids {
			var (
				use nomad.ResourceUse
				err error
			)

			if kind == screenNodes {
				use, err = client.NodeUsage(ctx, id)
			} else {
				use, err = client.AllocationUsage(ctx, namespace, id)
			}

			// A machine that does not answer leaves its row empty, the rest
			// of the list is still worth showing.
			if err == nil {
				readings[id] = use
			}
		}

		return rowUsageMsg(readings)
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

// visibleIDs are the resources the rows on the screen stand for, when the
// screen is one that has readings.
func (m Model) visibleIDs() []string {
	switch m.screen.kind {
	case screenAllocations:
		allocs := m.visibleAllocs()

		ids := make([]string, 0, len(m.index))
		for _, at := range m.index {
			if at < len(allocs) {
				ids = append(ids, allocs[at].ID)
			}
		}

		return ids

	case screenNodes:
		ids := make([]string, 0, len(m.index))
		for _, at := range m.index {
			if at < len(m.nodes) {
				ids = append(ids, m.nodes[at].ID)
			}
		}

		return ids
	}

	return nil
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
