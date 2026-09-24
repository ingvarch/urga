package ui

import (
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
