package nomad

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/nomad/api"
)

// File is one entry of a directory of an allocation.
type File struct {
	Name     string
	Dir      bool
	Size     int64
	Mode     string
	Modified time.Time
}

// Pipe says the file is a named pipe, like the ones a task writes its logs
// through. It is read by whoever holds its other end: a read of it waits for
// as long as the task writes nothing.
func (f File) Pipe() bool {
	return strings.HasPrefix(f.Mode, "p")
}

// Files lists a directory of an allocation: its directories first, then its
// files, each in the order of their names. The client that runs it answers
// through the servers.
func (c *Client) Files(ctx context.Context, namespace, allocID, path string) ([]File, error) {
	infos, _, err := c.api.AllocFS().List(&api.Allocation{ID: allocID}, path, c.query(ctx, namespace))
	if err != nil {
		return nil, err
	}

	files := make([]File, 0, len(infos))
	for _, info := range infos {
		files = append(files, File{
			Name:     info.Name,
			Dir:      info.IsDir,
			Size:     info.Size,
			Mode:     info.FileMode,
			Modified: info.ModTime,
		})
	}

	sort.SliceStable(files, func(i, j int) bool {
		if files[i].Dir != files[j].Dir {
			return files[i].Dir
		}

		return files[i].Name < files[j].Name
	})

	return files, nil
}

// fileTail is how much of a big file is read: its last MiB.
const fileTail = 1 << 20

// ErrNotText says a file is not text. Shown as text it would be noise.
var ErrNotText = errors.New("not text")

// File follows a file of an allocation as it grows: from its start, or when
// it is big, from its last MiB. The caller closes the stream when it stops
// reading, otherwise the request stays open. A failure arrives on its Err after
// the lines before it and before Lines closes. Do not call it for a pipe: the
// client reads its first bytes to detect the content type, and waits.
func (c *Client) File(ctx context.Context, namespace, allocID, name string) (*LogStream, error) {
	// The stream is requested from the node that runs the allocation when it
	// can be reached, and from the servers when it cannot.
	alloc, _, err := c.api.Allocations().Info(allocID, c.query(ctx, namespace))
	if err != nil {
		return nil, err
	}

	info, _, err := c.api.AllocFS().Stat(alloc, name, c.query(ctx, namespace))
	if err != nil {
		return nil, err
	}

	if !strings.HasPrefix(info.ContentType, "text/") {
		return nil, fmt.Errorf("%s is %w (%s)", path.Base(name), ErrNotText, info.ContentType)
	}

	from := max(info.Size-fileTail, 0)
	cancel := make(chan struct{})

	frames, errs := c.api.AllocFS().Stream(alloc, name, "start", from, cancel, c.query(context.Background(), namespace))

	// An empty file has no end to reach: all of it is read by whole lines.
	var atEnd func(*api.StreamFrame) bool
	if info.Size > 0 {
		atEnd = func(frame *api.StreamFrame) bool { return frame.Offset >= info.Size }
	}

	lines := make(chan string)
	out := make(chan error, 1)

	go func() {
		defer close(lines)

		followFrames(frames, errs, func(*api.StreamFrame) bool { return from > 0 }, atEnd, lines, out, cancel)
	}()

	return &LogStream{Lines: lines, Err: out, Size: info.Size, From: from, cancel: cancel}, nil
}

// followFrames sends the lines of the frames, then the first failure. The
// failure goes out after the lines and before they close (the caller closes
// them when this returns), so a reader that sees the end finds it.
func followFrames(frames <-chan *api.StreamFrame, failed <-chan error, startsCut, atEnd func(*api.StreamFrame) bool, lines chan<- string, errs chan<- error, cancel <-chan struct{}) {
	if frames != nil {
		for chunk := range streamLines(frames, startsCut, atEnd, nil, cancel) {
			if !sendText(lines, chunk, cancel) {
				return
			}
		}
	}

	passFailures(failed, errs, cancel)
}

// passFailures sends the first error of errs to out, unless it is an end. The
// stream of a file reports its end as an error, the stream of a log does not.
func passFailures(errs <-chan error, out chan<- error, cancel <-chan struct{}) {
	select {
	case err := <-errs:
		if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
			out <- err
		}

	case <-cancel:
	}
}
