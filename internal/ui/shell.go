package ui

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"

	"github.com/ingvarch/urga/internal/nomad"
)

// shellCommand is what a shell needs to know about.
type shellCommand struct {
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
	row, ok := m.selectedIndex()
	if !ok || m.screen.kind != screenTasks {
		return m, nil
	}

	tasks := m.tasks()
	if row >= len(tasks) {
		return m, nil
	}

	if m.opts.Shell == nil {
		m.err = fmt.Errorf("no shell: urga was started without a terminal")

		return m, nil
	}

	return m, m.opts.Shell.Open(shellCommand{
		Namespace: m.screen.namespace,
		AllocID:   m.screen.allocID,
		Task:      tasks[row].Name,
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
	session := &shellSession{client: s.client, cmd: cmd}

	return tea.Exec(session, func(err error) tea.Msg {
		return shellDoneMsg{task: cmd.Task, err: err}
	})
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

// watchSize tells the task how big the terminal is, now and whenever it
// changes.
func watchSize(ctx context.Context, fd int, sizes chan<- nomad.TerminalSize) {
	defer close(sizes)

	changed := make(chan os.Signal, 1)
	signal.Notify(changed, syscall.SIGWINCH)
	defer signal.Stop(changed)

	send := func() {
		width, height, err := term.GetSize(fd)
		if err != nil {
			return
		}

		select {
		case sizes <- nomad.TerminalSize{Width: width, Height: height}:
		case <-ctx.Done():
		}
	}

	send()

	for {
		select {
		case <-changed:
			send()
		case <-ctx.Done():
			return
		}
	}
}
