package ui

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// fakeShell records what it was asked to open instead of taking the terminal.
type fakeShell struct {
	opened shellCommand
	err    error
}

func (s *fakeShell) Open(cmd shellCommand) tea.Cmd {
	s.opened = cmd

	return func() tea.Msg {
		return shellDoneMsg{task: cmd.Task, err: s.err}
	}
}

func shellModel(t *testing.T, shell Shell) Model {
	t.Helper()

	client := &fakeClient{
		jobs:   twoJobs(),
		allocs: twoAllocs(),
		logs:   &nomad.LogStream{Lines: make(chan string)},
	}

	m := New(client, Options{Namespace: "production", Version: "v-test", Shell: shell, PollEvery: time.Millisecond})
	m, _ = m.update(sizeMsg())
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(enter())
	m, _ = m.update(allocsMsg(twoAllocs()))
	m, _ = m.update(enter())

	return m
}

func TestShell_OpensInTheTaskUnderTheCursor(t *testing.T) {
	r := require.New(t)

	shell := &fakeShell{}
	m := shellModel(t, shell)

	m, cmd := m.update(key('s'))
	r.NotNil(cmd)

	m = follow(m, cmd, 3)

	// The shell goes into that task of that allocation, in its namespace.
	r.Equal("server", shell.opened.Task)
	r.Equal("af1f37df-7b19-6b1c-da67-5e8f482b5a15", shell.opened.AllocID)
	r.Equal("production", shell.opened.Namespace)

	// Coming back from the shell, the task list is where it was.
	r.IsType(tasksPage{}, m.screen.page)
	r.Contains(plain(m.render()), "Shell in server closed")
}

func TestShell_SaysWhatWentWrong(t *testing.T) {
	r := require.New(t)

	m := shellModel(t, &fakeShell{err: errors.New("task is not running")})

	m, cmd := m.update(key('s'))
	m = follow(m, cmd, 3)

	r.Contains(plain(m.render()), "task is not running")
}

func TestShell_WithoutOne(t *testing.T) {
	r := require.New(t)

	m := shellModel(t, nil)

	m, _ = m.update(key('s'))

	r.Contains(plain(m.render()), "no shell")
}

func TestShell_OpensInTheRegionInUse(t *testing.T) {
	r := require.New(t)

	shell := &fakeShell{}
	m := shellModel(t, shell)

	// The session has moved to another region since urga started.
	m.client.(*fakeClient).region = "us"

	m, cmd := m.update(key('s'))
	follow(m, cmd, 3)

	r.Equal("us", shell.opened.Region)
}

func TestShellRunner_AsksInTheRegionOfTheTask(t *testing.T) {
	r := require.New(t)

	client, err := nomad.New(nomad.Config{Address: "https://nomad.example.com", Region: "eu"})
	r.NoError(err)

	session := shellRunner{client: client}.session(shellCommand{Region: "us", Task: "server"})

	// The runner has the client urga started with, in eu; the shell opens in
	// the region the session uses now.
	r.Equal("us", session.client.Region())
}
