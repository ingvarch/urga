package nomad

import (
	"bytes"
	"context"
	"strings"

	"github.com/hashicorp/nomad/api"
)

// Log sources a task writes to.
const (
	LogStdout = "stdout"
	LogStderr = "stderr"
)

// logTail is how much of a log is read back when it is opened: the last
// screens of it, without flooding the screen with a chatty task.
const logTail = 64 << 10

// taskStateDead is how the cluster says a task has stopped for good.
const taskStateDead = "dead"

// LogStream is a task writing. Lines arrive on Lines until the task stops or
// Close is called; Err says why it ended, if it did not end on its own.
type LogStream struct {
	Lines <-chan string
	Err   <-chan error

	// Finished says the task is dead: what it wrote is all it will write,
	// and the stream ends with it.
	Finished bool

	// Previous is the allocation this one replaced, empty for the first.
	Previous string

	// Size is how big a file was when it was opened, and From where in it
	// reading began: over zero, only its end was read.
	Size int64
	From int64

	// OnClose runs when the stream is closed, which is how a test sees that
	// the request behind it was let go of.
	OnClose func()

	cancel chan struct{}
}

// Close stops the stream and the request behind it.
func (s *LogStream) Close() {
	if s.OnClose != nil {
		s.OnClose()
	}

	if s.cancel == nil {
		return
	}

	select {
	case <-s.cancel:
	default:
		close(s.cancel)
	}
}

// Logs follows what a task writes. The caller closes the stream when it stops
// reading, otherwise the request stays open.
func (c *Client) Logs(ctx context.Context, namespace, allocID, task, source string) (*LogStream, error) {
	alloc, _, err := c.api.Allocations().Info(allocID, c.query(ctx, namespace))
	if err != nil {
		return nil, err
	}

	cancel := make(chan struct{})

	// A dead task is read to the end of what it wrote. Followed, its stream
	// would stay open with nothing more to say.
	finished := taskDead(alloc, task)

	frames, errs := c.api.AllocFS().Logs(alloc, !finished, task, source, "end", logTail, cancel, c.query(context.Background(), namespace))

	lines := streamLines(frames, startsCut, cancel)

	return &LogStream{Lines: lines, Err: errs, Finished: finished, Previous: alloc.PreviousAllocation, cancel: cancel}, nil
}

// streamLines passes on what the frames hold, from the first whole line when
// the first frame says the read began in the middle of one.
func streamLines(frames <-chan *api.StreamFrame, startsCut func(first *api.StreamFrame) bool, cancel <-chan struct{}) <-chan string {
	lines := make(chan string)

	go func() {
		defer close(lines)

		first, cut := true, false

		for frame := range frames {
			if frame == nil || len(frame.Data) == 0 {
				continue
			}

			if first {
				first, cut = false, startsCut(frame)
			}

			data := frame.Data

			// Up to the first line break is the rest of a line whose start
			// was not read.
			if cut {
				at := bytes.IndexByte(data, '\n')
				if at < 0 {
					continue
				}

				data, cut = data[at+1:], false
			}

			if len(data) == 0 {
				continue
			}

			select {
			case lines <- string(data):
			case <-cancel:
				return
			}
		}
	}()

	return lines
}

// taskDead says the task of the allocation has stopped for good.
func taskDead(alloc *api.Allocation, task string) bool {
	state, ok := alloc.TaskStates[task]

	return ok && state != nil && state.State == taskStateDead
}

// startsCut says the log read back does not start where the task started
// writing, so its first line is likely cut. A frame ends at its Offset in its
// File, and a log is turned over into a new file by size, not at a line break.
func startsCut(frame *api.StreamFrame) bool {
	start := frame.Offset - int64(len(frame.Data))

	return start > 0 || !strings.HasSuffix(frame.File, ".0")
}
