package nomad

import (
	"context"
	"io"

	"github.com/hashicorp/nomad/api"
)

// TerminalSize is how wide and tall the terminal is, which the task needs to
// know to draw.
type TerminalSize struct {
	Width  int
	Height int
}

// Exec runs a command inside a task and wires the terminal to it. It returns
// what the command exited with.
func (c *Client) Exec(
	ctx context.Context,
	namespace, allocID, task string,
	command []string,
	stdin io.Reader,
	stdout, stderr io.Writer,
	sizes <-chan TerminalSize,
) (int, error) {
	alloc, _, err := c.api.Allocations().Info(allocID, c.query(ctx, namespace))
	if err != nil {
		return 0, err
	}

	// The api takes its own size type, so the sizes are handed over as they
	// arrive.
	apiSizes := make(chan api.TerminalSize)

	go func() {
		defer close(apiSizes)

		for size := range sizes {
			select {
			case apiSizes <- api.TerminalSize{Width: size.Width, Height: size.Height}:
			case <-ctx.Done():
				return
			}
		}
	}()

	return c.api.Allocations().Exec(ctx, alloc, task, true, command, stdin, stdout, stderr, apiSizes, c.query(ctx, namespace))
}
