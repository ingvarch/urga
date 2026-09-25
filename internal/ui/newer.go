package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
)

// A session that stays open for days asks again once a day. A network that
// cannot reach the releases waits no longer than it takes to say so.
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

	// checkReleaseMsg is the timer of the next ask going off.
	checkReleaseMsg struct{}
)

// checkRelease asks whether a newer urga is out, when the session may.
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

// keepRelease stores what the ask found and sets off the next one. A failed
// ask says nothing and keeps what an earlier one found: a network that
// cannot reach the releases is not the cluster's trouble.
func (m Model) keepRelease(msg newerReleaseMsg) (Model, tea.Cmd) {
	if msg.err == nil {
		m.newer = msg.version
	}

	return m, tea.Tick(releaseEvery, func(time.Time) tea.Msg { return checkReleaseMsg{} })
}
