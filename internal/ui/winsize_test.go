package ui

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestWatchSize_StopsWithTheSession(t *testing.T) {
	r := require.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	sizes := make(chan nomad.TerminalSize, 1)

	go watchSize(ctx, int(os.Stdin.Fd()), sizes)

	cancel()

	// The watcher lets go of the channel when the shell ends, whatever the
	// platform tells it about the terminal.
	select {
	case _, more := <-sizes:
		if more {
			select {
			case _, more := <-sizes:
				r.False(more)
			case <-time.After(time.Second):
				t.Fatal("the channel was not closed")
			}
		}
	case <-time.After(time.Second):
		t.Fatal("the channel was not closed")
	}
}
