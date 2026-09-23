package ui

import (
	"context"
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"

	"github.com/ingvarch/urga/internal/nomad"
)

// shellCommand is what a shell needs to know about. The region travels with
// it: the session may have moved to another one since urga started.
type shellCommand struct {
	Region    string
	Namespace string
	AllocID   string
	Task      string
}

// Shell runs a shell inside a task, with the terminal handed over to it.
type Shell interface {
	Open(cmd shellCommand) tea.Cmd
}

// shellDoneMsg is what came of a shell session.
type shellDoneMsg struct {
	task string
	err  error
}

// shell opens a shell in the task under the cursor.
func (m Model) shell() (Model, tea.Cmd) {
	task, ok := selectedOf(m, screenTasks, m.tasks())
	if !ok {
		return m, nil
	}

	if m.opts.Shell == nil {
		return m.fail(fmt.Errorf("no shell: urga was started without a terminal")), nil
	}

	return m, m.opts.Shell.Open(shellCommand{
		Region:    m.client.Region(),
		Namespace: m.screen.namespace,
		AllocID:   m.screen.allocID,
		Task:      task.Name,
	})
}

// shellRunner opens the session against the cluster and gives it the
// terminal.
type shellRunner struct {
	client *nomad.Client
}

// NewShell runs shells against a cluster.
func NewShell(client *nomad.Client) Shell { return shellRunner{client: client} }

func (s shellRunner) Open(cmd shellCommand) tea.Cmd {
	return tea.Exec(s.session(cmd), func(err error) tea.Msg {
		return shellDoneMsg{task: cmd.Task, err: err}
	})
}

// session is a shell in the region of the task.
func (s shellRunner) session(cmd shellCommand) *shellSession {
	return &shellSession{client: s.client.InRegion(cmd.Region), cmd: cmd}
}

// shellSession is a running shell. Bubble Tea gives it the terminal while it
// runs and takes it back afterwards.
type shellSession struct {
	client *nomad.Client
	cmd    shellCommand

	stdin          io.Reader
	stdout, stderr io.Writer
}

func (s *shellSession) SetStdin(r io.Reader)  { s.stdin = r }
func (s *shellSession) SetStdout(w io.Writer) { s.stdout = w }
func (s *shellSession) SetStderr(w io.Writer) { s.stderr = w }

func (s *shellSession) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The task reads keys as they are typed, so the terminal stops cooking
	// them. It is given back the way it was found.
	fd := int(os.Stdin.Fd())

	if term.IsTerminal(fd) {
		state, err := term.MakeRaw(fd)
		if err != nil {
			return err
		}

		defer func() { _ = term.Restore(fd, state) }()
	}

	sizes := make(chan nomad.TerminalSize, 1)
	go watchSize(ctx, fd, sizes)

	code, err := s.client.Exec(ctx, s.cmd.Namespace, s.cmd.AllocID, s.cmd.Task,
		[]string{"/bin/sh", "-c", "command -v bash >/dev/null && exec bash || exec sh"},
		s.stdin, s.stdout, s.stderr, sizes)
	if err != nil {
		return err
	}

	if code != 0 {
		return fmt.Errorf("the shell ended with %d", code)
	}

	return nil
}
