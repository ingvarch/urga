package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// checksEvery is how often the checks of an allocation are read. The event
// stream does not report a change of a check: the Nomad client of the
// allocation keeps the result.
const checksEvery = rowUsageEvery

// Messages of the checks of the allocation on the tasks screen. An answer
// carries the allocation it was requested for: one that arrives after the
// screen switched to another allocation is ignored.
type (
	checksMsg struct {
		allocID string
		checks  []nomad.Check
		err     error
	}

	// pollChecksMsg is the timer of the checks going off.
	pollChecksMsg struct{}
)

// checksOnce requests the checks once, when the allocation of a tasks screen
// is read: after that the timer requests them.
func (p tasksPage) checksOnce(e env) (page, outcome, bool) {
	if p.due || !p.runs() {
		return p, outcome{}, true
	}

	p.due = true

	return p, outcome{cmd: p.readChecks(e)}, true
}

// runs says the allocation on the screen runs: only then are its checks run.
func (p tasksPage) runs() bool {
	return p.read && p.alloc.Status == statusRunning
}

// readChecks reads the checks of the allocation on the screen.
func (p tasksPage) readChecks(e env) tea.Cmd {
	client, namespace, allocID := e.client, p.alloc.Namespace, p.alloc.ID

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		checks, err := client.AllocationChecks(ctx, namespace, allocID)

		return checksMsg{allocID: allocID, checks: checks, err: err}
	}
}

// pollChecks reads the checks again when their timer goes off, while the
// allocation still runs.
func (p tasksPage) pollChecks(e env) (page, outcome, bool) {
	if !p.runs() {
		p.due = false

		return p, outcome{reading: true}, true
	}

	return p, outcome{cmd: p.readChecks(e), reading: true}, true
}

// keepChecks shows the checks on the panel, and sets the timer for the next
// reading. A client that did not answer shows an error and is asked again;
// an answer clears the previous error from the status line.
func (p tasksPage) keepChecks(msg checksMsg) (page, outcome, bool) {
	next := tea.Tick(checksEvery, func(time.Time) tea.Msg { return pollChecksMsg{} })

	if msg.err != nil {
		return p, outcome{now: []tea.Msg{failMsg{err: msg.err}}, cmd: next, reading: true}, true
	}

	p.checks = msg.checks

	return p, outcome{now: []tea.Msg{forgetMsg{}}, cmd: next, reading: true}, true
}
