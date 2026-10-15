package nomad

import (
	"bytes"
	"context"
	"strings"
	"time"

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

// drainWait is how long a cancelled stream keeps taking frames, so a
// producer blocked on its send reaches its own cancel check.
const drainWait = time.Second

// taskStateDead is how the cluster says a task has stopped for good.
const taskStateDead = "dead"

// taskStateRunning is how the cluster says a task is running.
const taskStateRunning = "running"

// LogStream follows what a task writes. Lines arrive on Lines until the task
// stops or Close is called; Err says why it ended, if it did not end on its
// own.
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

	// OnClose runs when the stream is closed, so a test can check that the
	// request behind it was cancelled.
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
	// would stay open with no more lines to come.
	finished := taskDead(alloc, task)

	frames, errs := c.api.AllocFS().Logs(alloc, !finished, task, source, "end", logTail, cancel, c.query(context.Background(), namespace))

	lines := streamLines(frames, startsCut, nil, cancel)

	return &LogStream{Lines: lines, Err: errs, Finished: finished, Previous: alloc.PreviousAllocation, cancel: cancel}, nil
}

// streamLines passes on what the frames hold, from the first whole line when
// the first frame says the read began in the middle of one. Frames are not
// aligned to lines, so it sends only whole lines: the end of a frame waits for
// the rest of its line. A line still being written shows when it ends, when
// the stream ends, or after logTail bytes. atEnd, when not nil, says a frame
// reached the end of what was there when the stream began, and what is held
// is sent after it, once: a file does not end, so its last line gets no break.
func streamLines(frames <-chan *api.StreamFrame, startsCut, atEnd func(frame *api.StreamFrame) bool, cancel <-chan struct{}) <-chan string {
	lines := make(chan string)

	go func() {
		defer close(lines)

		send := func(chunk []byte) bool { return sendChunk(lines, chunk, frames, cancel) }

		var (
			whole = wholeLines{first: true, startsCut: startsCut}
			frame *api.StreamFrame
			open  bool
		)

		for {
			select {
			case frame, open = <-frames:
			case <-cancel:
				go drain(frames)

				return
			}

			if !open {
				if rest := whole.rest(); len(rest) > 0 {
					send(rest)
				}

				return
			}

			if frame == nil || len(frame.Data) == 0 {
				continue
			}

			chunk := whole.add(frame)

			if atEnd != nil && atEnd(frame) {
				chunk = append(chunk, whole.rest()...)
				atEnd = nil
			}

			if len(chunk) > 0 && !send(chunk) {
				return
			}
		}
	}()

	return lines
}

// wholeLines holds the end of a stream that is not a whole line yet.
type wholeLines struct {
	held []byte

	// unended is set when rest sent a line that has no break yet: a break
	// that comes next ends that line, it does not start a new one.
	unended bool

	// first is set until the first frame is seen, and cut while the rest of
	// a line whose start was not read is still to be dropped.
	first, cut bool
	startsCut  func(first *api.StreamFrame) bool
}

// add takes a frame and gives back what is whole up to its last line break,
// or all of it when the line held grows past logTail with no break.
func (w *wholeLines) add(frame *api.StreamFrame) []byte {
	if w.first {
		w.first, w.cut = false, w.startsCut(frame)
	}

	data := frame.Data

	// Up to the first line break is the rest of a line whose start was not
	// read.
	if w.cut {
		at := bytes.IndexByte(data, '\n')
		if at < 0 {
			return nil
		}

		data, w.cut = data[at+1:], false
	}

	if w.unended && len(data) > 0 {
		w.unended = false

		if data[0] == '\n' {
			data = data[1:]
		}
	}

	data = append(w.held, data...)

	end := bytes.LastIndexByte(data, '\n') + 1

	w.held = append([]byte(nil), data[end:]...)

	if len(w.held) > logTail {
		end, w.held, w.unended = len(data), nil, true
	}

	return data[:end]
}

// rest gives back what is held and forgets it.
func (w *wholeLines) rest() []byte {
	held := w.held
	w.held = nil
	w.unended = len(held) > 0

	return held
}

// sendChunk sends the chunk, or drains the frames when cancel comes first.
func sendChunk(lines chan<- string, chunk []byte, frames <-chan *api.StreamFrame, cancel <-chan struct{}) bool {
	select {
	case lines <- string(chunk):
		return true
	case <-cancel:
		go drain(frames)

		return false
	}
}

// drain takes frames until the channel is closed or drainWait has passed.
func drain(frames <-chan *api.StreamFrame) {
	if frames == nil {
		return
	}

	timeout := time.After(drainWait)

	for {
		select {
		case _, ok := <-frames:
			if !ok {
				return
			}
		case <-timeout:
			return
		}
	}
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
