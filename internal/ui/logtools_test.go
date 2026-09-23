package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// rowsWith is how many rows of the screen say it.
func rowsWith(m Model, text string) int {
	count := 0

	for _, line := range strings.Split(plain(m.render()), "\n") {
		if strings.Contains(line, text) {
			count++
		}
	}

	return count
}

// onLogs opens a log screen with a few lines already in it.
func onLogs(t *testing.T, lines ...string) (Model, *fakeClient) {
	t.Helper()

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), logs: &nomad.LogStream{Lines: make(chan string)}}

	m := openTasks(t, client)
	m, _ = m.update(enter())

	for _, line := range lines {
		m, _ = m.update(logLineMsg(line))
	}

	return m, client
}

func TestLogs_TheFilterShowsWhereItMatched(t *testing.T) {
	r := require.New(t)

	m, _ := onLogs(t, "ready to serve\n", "connection refused\n", "serving on 8080\n")

	m, _ = m.update(key('/'))
	m = typeIn(m, "serv")
	m, _ = m.update(enter())

	out := m.render()

	// Only the lines that say it are left, and the word itself stands out
	// of them.
	r.Contains(plain(out), "ready to serve")
	r.NotContains(plain(out), "connection refused")

	r.Contains(out, styleMatch.Render("serv"))
}

func TestLogs_WrapLongLines(t *testing.T) {
	r := require.New(t)

	long := strings.Repeat("a", 400)

	m, _ := onLogs(t, long+"\n")

	// Cut to the width of the screen, a long line says nothing of its tail.
	r.Equal(1, rowsWith(m, "aaa"))

	m, _ = m.update(key('w'))

	// Wrapped, the whole line is there, over as many rows as it takes.
	r.Greater(rowsWith(m, "aaa"), 3)
}

func TestLogs_SayWhenALineArrived(t *testing.T) {
	r := require.New(t)

	m, _ := onLogs(t, "ready to serve\n")

	m, _ = m.update(key('t'))

	out := plain(m.render())

	// What a task writes carries no time of its own, so the time is when
	// urga read it, and only for the lines it watched arrive.
	r.Regexp(`\d\d:\d\d:\d\d  ready to serve`, out)
}

func TestLogs_SaveWhatIsOnTheScreen(t *testing.T) {
	r := require.New(t)

	dir := t.TempDir()
	t.Chdir(dir)

	m, _ := onLogs(t, "ready to serve\n", "connection refused\n")

	m, cmd := m.update(ctrlKey('s'))
	m = drain(m, cmd)

	files, err := filepath.Glob(filepath.Join(dir, "*.log"))
	r.NoError(err)
	r.Len(files, 1)

	saved, err := os.ReadFile(files[0])
	r.NoError(err)
	r.Contains(string(saved), "ready to serve")
	r.Contains(string(saved), "connection refused")

	// The screen says where it went.
	r.Contains(plain(m.render()), filepath.Base(files[0]))
}

func TestLogs_SaveADescriptionToo(t *testing.T) {
	r := require.New(t)

	dir := t.TempDir()
	t.Chdir(dir)

	client := &fakeClient{jobs: twoJobs(), describe: `{"ID": "web"}`}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, cmd := m.update(key('d'))
	m = drain(m, cmd)

	m, cmd = m.update(ctrlKey('s'))
	drain(m, cmd)

	files, err := filepath.Glob(filepath.Join(dir, "*.txt"))
	r.NoError(err)
	r.Len(files, 1)
}
