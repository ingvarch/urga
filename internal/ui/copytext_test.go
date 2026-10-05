package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// clipped is what pressing c puts on the clipboard, and the model after it.
func clipped(m Model) (Model, string, bool) {
	m, cmd := m.update(key('c'))
	if cmd == nil {
		return m, "", false
	}

	return m, clipboardOf(cmd), true
}

func TestCopyText_CopiesWhatTheScreenShows(t *testing.T) {
	r := require.New(t)

	m, _ := onLogs(t, "ready to serve\n", "connection refused\n")

	m, text, ok := clipped(m)

	r.True(ok)
	r.Equal("ready to serve\nconnection refused", text)
	r.Contains(plain(m.render()), "Copied 2 lines.")
}

func TestCopyText_CopiesWhatTheFilterLeft(t *testing.T) {
	r := require.New(t)

	m, _ := onLogs(t, "ready to serve\n", "connection refused\n")

	m, _ = m.update(key('/'))
	m = typeIn(m, "refused")
	m, _ = m.update(enter())

	m, text, ok := clipped(m)

	r.True(ok)
	r.Equal("connection refused", text)
	r.Contains(plain(m.render()), "Copied 1 line.")
}

func numbered(count int) []string {
	lines := make([]string, count)

	for i := range lines {
		lines[i] = fmt.Sprintf("%0*d\n", 99, i)
	}

	return lines
}

func TestCopyText_AnOverlongTextIsCutToItsTail(t *testing.T) {
	r := require.New(t)

	m, _ := onLogs(t)

	total := 3 * copyLimit / 100
	fed := numbered(total)
	m = withLines(m, fed)

	m, text, ok := clipped(m)

	r.True(ok)
	r.LessOrEqual(len(text), copyLimit)

	// Whole lines only, the last ones, and every one that fits.
	got := strings.Split(text, "\n")
	r.Len(got, 655)
	r.Equal(strings.TrimSuffix(fed[total-1], "\n"), got[len(got)-1])
	r.Equal(strings.TrimSuffix(fed[total-len(got)], "\n"), got[0])
	r.Greater(len(text)+1+len(strings.TrimSuffix(fed[total-len(got)-1], "\n")), copyLimit)
	r.Contains(plain(m.render()), fmt.Sprintf("Copied last 655 of %d lines.", total))
}

func TestCopyText_ATextOfExactlyTheLimitIsCopiedWhole(t *testing.T) {
	r := require.New(t)

	m, _ := onLogs(t)
	m = withLines(m, []string{strings.Repeat("a", copyLimit/2-1) + "\n", strings.Repeat("b", copyLimit/2) + "\n"})

	m, text, ok := clipped(m)

	r.True(ok)
	r.Len(text, copyLimit)
	r.Contains(plain(m.render()), "Copied 2 lines.")
}

func TestCopyText_DoesNothingOffATextScreen(t *testing.T) {
	r := require.New(t)

	m := openTasks(t, &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), logs: &nomad.LogStream{Lines: make(chan string)}})
	m, cmd := m.update(copyTextMsg{})
	r.Nil(cmd)
	r.NotContains(plain(m.render()), "Nothing to copy.")

	// Back from a log: its text is left behind, and must not be copied.
	m, cmd = m.update(enter())
	m = drain(m, cmd)
	m = withLines(m, []string{"left over\n"})
	m, _ = m.update(escape())

	m, cmd = m.update(copyTextMsg{})
	r.Nil(cmd)
	r.NotContains(plain(m.render()), "Copied")
}

func TestCopyText_ALastLineOverTheLimitCopiesNothing(t *testing.T) {
	r := require.New(t)

	m, _ := onLogs(t)
	m = withLines(m, []string{"short\n", strings.Repeat("x", copyLimit+1) + "\n"})

	m, _, ok := clipped(m)

	r.False(ok)
	r.Contains(plain(m.render()), "too long")
}

func TestCopyText_NothingToCopy(t *testing.T) {
	r := require.New(t)

	m, _ := onLogs(t)

	m, _, ok := clipped(m)

	r.False(ok)
	r.Contains(plain(m.render()), "Nothing to copy.")
}

func TestCopyText_EveryTextScreenOffersIt(t *testing.T) {
	logs, _ := onLogs(t, "a\n")
	jobLogs, _ := oneTask(t)

	describe := newTestModel(&fakeClient{jobs: twoJobs(), describe: "one\ntwo"})
	describe, _ = describe.update(jobsMsg(twoJobs()))
	describe, cmd := describe.update(key('d'))
	describe = drain(describe, cmd)

	file := openedEnv(t, &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), file: written("DB_HOST=1\n")})
	plan := planned(t, &fakeClient{jobs: twoJobs(), plan: changedEnv()})

	about := aboutOf(t, released(readsReleases(threeReleases()[2:], nil, new(int))))

	screens := map[string]Model{
		"logs": logs, "job logs": jobLogs, "describe": describe,
		"file": file, "plan": plan, "about": about,
	}

	for name, m := range screens {
		require.True(t, m.readsAsText(), name)
		require.True(t, offersLabel(m, "c", "Copy"), name)
	}
}
