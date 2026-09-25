package ui

import (
	"fmt"
	"os"
	"path/filepath"
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

	rows := lines(plain(m.list.table.view()))
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
	r.Contains(plain(m.render()), "Files (Allocation: af1f37df, /server/local)")
	r.Equal("/server/local", client.filesPath)
	r.Equal("production", client.filesNamespace)
	r.Contains(plain(m.render()), "app.env")

	// Escape comes back to where it was.
	m, _ = m.update(escape())
	r.Contains(plain(m.render()), "Files (Allocation: af1f37df, /server)")
}

func TestFiles_UpToTheAllocation(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := browsed(t, client)

	m, cmd := m.update(enter())
	m = drain(m, cmd)

	// The allocation holds what its tasks share, and nothing is above it.
	r.Contains(plain(m.render()), "Files (Allocation: af1f37df, /)")
	r.Contains(fileRow(t, m, 0), "alloc/")
	r.NotContains(plain(m.render()), "..")
}

func TestFiles_NoRowsUntilTheDirectoryIsListed(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), files: allocFiles()}
	m := openTasks(t, client)

	// Not even the row that goes up: nothing of a directory shows before
	// it is listed.
	m, _ = m.update(key('b'))
	r.Contains(plain(m.render()), "Files (Allocation: af1f37df, /server) [0]")
}

func TestFiles_TheHeaderOffersToOpen(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := browsed(t, client)

	// Enter opens what the cursor is on, a directory or a file; a
	// directory has no other key.
	r.Equal([]hint{{Key: "<enter>", Description: "Open"}}, m.hints())
}

func TestFiles_ADirectoryIsAskedAgain(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := browsed(t, client)

	client.files["/server"] = append(client.files["/server"], nomad.File{Name: "late.log", Size: 7})
	client.filesPath = ""

	m, cmd := m.update(pollMsg{})
	m = drain(m, cmd)

	r.Equal("/server", client.filesPath)
	r.Contains(plain(m.render()), "late.log")
}

func TestFiles_StayInTheNamespaceOfTheAllocation(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := browsed(t, client)
	m.namespaceOrder = []string{"production", "staging"}

	m, cmd := m.update(key('2'))
	m = drain(m, cmd)

	// The allocation lives in production, whatever the session looks at.
	r.Equal("staging", m.namespace)
	r.Equal("production", client.filesNamespace)
	r.Contains(plain(m.render()), "Files (Allocation: af1f37df, /server)")
}

func TestFiles_TheListingOfADirectoryLeftIsDropped(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := browsed(t, client)

	m, _ = m.update(key('j'))
	m, listing := m.update(enter())
	m, _ = m.update(escape())

	// What local holds arrives after it was left: it is not what the
	// directory above holds.
	m = drain(m, listing)

	out := plain(m.render())
	r.Contains(out, "Files (Allocation: af1f37df, /server)")
	r.NotContains(out, "app.env")
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
	r.Contains(plain(m.render()), "Files (Allocation: af1f37df, /server)")
	r.NotContains(plain(m.render()), "app.env")
}

// written is a stream that has said what it holds and ended.
func written(text string) *nomad.LogStream {
	lines := make(chan string, 1)
	lines <- text
	close(lines)

	return &nomad.LogStream{Lines: lines, Err: make(chan error)}
}

// onEnv is the directory local of the task server, with the cursor on
// app.env.
func onEnv(t *testing.T, client *fakeClient) Model {
	t.Helper()

	m := browsed(t, client)

	m, _ = m.update(key('j'))
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	// Past the row that goes up.
	m, _ = m.update(key('j'))

	return m
}

// openedEnv is app.env of the task server, open on its screen and read to
// the end of what it says.
func openedEnv(t *testing.T, client *fakeClient) Model {
	t.Helper()

	m, cmd := onEnv(t, client).update(enter())

	return follow(m, cmd, 4)
}

// readingEnv is app.env open on its screen on a stream that stays open:
// what it says is up to the test.
func readingEnv(t *testing.T, client *fakeClient, stream *nomad.LogStream) Model {
	t.Helper()

	client.file = stream

	m, cmd := onEnv(t, client).update(enter())

	return drain(m, cmd)
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
}

func TestFile_EscapeLetsGoOfIt(t *testing.T) {
	r := require.New(t)

	closed := false
	stream := &nomad.LogStream{Lines: make(chan string), Err: make(chan error), OnClose: func() { closed = true }}

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := readingEnv(t, client, stream)
	r.Equal(screenFile, m.screen.kind)

	// A file that is still being read is let go of.
	m, _ = m.update(escape())
	r.Equal(screenFiles, m.screen.kind)
	r.True(closed)
}

// closing is a stream that stays open, and says whether it was closed.
func closing() (*nomad.LogStream, *bool) {
	closed := false

	return &nomad.LogStream{Lines: make(chan string), Err: make(chan error), OnClose: func() { closed = true }}, &closed
}

func TestFile_CoveredLetsGoOfIt(t *testing.T) {
	r := require.New(t)

	stream, closed := closing()

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := readingEnv(t, client, stream)

	// Nothing reads it under another screen, and escape reads it again.
	m, _ = m.show(screenJobs)
	r.Equal(screenJobs, m.screen.kind)
	r.True(*closed)
}

func TestFile_OneStreamPerFile(t *testing.T) {
	r := require.New(t)

	first, _ := closing()
	second, closed := closing()

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := onEnv(t, client)

	// The file is asked for twice before either answer arrives.
	m, once := m.update(enter())
	m, twice := m.update(enter())

	client.file = first
	m = drain(m, once)
	client.file = second
	m = drain(m, twice)

	r.Equal(screenFile, m.screen.kind)
	r.True(*closed, "a second stream of the same file is left open")

	m, _ = m.update(logLineMsg{stream: first, text: "DB_HOST=10.0.0.5\n"})
	r.Contains(plain(m.render()), "DB_HOST=10.0.0.5")
}

func TestFile_AnotherFileAskedMeanwhileIsLetGo(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := onEnv(t, client)

	client.files["/server/local"] = append(client.files["/server/local"], nomad.File{Name: "other.env", Size: 8})
	m, listed := m.update(pollMsg{})
	m = drain(m, listed)

	// Both files are asked for, one after the other, before either answers.
	m, first := m.update(enter())
	m, _ = m.update(key('j'))
	m, second := m.update(enter())

	client.file = written("DB_HOST=10.0.0.5\n")
	m = follow(m, first, 4)

	other, closed := closing()
	client.file = other
	m = drain(m, second)

	// The screen is app.env's, read to its end: the other file is not
	// read into it.
	r.True(*closed)
	r.Contains(plain(m.render()), "File (Allocation: af1f37df) [/server/local/app.env]")
}

func TestFile_ANamespaceSwitchLeavesItAsItIs(t *testing.T) {
	r := require.New(t)

	stream, closed := closing()

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := readingEnv(t, client, stream)
	m.namespaceOrder = []string{"production", "staging"}
	m, _ = m.update(logLineMsg{stream: stream, text: "DB_HOST=10.0.0.5\n"})

	// The file belongs to its allocation: the namespace the lists look at
	// changes nothing about it, and it is not read again.
	// Played out with a deadline per command: a file opened again waits on
	// its stream for good.
	m, cmd := m.update(key('2'))
	m = playOut(m, cmd)

	r.Equal("staging", m.namespace)
	r.False(*closed)
	r.Equal(1, client.fileCalls)
	r.Contains(plain(m.render()), "DB_HOST=10.0.0.5")
}

func TestFile_ReadAgainAfterLeavingIsLetGo(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), file: written("DB_HOST=10.0.0.5\n")}
	m := openedEnv(t, client)

	m, _ = m.show(screenJobs)

	// Back on the file, and gone again before it is read.
	stream, closed := closing()
	client.file = stream

	m, read := m.update(escape())
	m, _ = m.update(escape())
	m = drain(m, read)

	r.True(*closed)
	r.Equal(screenFiles, m.screen.kind)
}

func TestFile_WhatItGrowsByStaysBelowTheTop(t *testing.T) {
	r := require.New(t)

	stream := &nomad.LogStream{Lines: make(chan string), Err: make(chan error)}

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := readingEnv(t, client, stream)

	m, _ = m.update(logLineMsg{stream: stream, text: "first\n"})

	for range 60 {
		m, _ = m.update(logLineMsg{stream: stream, text: "more\n"})
	}

	// Autoscroll is off: the top of the file stays where it was.
	r.Contains(plain(m.render()), "first")

	m, _ = m.update(key('s'))
	r.Contains(plain(m.render()), "Autoscroll:On")
	r.NotContains(plain(m.render()), "first")

	// Followed, what it grows by next is followed too.
	for i := range 30 {
		m, _ = m.update(logLineMsg{stream: stream, text: fmt.Sprintf("later-%02d\n", i)})
	}

	r.Contains(plain(m.render()), "later-29")
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
	r.Contains(plain(m.render()), "Files (Allocation: af1f37df, /server)")
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

func TestFile_SaveWhatIsOnTheScreen(t *testing.T) {
	r := require.New(t)

	dir := t.TempDir()
	t.Chdir(dir)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), file: written("DB_HOST=10.0.0.5\n")}
	m := openedEnv(t, client)

	m, cmd := m.update(ctrlKey('s'))
	m = drain(m, cmd)

	// Named after the file, not after the screen it was read on.
	files, err := filepath.Glob(filepath.Join(dir, "app.env-*.txt"))
	r.NoError(err)
	r.Len(files, 1)

	saved, err := os.ReadFile(files[0])
	r.NoError(err)
	r.Equal("DB_HOST=10.0.0.5", string(saved))
	r.Contains(plain(m.render()), filepath.Base(files[0]))
}

func TestFile_Wraps(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), file: written(strings.Repeat("x", 400) + "\n")}
	m := openedEnv(t, client)
	r.Equal(1, rowsWith(m, "xxx"))

	m, _ = m.update(key('w'))

	r.Contains(plain(m.render()), "Wrap:On")
	r.Greater(rowsWith(m, "xxx"), 3)
}

func TestSaveName_OfAFile(t *testing.T) {
	r := require.New(t)

	name := saveName(fileScreen(filePage{path: "/server/local/app.env"}))

	r.True(strings.HasPrefix(name, "app.env-"), name)
	r.True(strings.HasSuffix(name, ".txt"), name)
}
