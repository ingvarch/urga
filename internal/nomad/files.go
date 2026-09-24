package nomad

import (
	"context"
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
