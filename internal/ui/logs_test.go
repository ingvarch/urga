package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// openTasks walks down to the task list of the first allocation.
func openTasks(t *testing.T, client *fakeClient) Model {
	t.Helper()

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(enter())
	m, _ = m.update(allocsMsg(twoAllocs()))
	m, _ = m.update(enter())

	return m
}

func TestLogs_OpenOnATask(t *testing.T) {
	r := require.New(t)

	lines := make(chan string, 2)
	lines <- "first line\n"
	lines <- "second line\n"

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), logs: &nomad.LogStream{Lines: lines}}
	m := openTasks(t, client)

	m, cmd := m.update(enter())
	r.Equal(screenLogs, m.screen.kind)
	r.NotNil(cmd)

	// The stream is opened for the task the cursor was on, in the namespace
	// of its allocation.
	m = drain(m, cmd)
	r.Equal("server", client.askedTask)
	r.Equal("af1f37df-7b19-6b1c-da67-5e8f482b5a15", client.askedID)
	r.Equal("production", client.askedNamespace)

	// What the task writes lands on the screen as it comes.
	m = drain(m, m.waitForLog())
	m = drain(m, m.waitForLog())

	out := plain(m.render())
	r.Contains(out, "Logs (Task: server) [stdout]")
	r.Contains(out, "first line")
	r.Contains(out, "second line")
}

func TestLogs_LeavingClosesTheStream(t *testing.T) {
	r := require.New(t)

	stream := &nomad.LogStream{Lines: make(chan string)}
	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), logs: stream}

	m := openTasks(t, client)
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	m, _ = m.update(escape())

	// The request behind the stream is not left hanging.
	r.Equal(screenTasks, m.screen.kind)
	r.True(client.logsClosed)
}

func TestLogs_FollowCanBeStopped(t *testing.T) {
	r := require.New(t)

	lines := make(chan string, 1)
	lines <- "first line\n"

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), logs: &nomad.LogStream{Lines: lines}}
	m := openTasks(t, client)

	m, cmd := m.update(enter())
	m = drain(m, cmd)
	m = drain(m, m.waitForLog())

	r.True(m.following)

	// Stopping leaves the window where it is, resuming jumps back to the end.
	m, _ = m.update(key('s'))
	r.False(m.following)

	m, _ = m.update(key('r'))
	r.True(m.following)
}

func TestLogs_Stderr(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), logs: &nomad.LogStream{Lines: make(chan string)}}
	m := openTasks(t, client)

	m, cmd := m.update(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	m = drain(m, cmd)

	r.Equal(screenLogs, m.screen.kind)
	r.Equal(nomad.LogStderr, client.askedSource)
	r.Contains(plain(m.render()), "[stderr]")
}
