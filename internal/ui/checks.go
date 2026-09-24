package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// checksEvery is how often the checks of an allocation are read. The event
// stream does not say when a check changes: its client keeps the result.
const checksEvery = rowUsageEvery

// Messages of the checks of the allocation on the tasks screen. An answer
// carries the allocation it was asked of: one that arrives after the screen
// moved on belongs to another.
type (
	checksMsg struct {
		allocID string
		checks  []nomad.Check
		err     error
	}

	// pollChecksMsg is the timer of the checks going off.
	pollChecksMsg struct{}
)

// checkState is what the checks of the allocation on the tasks screen last
// said.
type checkState struct {
	allocID string
	list    []nomad.Check

	// due says the next reading or its timer is on its way. The allocation
	// is read again on every change the cluster reports, and none of those
	// may start a second chain of readings next to the first.
	due bool
}

// checksOnce takes the first reading of the checks when the allocation of a
// tasks screen is read, and only then: the timer keeps them coming after
// that.
func checksOnce(m Model, read tea.Cmd) (Model, tea.Cmd) {
	if m.screen.kind != screenTasks || m.checks.due || !m.runs() {
		return m, read
	}

	m.checks.due = true

	return m, tea.Batch(read, m.readChecks())
}

// runs says the allocation on the screen runs: only then are its checks run.
func (m Model) runs() bool {
	alloc, ok := m.shownAlloc()

	return ok && alloc.Status == statusRunning
}

// readChecks reads the checks of the allocation on the screen. Like the
// screen, it belongs to the ask it was made for.
func (m Model) readChecks() tea.Cmd {
	client, namespace, allocID := m.client, m.alloc.Namespace, m.alloc.ID

	return askedFor(m.asked, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		checks, err := client.AllocationChecks(ctx, namespace, allocID)

		return checksMsg{allocID: allocID, checks: checks, err: err}
	})
}

// pollChecks reads the checks again when their timer goes off, while the
// allocation still runs.
func (m Model) pollChecks() (Model, tea.Cmd) {
	if m.screen.kind != screenTasks || !m.runs() {
		m.checks.due = false

		return m, nil
	}

	return m, m.readChecks()
}

// keepChecks puts what the checks said on the panel, and sets the timer for
// the next reading. A client that did not answer says so and is asked again.
func (m Model) keepChecks(msg checksMsg) (Model, tea.Cmd) {
	if m.screen.kind != screenTasks || msg.allocID != m.screen.allocID {
		return m, nil
	}

	next := askedFor(m.asked, tea.Tick(checksEvery, func(time.Time) tea.Msg { return pollChecksMsg{} }))

	if msg.err != nil {
		return m.fail(msg.err), next
	}

	m = m.forget()
	m.checks.allocID, m.checks.list = msg.allocID, msg.checks
	m.layout()

	return m, next
}
