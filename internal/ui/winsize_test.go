package ui

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestWatchSize_StopsWithTheSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sizes := make(chan nomad.TerminalSize, 1)

	go watchSize(ctx, int(os.Stdin.Fd()), sizes)

	cancel()

	// The watcher closes the channel when the shell ends, whatever the
	// platform tells it about the terminal. Anything it managed to send
	// first is drained on the way.
	done := make(chan struct{})

	go func() {
		defer close(done)

		// Whatever it managed to send before the end is read and dropped,
		// the test waits for the channel to close.
		for range sizes { //nolint:revive // draining is the point
			continue
		}
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("the channel was not closed")
	}
}
