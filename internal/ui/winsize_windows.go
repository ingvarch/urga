//go:build windows

package ui

import (
	"context"
	"time"

	"github.com/ingvarch/urga/internal/nomad"
)

// sizeEvery is how often the terminal is measured on Windows, which has no
// signal to say that the window changed.
const sizeEvery = time.Second

// watchSize tells the task how big the terminal is, now and whenever it
// turns out to have changed.
func watchSize(ctx context.Context, measure func() (width, height int, err error), sizes chan<- nomad.TerminalSize) {
	defer close(sizes)

	last := sendSize(ctx, measure, sizes, nomad.TerminalSize{})

	ticker := time.NewTicker(sizeEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			last = sendSize(ctx, measure, sizes, last)
		case <-ctx.Done():
			return
		}
	}
}

// sendSize sends the size of the terminal when it differs from the last one
// sent.
func sendSize(
	ctx context.Context,
	measure func() (width, height int, err error),
	sizes chan<- nomad.TerminalSize,
	last nomad.TerminalSize,
) nomad.TerminalSize {
	width, height, err := measure()
	if err != nil {
		return last
	}

	size := nomad.TerminalSize{Width: width, Height: height}
	if size == last {
		return last
	}

	select {
	case sizes <- size:
	case <-ctx.Done():
	}

	return size
}
