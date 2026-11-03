//go:build !windows

package ui

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/ingvarch/urga/internal/nomad"
)

// watchSize tells the task how big the terminal is, now and whenever the
// window changes.
func watchSize(ctx context.Context, measure func() (width, height int, err error), sizes chan<- nomad.TerminalSize) {
	defer close(sizes)

	changed := make(chan os.Signal, 1)
	signal.Notify(changed, syscall.SIGWINCH)
	defer signal.Stop(changed)

	sendSize(ctx, measure, sizes)

	for {
		select {
		case <-changed:
			sendSize(ctx, measure, sizes)
		case <-ctx.Done():
			return
		}
	}
}

// sendSize hands over how big the terminal is, if it can be read.
func sendSize(ctx context.Context, measure func() (width, height int, err error), sizes chan<- nomad.TerminalSize) {
	width, height, err := measure()
	if err != nil {
		return
	}

	select {
	case sizes <- nomad.TerminalSize{Width: width, Height: height}:
	case <-ctx.Done():
	}
}
