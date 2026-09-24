package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// allocFiles are the directories of an allocation of served, by path.
func allocFiles() map[string][]nomad.File {
	then := time.Now().Add(-12 * time.Minute)

	return map[string][]nomad.File{
		"/": {
			{Name: "alloc", Dir: true, Modified: then},
			{Name: "server", Dir: true, Modified: then},
		},
		"/server": {
			{Name: "local", Dir: true, Size: 64, Mode: "drwxrwxrwx", Modified: then},
			{Name: "secrets", Dir: true, Size: 64, Mode: "drwxrwxrwx", Modified: then},
			{Name: "executor.out", Size: 1143, Mode: "-rw-r--r--", Modified: then},
		},
		"/server/local": {
			{Name: "app.env", Size: 42, Mode: "-rw-r--r--", Modified: then},
		},
	}
}

// fileRow is a row of the list as it reads; the header comes before it.
func fileRow(t *testing.T, m Model, i int) string {
	t.Helper()

	rows := lines(plain(m.table.view()))
	require.Greater(t, len(rows), i+1)

	return rows[i+1]
}

// browsed is the directory of the task server, opened from its tasks.
func browsed(t *testing.T, client *fakeClient) Model {
	t.Helper()

	client.files = allocFiles()

	m := openTasks(t, client)
	m, cmd := m.update(key('b'))

	return drain(m, cmd)
}

func TestTasks_BrowseTheDirectoryOfATask(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := browsed(t, client)

	// The directory of the task under the cursor, asked of its allocation
	// in its namespace.
	r.Equal(screenFiles, m.screen.kind)
	r.Equal("/server", client.filesPath)
	r.Equal("af1f37df-7b19-6b1c-da67-5e8f482b5a15", client.filesAllocID)
	r.Equal("production", client.filesNamespace)

	out := plain(m.render())
	r.Contains(out, "Files (Allocation: af1f37df, /server)")

	// Up first, then directories, then files, with how big each file is.
	r.Contains(fileRow(t, m, 0), "..")
	r.Contains(fileRow(t, m, 1), "local/")
	r.Contains(fileRow(t, m, 2), "secrets/")
	r.Contains(fileRow(t, m, 3), "executor.out")
	r.Contains(fileRow(t, m, 3), "1.1 KiB")
	r.Contains(fileRow(t, m, 3), "12m")
}

func TestFiles_EnterGoesIntoADirectory(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := browsed(t, client)

	m, _ = m.update(key('j'))
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	r.Equal(screenFiles, m.screen.kind)
	r.Equal("/server/local", m.screen.path)
	r.Equal("/server/local", client.filesPath)
	r.Contains(plain(m.render()), "app.env")

	// Escape comes back to where it was.
	m, _ = m.update(escape())
	r.Equal("/server", m.screen.path)
}

func TestFiles_UpToTheAllocation(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := browsed(t, client)

	m, cmd := m.update(enter())
	m = drain(m, cmd)

	// The allocation holds what its tasks share, and nothing is above it.
	r.Equal("/", m.screen.path)
	r.Contains(fileRow(t, m, 0), "alloc/")
	r.NotContains(plain(m.render()), "..")
}

func TestSizeOf(t *testing.T) {
	r := require.New(t)

	r.Equal("0 B", sizeOf(0))
	r.Equal("512 B", sizeOf(512))
	r.Equal("1.1 KiB", sizeOf(1143))
	r.Equal("5.2 MiB", sizeOf(5452595))
	r.Equal("2.0 GiB", sizeOf(2<<30))
}

func TestFiles_ADirectoryShowsOnlyItsOwnRows(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := browsed(t, client)

	m, _ = m.update(key('j'))
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	// Back up, before the cluster has listed it again: what was listed of
	// the directory below is not what this one holds.
	m, _ = m.update(escape())
	r.Equal("/server", m.screen.path)
	r.NotContains(plain(m.render()), "app.env")
}

// written is a stream that has said what it holds and ended.
func written(text string) *nomad.LogStream {
	lines := make(chan string, 1)
	lines <- text
	close(lines)

	return &nomad.LogStream{Lines: lines, Err: make(chan error)}
}

// openedEnv is app.env of the task server, open on its screen.
func openedEnv(t *testing.T, client *fakeClient) Model {
	t.Helper()

	m := browsed(t, client)

	m, _ = m.update(key('j'))
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	// Past the row that goes up.
	m, _ = m.update(key('j'))
	m, cmd = m.update(enter())

	return follow(m, cmd, 4)
}

func TestFiles_OpenAFile(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), file: written("DB_HOST=10.0.0.5\nMODE=fast\n")}
	m := openedEnv(t, client)

	// Asked of its allocation in its namespace, read like a log.
	r.Equal(screenFile, m.screen.kind)
	r.Equal("/server/local/app.env", client.filePath)
	r.Equal("af1f37df-7b19-6b1c-da67-5e8f482b5a15", client.fileAllocID)
	r.Equal("production", client.fileNamespace)

	out := plain(m.render())
	r.Contains(out, "File (Allocation: af1f37df) [/server/local/app.env]")
	r.Contains(out, "DB_HOST=10.0.0.5")
	r.Contains(out, "MODE=fast")

	// A file is read from the top, and when urga read a line is nothing
	// to it.
	r.Contains(out, "Autoscroll:Off")
	r.Contains(out, "Wrap:Off")
	r.NotContains(out, "Timestamps")
	r.True(offers(m, "s"))
	r.True(offers(m, "w"))
	r.True(offers(m, "ctrl-s"))
	r.False(offers(m, "t"))

	// Escape lets go of a file that is still being read.
	closed := false
	m.logs.stream = &nomad.LogStream{Lines: make(chan string), OnClose: func() { closed = true }}

	m, _ = m.update(escape())
	r.Equal(screenFiles, m.screen.kind)
	r.True(closed)
}

func TestFile_WhatItGrowsByStaysBelowTheTop(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), file: written("first\n")}
	m := openedEnv(t, client)

	stream := &nomad.LogStream{Lines: make(chan string), Err: make(chan error)}
	m.logs.stream = stream

	for range 60 {
		m, _ = m.update(logLineMsg{stream: stream, text: "more\n"})
	}

	// Autoscroll is off: the top of the file stays where it was.
	r.Contains(plain(m.render()), "first")

	m, _ = m.update(key('s'))
	r.Contains(plain(m.render()), "Autoscroll:On")
	r.NotContains(plain(m.render()), "first")
}

func TestFile_OnlyTheEndOfABigOne(t *testing.T) {
	r := require.New(t)

	big := written("line 2000000\n")
	big.Size, big.From = 3<<20, 2<<20

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), file: big}
	m := openedEnv(t, client)

	r.Contains(plain(m.render()), "[/server/local/app.env, last 1.0 MiB of 3.0 MiB]")
}

func TestFiles_APipeIsNotOpened(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := browsed(t, client)

	client.files["/server"] = append(client.files["/server"], nomad.File{Name: ".server.stdout.fifo", Mode: "prw-------"})
	m, cmd := m.update(escape())
	m = drain(m, cmd)
	m, cmd = m.update(key('b'))
	m = drain(m, cmd)

	m, _ = m.update(key('G'))
	m, cmd = m.update(enter())
	m = drain(m, cmd)

	// Asked what it holds, its client would wait on it for as long as the
	// task writes nothing.
	r.Zero(client.fileCalls)
	r.Equal(screenFiles, m.screen.kind)
	r.Contains(plain(m.render()), ".server.stdout.fifo is a pipe")
}

func TestFiles_WhatIsNotTextIsNotOpened(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		jobs: twoJobs(), allocs: twoAllocs(),
		fileErr: fmt.Errorf("blob.bin is %w (application/octet-stream)", nomad.ErrNotText),
	}
	m := openedEnv(t, client)

	r.Equal(screenFiles, m.screen.kind)
	r.Contains(plain(m.render()), "blob.bin is not text (application/octet-stream)")
}

func TestFiles_AFileOpenedAfterLeavingIsLetGo(t *testing.T) {
	r := require.New(t)

	stream := written("DB_HOST=10.0.0.5\n")
	closed := false
	stream.OnClose = func() { closed = true }

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), file: stream}
	m := browsed(t, client)

	m, _ = m.update(key('j'))
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	m, _ = m.update(key('j'))
	m, open := m.update(enter())

	// Gone back up before the file answered: nothing would ever close it.
	m, _ = m.update(escape())
	m = drain(m, open)

	r.Equal(screenFiles, m.screen.kind)
	r.Equal("/server", m.screen.path)
	r.True(closed)
}

func TestFile_ComingBackReadsItAgain(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), file: written("DB_HOST=10.0.0.5\n")}
	m := openedEnv(t, client)

	m, _ = m.show(screenJobs)
	client.file = written("DB_HOST=10.0.0.6\n")

	m, cmd := m.update(escape())
	m = follow(m, cmd, 6)

	r.Equal(screenFile, m.screen.kind)
	r.Equal(2, client.fileCalls)
	r.Contains(plain(m.render()), "DB_HOST=10.0.0.6")
	r.NotContains(plain(m.render()), "DB_HOST=10.0.0.5")
}

func TestSaveName_OfAFile(t *testing.T) {
	r := require.New(t)

	name := saveName(screen{kind: screenFile, path: "/server/local/app.env"})

	r.True(strings.HasPrefix(name, "app.env-"), name)
	r.True(strings.HasSuffix(name, ".txt"), name)
}
