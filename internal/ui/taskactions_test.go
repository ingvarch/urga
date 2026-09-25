package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

func TestTasks_RestartOneTask(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openTasks(t, client)
	r.True(offers(m, "r"))

	m, _ = m.update(key('r'))

	// The task under the cursor, the rest of the allocation keeps running.
	r.Equal(overlayConfirm, m.overlay)
	r.Contains(plain(m.render()), "Really restart task server of the allocation af1f37df?")
	r.Empty(client.restartedTask)

	m, cmd := m.update(key('y'))
	m = drain(m, cmd)

	r.Equal("server", client.restartedTask)
	r.Equal("af1f37df-7b19-6b1c-da67-5e8f482b5a15", client.askedID)
	r.Equal("production", client.askedNamespace)
	r.Contains(plain(m.render()), "Task server restarted.")
}

func TestTasks_OnlyARunningTaskIsRestartedOrSignalled(t *testing.T) {
	r := require.New(t)

	m := openTasks(t, &fakeClient{jobs: twoJobs(), allocs: twoAllocs()})

	// The sidecar is dead: its client has nothing to restart or signal.
	m, _ = m.update(key('j'))

	r.False(offers(m, "r"))
	r.False(offers(m, "x"))
}

func TestTasks_SendASignal(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openTasks(t, client)
	r.True(offers(m, "x"))

	m, _ = m.update(key('x'))

	// The line comes up with the signal a task most often reloads on.
	r.Equal(overlaySignal, m.overlay)
	r.True(m.overlay.asksForALine())
	r.Contains(plain(m.render()), "signal server with:")
	r.Equal("SIGHUP", m.prompt.text)

	// Named the short way and in small letters, it is still the signal.
	m, _ = m.update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = typeIn(m, "usr1")
	m, _ = m.update(enter())

	r.Equal(overlayConfirm, m.overlay)
	r.Contains(plain(m.render()), "Really send SIGUSR1 to task server of the allocation af1f37df?")
	r.Empty(client.signal)

	m, cmd := m.update(key('y'))
	m = drain(m, cmd)

	r.Equal("SIGUSR1", client.signal)
	r.Equal("server", client.signalledTask)
	r.Equal("af1f37df-7b19-6b1c-da67-5e8f482b5a15", client.askedID)
	r.Contains(plain(m.render()), "Sent SIGUSR1 to task server.")
}

func TestTasks_RefuseWhatIsNotASignal(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openTasks(t, client)

	m, _ = m.update(key('x'))
	m, _ = m.update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = typeIn(m, "hup now")

	m, cmd := m.update(enter())
	m = drain(m, cmd)

	r.NotEqual(overlayConfirm, m.overlay)
	r.Empty(client.signal)
	r.Contains(plain(m.render()), `"hup now" is not a signal`)
}

func TestSignalName(t *testing.T) {
	r := require.New(t)

	// What is typed may come with spaces around it.
	for _, c := range []struct{ typed, name string }{
		{"SIGHUP", "SIGHUP"},
		{"hup", "SIGHUP"},
		{" sigterm ", "SIGTERM"},
		{"usr1", "SIGUSR1"},
		{"winch", "SIGWINCH"},
	} {
		got, err := signalName(c.typed)
		r.NoError(err, c.typed)
		r.Equal(c.name, got, c.typed)
	}

	// A client sends SIGINT for a name it does not know, which stops most
	// tasks: a typo must not reach it.
	for _, typed := range []string{"", "  ", "hup now", "9", "foo", "sighupp", "SIGRTMIN+1"} {
		_, err := signalName(typed)
		r.Error(err, typed)
	}
}
