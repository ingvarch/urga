package nomad

import (
	"bytes"
	"context"
	"math"
	"strconv"
	"strings"
	"sync/atomic"
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

	// start and there are written by the goroutine that reads the frames,
	// before it sends the first line. there is how many bytes of the text
	// the log held when it was opened, and given how many bytes Old has
	// seen.
	start atomic.Pointer[LogMark]
	there atomic.Int64
	given atomic.Int64

	// end is where the log ended when it was opened, when endKnown says it
	// was read. all says every line was there when it opened: a dead task
	// writes no more.
	end      LogMark
	endKnown bool
	all      bool
}

// Start is where the first line the stream sent begins: the zero mark until
// it sent one.
func (s *LogStream) Start() LogMark {
	if mark := s.start.Load(); mark != nil {
		return *mark
	}

	return LogMark{}
}

// Old says how many whole lines at the front of a chunk the log held when it
// was opened, the rest being what the task wrote after. It is called once per
// chunk, in the order they were sent.
func (s *LogStream) Old(chunk string) int {
	there, given := s.there.Load(), s.given.Load()
	s.given.Store(given + int64(len(chunk)))

	count, at := 0, 0

	for at < len(chunk) {
		end := len(chunk)
		if i := strings.IndexByte(chunk[at:], '\n'); i >= 0 {
			end = at + i + 1
		}

		if given+int64(end) > there {
			break
		}

		count++
		at = end
	}

	return count
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

	stream := &LogStream{Finished: finished, Previous: alloc.PreviousAllocation, cancel: cancel, all: finished}

	// Where a running log ends is read before the stream opens, so that
	// what the task writes from then on is told from what was there.
	if !finished {
		stream.end, stream.endKnown = c.logEnd(ctx, namespace, alloc, task, source)
	}

	frames, errs := c.api.AllocFS().Logs(alloc, !finished, task, source, "end", logTail, cancel, c.query(context.Background(), namespace))

	stream.Err = errs
	stream.Lines = streamLines(frames, startsCut, nil, stream, cancel)

	return stream, nil
}

// logEnd reads where a log ends now. A log that is empty, or one that cannot
// be read, has no end known.
func (c *Client) logEnd(ctx context.Context, namespace string, alloc *api.Allocation, task, source string) (LogMark, bool) {
	cancel := make(chan struct{})
	defer close(cancel)

	frames, errs := c.api.AllocFS().Logs(alloc, false, task, source, "end", 1, cancel, c.query(ctx, namespace))

	var (
		end   LogMark
		known bool
	)

	for {
		select {
		case frame, open := <-frames:
			if !open {
				return end, known
			}

			if frame != nil && len(frame.Data) > 0 {
				end, known = LogMark{File: frame.File, Offset: frame.Offset}, true
			}

		case <-errs:
			go drain(frames)

			return LogMark{}, false
		}
	}
}

// streamLines passes on what the frames hold, from the first whole line when
// the first frame says the read began in the middle of one. Frames are not
// aligned to lines, so it sends only whole lines: the end of a frame waits for
// the rest of its line. A line still being written shows when it ends, when
// the stream ends, or after logTail bytes. atEnd, when not nil, says a frame
// reached the end of what was there when the stream began, and what is held
// is sent after it, once: a file does not end, so its last line gets no break.
// stream, when not nil, gets the mark of the first byte sent and how much of
// the text was there when it opened, before anything is sent.
func streamLines(frames <-chan *api.StreamFrame, startsCut, atEnd func(frame *api.StreamFrame) bool, stream *LogStream, cancel <-chan struct{}) <-chan string {
	lines := make(chan string)

	go func() {
		defer close(lines)

		send := func(chunk []byte) bool { return sendChunk(lines, chunk, frames, cancel) }

		var (
			whole = wholeLines{first: true, startsCut: startsCut, stream: stream}
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

	// stream, when not nil, gets once where the first byte kept is.
	stream *LogStream

	// next is where the frame after the last one begins.
	next LogMark
}

// add takes a frame and gives back what is whole up to its last line break,
// or all of it when the line held grows past logTail with no break.
func (w *wholeLines) add(frame *api.StreamFrame) []byte {
	if w.first {
		w.first, w.cut = false, w.startsCut(frame)
	}

	front := w.frontOf(frame)
	w.next = LogMark{File: frame.File, Offset: front.Offset + int64(len(frame.Data))}

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

	w.noteStart(LogMark{File: front.File, Offset: front.Offset + int64(len(frame.Data)-len(data))}, data)

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

// frontOf is where the frame begins. The Offset of a frame that merged reads
// is the one of its first read, so a frame in the file of the one before
// begins where that one ended. The first frame of a file holds one read,
// unless it begins at the file's first byte, which the count below 0 clamps to.
func (w *wholeLines) frontOf(frame *api.StreamFrame) LogMark {
	if frame.File == w.next.File {
		return w.next
	}

	return frameStart(frame, len(frame.Data))
}

// noteStart records where the first byte kept is, when there is one in data.
func (w *wholeLines) noteStart(mark LogMark, data []byte) {
	if w.stream == nil || len(data) == 0 || w.stream.start.Load() != nil {
		return
	}

	w.stream.there.Store(w.thereFrom(mark))
	w.stream.start.Store(&mark)
}

// thereFrom is how many bytes of the text, which begins at start, the log
// held when it was opened: the text of the frames read so far ends at w.next.
func (w *wholeLines) thereFrom(start LogMark) int64 {
	s := w.stream

	switch {
	case s.all:
		return math.MaxInt64
	case !s.endKnown:
		return 0
	case s.end.File == start.File:
		return max(s.end.Offset-start.Offset, 0)
	case follows(start.File, s.end.File):
		// The tail's part in the start's file is one read under 64 KiB, so it
		// ends in the frame that holds the start.
		return w.next.Offset - start.Offset + s.end.Offset
	}

	return 0
}

// follows says next is the file a log is turned over into after file: the
// same name with the number one up.
func follows(file, next string) bool {
	at := strings.LastIndexByte(file, '.')
	if at < 0 {
		return false
	}

	n, err := strconv.Atoi(file[at+1:])

	return err == nil && next == file[:at+1]+strconv.Itoa(n+1)
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
	return !frameStart(frame, len(frame.Data)).AtStart()
}

// frameStart is where the last kept bytes of a frame begin in its file, for
// the first frame of a file. The cluster puts a read into the frame of the one
// before while that frame is not sent yet, under the Offset of the first
// read, so the count can come out below 0: the frame then starts at the start
// of the file.
func frameStart(frame *api.StreamFrame, kept int) LogMark {
	return LogMark{File: frame.File, Offset: max(frame.Offset-int64(kept), 0)}
}
