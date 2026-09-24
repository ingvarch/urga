package nomad_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// serverDir is the directory of the task server, as the cluster lists it.
const serverDir = `[
	{"Name": "tmp", "IsDir": true, "Size": 64, "FileMode": "dtrwxrwxrwx", "ModTime": "2026-09-24T17:32:19Z"},
	{"Name": "executor.out", "IsDir": false, "Size": 1143, "FileMode": "-rw-r--r--", "ModTime": "2026-09-24T17:55:00Z"},
	{"Name": "local", "IsDir": true, "Size": 64, "FileMode": "drwxrwxrwx", "ModTime": "2026-09-24T17:32:19Z"},
	{"Name": ".server.stdout.fifo", "IsDir": false, "Size": 0, "FileMode": "prw-------", "ModTime": "2026-09-24T17:54:45Z"}
]`

func TestFiles(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, serverDir)

	files, err := client.Files(context.Background(), "production", "af1f37df", "/server")
	r.NoError(err)

	// A directory of the allocation, asked of its client through the
	// servers, in the namespace of the allocation.
	r.Equal("/v1/client/fs/ls/af1f37df", asked.URL.Path)
	r.Equal("/server", asked.URL.Query().Get("path"))
	r.Equal("production", asked.URL.Query().Get("namespace"))

	// Directories first, then files, each in the order of their names.
	r.Equal([]nomad.File{
		{Name: "local", Dir: true, Size: 64, Mode: "drwxrwxrwx", Modified: time.Date(2026, 9, 24, 17, 32, 19, 0, time.UTC)},
		{Name: "tmp", Dir: true, Size: 64, Mode: "dtrwxrwxrwx", Modified: time.Date(2026, 9, 24, 17, 32, 19, 0, time.UTC)},
		{Name: ".server.stdout.fifo", Mode: "prw-------", Modified: time.Date(2026, 9, 24, 17, 54, 45, 0, time.UTC)},
		{Name: "executor.out", Size: 1143, Mode: "-rw-r--r--", Modified: time.Date(2026, 9, 24, 17, 55, 0, 0, time.UTC)},
	}, files)
}

func TestFile_IsAPipe(t *testing.T) {
	r := require.New(t)

	// A pipe is read by whoever holds its other end: a read of it waits
	// for as long as the task writes nothing.
	r.True(nomad.File{Mode: "prw-------"}.Pipe())
	r.False(nomad.File{Mode: "-rw-r--r--"}.Pipe())
	r.False(nomad.File{Mode: "drwxrwxrwx", Dir: true}.Pipe())
}
