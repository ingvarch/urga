package nomad_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
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

// streamed is a frame of a file as the cluster sends it.
func streamed(t *testing.T, offset int64, data string) string {
	t.Helper()

	out, err := json.Marshal(frame{Data: []byte(data), File: "t/local/app.env", Offset: offset})
	require.NoError(t, err)

	return string(out)
}

// allocInfo is the allocation af1f37df, on a node the test cannot reach:
// its files are read through the servers.
const allocInfo = `{"ID": "af1f37df", "Namespace": "production", "NodeID": "n1"}`

// sentTo is the request sent to a path.
func sentTo(t *testing.T, asked []sent, path string) sent {
	t.Helper()

	for _, req := range asked {
		if req.path == path {
			return req
		}
	}

	t.Fatalf("nothing asked of %s", path)

	return sent{}
}

// read is every line a stream sends until it ends.
func read(stream *nomad.LogStream) string {
	var out strings.Builder
	for line := range stream.Lines {
		out.WriteString(line)
	}

	return out.String()
}

func TestFile_ReadFromItsStart(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{
		"/v1/allocation/af1f37df":       allocInfo,
		"/v1/client/fs/stat/af1f37df":   `{"Name": "app.env", "Size": 29, "FileMode": "-rw-r--r--", "ContentType": "text/plain; charset=utf-8"}`,
		"/v1/client/fs/stream/af1f37df": streamed(t, 29, "DB_HOST=10.0.0.5\nMODE=fast\n"),
	})

	stream, err := client.File(context.Background(), "production", "af1f37df", "/t/local/app.env")
	r.NoError(err)
	t.Cleanup(stream.Close)

	r.Equal("DB_HOST=10.0.0.5\nMODE=fast\n", read(stream))
	r.Equal(int64(29), stream.Size)
	r.Zero(stream.From)

	// A stream the cluster closed has ended, it has not failed.
	select {
	case err := <-stream.Err:
		r.NoError(err)
	case <-time.After(100 * time.Millisecond):
	}

	// Asked for in its namespace, followed from its first byte.
	stat := sentTo(t, *asked, "/v1/client/fs/stat/af1f37df")
	r.Equal("/t/local/app.env", stat.query.Get("path"))
	r.Equal("production", stat.namespace)

	follow := sentTo(t, *asked, "/v1/client/fs/stream/af1f37df")
	r.Equal("/t/local/app.env", follow.query.Get("path"))
	r.Equal("start", follow.query.Get("origin"))
	r.Equal("0", follow.query.Get("offset"))
	r.Equal("production", follow.namespace)

	// The node it runs on is requested by its ID, never by an empty one.
	for _, req := range *asked {
		r.NotEqual("/v1/node/", req.path)
	}
}

// openFile opens a file of the given size whose stream sends the frames at
// their offsets and then stays open until the test ends.
func openFile(t *testing.T, size int64, frames map[int64]string) *nomad.LogStream {
	t.Helper()

	held := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch req.URL.Path {
		case "/v1/allocation/af1f37df":
			_, _ = w.Write([]byte(allocInfo))
		case "/v1/client/fs/stat/af1f37df":
			_, _ = fmt.Fprintf(w, `{"Name": "app.env", "Size": %d, "FileMode": "-rw-r--r--", "ContentType": "text/plain; charset=utf-8"}`, size)
		case "/v1/client/fs/stream/af1f37df":
			offsets := make([]int64, 0, len(frames))
			for offset := range frames {
				offsets = append(offsets, offset)
			}

			slices.Sort(offsets)

			for _, offset := range offsets {
				_, _ = w.Write([]byte(streamed(t, offset, frames[offset])))
			}

			w.(http.Flusher).Flush()

			// Not the request context: the client does not close the body
			// when it cancels.
			<-held
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(held) })

	client, err := nomad.New(nomad.Config{Address: server.URL})
	require.NoError(t, err)

	stream, err := client.File(context.Background(), "production", "af1f37df", "/t/local/app.env")
	require.NoError(t, err)
	t.Cleanup(stream.Close)

	return stream
}

// next is the next chunk of a stream, or a failure when none shows.
func next(t *testing.T, stream *nomad.LogStream) string {
	t.Helper()

	select {
	case line := <-stream.Lines:
		return line
	case <-time.After(2 * time.Second):
		t.Fatal("no line showed while the stream was open")
	}

	return ""
}

func TestFile_SendTheLastLineOfAFileWithNoBreakWhileTheStreamStaysOpen(t *testing.T) {
	stream := openFile(t, 7, map[int64]string{7: "a=1\nb=2"})

	require.Equal(t, "a=1\nb=2", next(t, stream))
}

func TestFile_JoinALineThatGrewAfterItWasOpened(t *testing.T) {
	stream := openFile(t, 4, map[int64]string{4: "a=1\n", 12: "first li", 15: "ne\n"})

	require.Equal(t, "a=1\n", next(t, stream))
	require.Equal(t, "first line\n", next(t, stream))
}

func TestFile_JoinALineOfAFileThatWasEmpty(t *testing.T) {
	stream := openFile(t, 0, map[int64]string{8: "first li", 14: "ne\nsec", 18: "ond\n"})

	require.Equal(t, "first line\nsecond\n", next(t, stream)+next(t, stream))
}

func TestFile_OnlyTheEndOfABigOne(t *testing.T) {
	r := require.New(t)

	size := int64(3 << 20)
	from := size - 1<<20

	client, asked := clusterServer(t, map[string]string{
		"/v1/allocation/af1f37df":       allocInfo,
		"/v1/client/fs/stat/af1f37df":   fmt.Sprintf(`{"Name": "big.txt", "Size": %d, "ContentType": "text/plain; charset=utf-8"}`, size),
		"/v1/client/fs/stream/af1f37df": streamed(t, from+25, "the rest of a line\nnext\n"),
	})

	stream, err := client.File(context.Background(), "production", "af1f37df", "/t/local/big.txt")
	r.NoError(err)
	t.Cleanup(stream.Close)

	// The last MiB of it, from the first whole line in it.
	r.Equal(fmt.Sprint(from), sentTo(t, *asked, "/v1/client/fs/stream/af1f37df").query.Get("offset"))
	r.Equal("next\n", read(stream))
	r.Equal(from, stream.From)
	r.Equal(size, stream.Size)
}

func TestFile_WhatIsNotText(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{
		"/v1/allocation/af1f37df":     allocInfo,
		"/v1/client/fs/stat/af1f37df": `{"Name": "blob.bin", "Size": 4000, "ContentType": "application/octet-stream"}`,
	})

	_, err := client.File(context.Background(), "production", "af1f37df", "/t/local/blob.bin")

	// Shown as text it would be noise: it is not read at all.
	r.ErrorIs(err, nomad.ErrNotText)
	r.ErrorContains(err, "blob.bin")
	r.ErrorContains(err, "application/octet-stream")

	for _, req := range *asked {
		r.NotEqual("/v1/client/fs/stream/af1f37df", req.path)
	}
}
