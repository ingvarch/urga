package ui

import (
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
)

// heldTimer is what a held timer answers: how long it would wait, and what
// it would send then.
type heldTimer struct {
	wait time.Duration
	msg  tea.Msg
}

// timersHeld is set by tests that hold timers, to tell a timer from slow
// work.
var timersHeld atomic.Bool

// startTimer is the one place a timer starts. The wait starts when the
// command is made.
func startTimer(wait time.Duration, msg tea.Msg) tea.Cmd {
	tick := tea.Tick(wait, func(time.Time) tea.Msg { return msg })

	return func() tea.Msg {
		if timersHeld.Load() {
			return heldTimer{wait: wait, msg: msg}
		}

		return tick()
	}
}
