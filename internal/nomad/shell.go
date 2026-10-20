package nomad

import (
	"bytes"
	"context"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/hashicorp/nomad/api"
)

// TerminalSize is how wide and tall the terminal is, which the task needs to
// know to draw.
type TerminalSize struct {
	Width  int
	Height int
}

// Exec runs a command inside a task and connects the terminal to it. It
// returns the exit code of the command.
func (c *Client) Exec(
	ctx context.Context,
	namespace, allocID, task string,
	command []string,
	stdin io.Reader,
	stdout, stderr io.Writer,
	sizes <-chan TerminalSize,
) (int, error) {
	// The api takes its own size type, so each size is converted and passed
	// on as it arrives.
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

	return c.exec(ctx, namespace, allocID, task, true, command, stdin, stdout, stderr, apiSizes)
}

// CommandOutput is what a command printed to stdout and stderr, in the order
// the task printed it, and the exit code.
type CommandOutput struct {
	Text     string
	ExitCode int

	// Size is how many bytes the command printed, and From how many of them
	// were dropped from the start to keep only its end.
	Size int64
	From int64
}

// outputTail is how much of what a command printed is kept: its last MiB.
// A command such as yes prints hundreds of MiB in the time urga waits for it.
const outputTail = 1 << 20

// RunCommand runs a command inside a task with no terminal and nothing on
// its stdin, and returns what it printed: all of it, or the whole lines of
// its last MiB.
func (c *Client) RunCommand(ctx context.Context, namespace, allocID, task string, command []string) (CommandOutput, error) {
	printed := tail{limit: outputTail}

	code, err := c.exec(ctx, namespace, allocID, task, false, command, strings.NewReader(""), &printed, &printed, nil)
	if err != nil {
		// The api may still be writing to the buffer.
		return CommandOutput{}, err
	}

	text := printed.text()

	return CommandOutput{Text: string(text), ExitCode: code, Size: printed.size, From: printed.size - int64(len(text))}, nil
}

// tail keeps the last limit bytes written to it, and the byte before them,
// and counts all of them.
type tail struct {
	limit int
	kept  []byte
	size  int64
}

func (t *tail) Write(p []byte) (int, error) {
	// The byte before the last limit bytes says whether a line starts
	// right where they start.
	keep := t.limit + 1

	t.size += int64(len(p))
	t.kept = append(t.kept, p[max(len(p)-keep, 0):]...)

	// The start is dropped only once twice the limit is held, so a byte is
	// copied a few times at most, not once per write.
	if len(t.kept) > 2*t.limit {
		t.kept = append(t.kept[:0], t.kept[len(t.kept)-keep:]...)
	}

	return len(p), nil
}

// text is all that was written, or when that was more than the limit, the
// lines that start within its last limit bytes. When no line starts there,
// it is those bytes from the first character that starts within them.
func (t *tail) text() []byte {
	if t.size <= int64(t.limit) {
		return t.kept
	}

	kept := t.kept[len(t.kept)-t.limit-1:]
	if at := bytes.IndexByte(kept[:len(kept)-1], '\n'); at >= 0 {
		return kept[at+1:]
	}

	// A character cut by the limit loses its first bytes, so the rest of it
	// is dropped too.
	start := 1
	for start < min(len(kept), utf8.UTFMax) && !utf8.RuneStart(kept[start]) {
		start++
	}

	return kept[start:]
}

func (c *Client) exec(
	ctx context.Context,
	namespace, allocID, task string,
	tty bool,
	command []string,
	stdin io.Reader,
	stdout, stderr io.Writer,
	sizes <-chan api.TerminalSize,
) (int, error) {
	alloc, _, err := c.api.Allocations().Info(allocID, c.query(ctx, namespace))
	if err != nil {
		return 0, err
	}

	return c.api.Allocations().Exec(ctx, alloc, task, tty, command, stdin, stdout, stderr, sizes, c.query(ctx, namespace))
}
