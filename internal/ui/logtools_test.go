package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// rowsWith is how many rows of the screen contain the text.
func rowsWith(m Model, text string) int {
	count := 0

	for _, line := range strings.Split(plain(m.render()), "\n") {
		if strings.Contains(line, text) {
			count++
		}
	}

	return count
}

// lineOf is what the stream of the open log hands over.
func lineOf(m Model, text string) tea.Msg {
	return logLineMsg{stream: logOf(m).stream, text: text}
}

// onLogs opens a log screen with a few lines already in it.
func onLogs(t *testing.T, lines ...string) (Model, *fakeClient) {
	t.Helper()

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), logs: &nomad.LogStream{Lines: make(chan string)}}

	m := openTasks(t, client)
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	for _, line := range lines {
		m, _ = m.update(lineOf(m, line))
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

	// Only the lines that contain it are left, and the match is highlighted
	// in them.
	r.Contains(plain(out), "ready to serve")
	r.NotContains(plain(out), "connection refused")

	r.Contains(out, styleMatch.Render("serv"))
}

func TestLogs_WrapLongLines(t *testing.T) {
	r := require.New(t)

	long := strings.Repeat("a", 400)

	m, _ := onLogs(t, long+"\n")

	// Cut to the width of the screen, a long line hides its tail.
	r.Equal(1, rowsWith(m, "aaa"))

	m, _ = m.update(key('w'))

	// Wrapped, the whole line is there, over as many rows as it takes.
	r.Greater(rowsWith(m, "aaa"), 3)
}

// longLines are lines that each wrap over three rows of the screen, and carry
// their number at both ends.
func longLines(count int) []string {
	lines := make([]string, 0, count)
	for i := range count {
		lines = append(lines, fmt.Sprintf("line-%03d %s end-%03d", i, strings.Repeat("x", 300), i))
	}

	return lines
}

func TestLogs_TheEndsAreAKeyAway(t *testing.T) {
	r := require.New(t)

	m, _ := onLogs(t, strings.Join(longLines(40), "\n")+"\n")

	// Followed, the window keeps to the last line.
	out := plain(m.render())
	r.Contains(out, "line-039")
	r.NotContains(out, "line-000")

	m, _ = m.update(key('g'))
	r.Contains(plain(m.render()), "line-000")

	m, _ = m.update(key('G'))
	r.Contains(plain(m.render()), "line-039")

	// Going to the top stopped autoscroll; turning it on goes to the end.
	m, _ = m.update(key('g'))
	m, _ = m.update(key('s'))
	r.Contains(plain(m.render()), "line-039")
}

func TestLogs_FollowingReachesTheLastWrappedRow(t *testing.T) {
	r := require.New(t)

	m, _ := onLogs(t)
	m, _ = m.update(key('w'))

	// Wrapped, a line takes several rows, and the end is the last of them.
	m, _ = m.update(lineOf(m, strings.Join(longLines(40), "\n")+"\n"))
	r.Contains(plain(m.render()), "end-039")
}

func TestLogs_AutoscrollReachesTheLastWrappedRow(t *testing.T) {
	r := require.New(t)

	m, _ := onLogs(t)
	m, _ = m.update(key('s'))
	m, _ = m.update(key('w'))
	m, _ = m.update(lineOf(m, strings.Join(longLines(40), "\n")+"\n"))
	r.Contains(plain(m.render()), "line-000")

	// Turned back on, autoscroll goes back to the end.
	m, _ = m.update(key('s'))
	r.Contains(plain(m.render()), "end-039")
}

func TestLogs_HomeReachesTheFirstWrappedRow(t *testing.T) {
	r := require.New(t)

	m, _ := onLogs(t)
	m, _ = m.update(key('w'))

	for _, line := range longLines(40) {
		m, _ = m.update(lineOf(m, line+"\n"))
	}

	r.Contains(plain(m.render()), "end-039")

	m, _ = m.update(key('g'))
	r.Contains(plain(m.render()), "line-000")
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

func TestLogs_AChunkIsTheLinesItHolds(t *testing.T) {
	r := require.New(t)

	m, _ := onLogs(t, "ready to serve\nserving on 8080\n")
	m, _ = m.update(key('t'))

	// The newline a chunk ends with starts no line: an empty one would
	// still show the time it arrived.
	r.Len(regexp.MustCompile(`\d\d:\d\d:\d\d`).FindAllString(plain(m.render()), -1), 2)
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

	// The screen shows the file it was saved to.
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

	// The file is named after what was described.
	files, err := filepath.Glob(filepath.Join(dir, "Job-web-*.txt"))
	r.NoError(err)
	r.Len(files, 1)
}

func TestLogs_WrapALineThatCarriesColour(t *testing.T) {
	r := require.New(t)

	// What a service writes is usually coloured. Wrapping must cut it by
	// what it looks like, not by the bytes the colour takes.
	coloured := "\x1b[31m" + strings.Repeat("error ", 40) + "\x1b[0m"

	done := make(chan []string, 1)
	go func() { done <- wrapLine(coloured, 20) }()

	select {
	case rows := <-done:
		r.Greater(len(rows), 1)
		r.Contains(plain(rows[0]), "error")
	case <-time.After(2 * time.Second):
		t.Fatal("wrapping a coloured line did not finish")
	}
}

func TestLogs_WrapALineOfWideCharacters(t *testing.T) {
	r := require.New(t)

	done := make(chan []string, 1)
	go func() { done <- wrapLine("восток東京восток", 1) }()

	select {
	case rows := <-done:
		// A column too narrow for a character still takes it, rather than
		// returning no rows at all.
		r.NotEmpty(rows)
	case <-time.After(2 * time.Second):
		t.Fatal("wrapping a wide line did not finish")
	}
}

func TestLogs_SaveWhatTheFilterLeft(t *testing.T) {
	r := require.New(t)

	dir := t.TempDir()
	t.Chdir(dir)

	m, _ := onLogs(t, "ready to serve\n", "connection refused\n")

	m, _ = m.update(key('/'))
	m = typeIn(m, "refused")
	m, _ = m.update(enter())

	m, cmd := m.update(ctrlKey('s'))
	drain(m, cmd)

	files, _ := filepath.Glob(filepath.Join(dir, "*.log"))
	r.Len(files, 1)

	saved, err := os.ReadFile(files[0])
	r.NoError(err)

	// Narrowing a log down to what matters and keeping that is the point of
	// having both keys on the same screen.
	r.Contains(string(saved), "connection refused")
	r.NotContains(string(saved), "ready to serve")
}

func TestLogs_TimesBelongToTheLogAlone(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), describe: "one\ntwo"}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, cmd := m.update(key('d'))
	m = drain(m, cmd)

	before := plain(m.render())
	m, _ = m.update(key('t'))

	// A description has no times to show, so the key leaves it alone
	// instead of shifting every line to the right.
	r.Equal(before, plain(m.render()))
}

func TestLogs_ASaveNameIsAName(t *testing.T) {
	r := require.New(t)

	// Everything a file name cannot hold is taken out, and what is left
	// does not start with a dash: a file that does is a nuisance for every
	// command that reads it afterwards.
	r.Equal("urga", plainName("///"))
	r.Equal("urga", plainName(""))
	r.Equal("web", plainName("/web/"))
	r.Equal("Job-web", plainName("Job: web"))
}
