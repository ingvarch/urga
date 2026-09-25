package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
)

// A session that stays open for days checks again once a day. When the
// releases cannot be reached, the check gives up after a few seconds.
const (
	releaseEvery   = 24 * time.Hour
	releaseTimeout = 3 * time.Second
)

// Messages of the check for a newer urga.
type (
	newerReleaseMsg struct {
		version string
		err     error
	}

	// checkReleaseMsg fires when the next check is due.
	checkReleaseMsg struct{}
)

// checkRelease asks whether a newer urga is out, if the session can ask.
func (m Model) checkRelease() tea.Cmd {
	ask := m.opts.NewerRelease
	if ask == nil {
		return nil
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), releaseTimeout)
		defer cancel()

		version, err := ask(ctx)

		return newerReleaseMsg{version: version, err: err}
	}
}

// keepRelease stores what the check found and schedules the next one. A
// failed check shows no error and keeps what an earlier one found: a network
// that cannot reach the releases is not the cluster's trouble.
func (m Model) keepRelease(msg newerReleaseMsg) (Model, tea.Cmd) {
	if msg.err == nil {
		m.newer = msg.version
	}

	return m, tea.Tick(releaseEvery, func(time.Time) tea.Msg { return checkReleaseMsg{} })
}
