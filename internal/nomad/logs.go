package nomad

import "context"

// Log sources a task writes to.
const (
	LogStdout = "stdout"
	LogStderr = "stderr"
)

// LogStream is a task writing. Lines arrive on Lines until the task stops or
// Close is called; Err says why it ended, if it did not end on its own.
type LogStream struct {
	Lines <-chan string
	Err   <-chan error

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

	frames, errs := c.api.AllocFS().Logs(alloc, true, task, source, "end", 0, cancel, c.query(context.Background(), namespace))

	lines := make(chan string)

	go func() {
		defer close(lines)

		for frame := range frames {
			if frame == nil || len(frame.Data) == 0 {
				continue
			}

			select {
			case lines <- string(frame.Data):
			case <-cancel:
				return
			}
		}
	}()

	return &LogStream{Lines: lines, Err: errs, cancel: cancel}, nil
}
