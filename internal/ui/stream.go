package ui

import (
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
)

// heldStream is what a held stream wait answers when nothing has come: it
// would wait.
type heldStream struct{}

// streamsHeld is set by tests that play out commands, to tell a stream wait
// from slow work.
var streamsHeld atomic.Bool

// waitOn is the one place a command waits on a stream: for the next thing
// on c, or for the error the stream ends with. A held wait does not wait.
func waitOn[T any](c <-chan T, errs <-chan error, next func(T, bool) tea.Msg, ended func(error) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		if streamsHeld.Load() {
			select {
			case v, ok := <-c:
				return next(v, ok)
			case err := <-errs:
				return ended(err)
			default:
				return heldStream{}
			}
		}

		select {
		case v, ok := <-c:
			return next(v, ok)
		case err := <-errs:
			return ended(err)
		}
	}
}
