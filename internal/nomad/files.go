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
// reading, otherwise the request stays open. Do not call it for a pipe: the
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

	lines := streamLines(frames, func(*api.StreamFrame) bool { return from > 0 }, cancel)

	return &LogStream{Lines: lines, Err: failures(errs, cancel), Size: info.Size, From: from, cancel: cancel}, nil
}

// failures passes on the errors of a stream. Its end is not a failure: the
// stream of a file reports it as an error, the stream of a log does not.
func failures(errs <-chan error, cancel <-chan struct{}) <-chan error {
	out := make(chan error, 1)

	go func() {
		select {
		case err := <-errs:
			if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
				out <- err
			}

		case <-cancel:
		}
	}()

	return out
}
