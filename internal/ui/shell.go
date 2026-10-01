package ui

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
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
func shell(p tasksPage, e env) (tasksPage, outcome) {
	task, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	return p, then(shellMsg{
		Region:    e.client.Region(),
		Namespace: p.namespace,
		AllocID:   p.allocID,
		Task:      task.Name,
	})
}

// openShell hands the terminal to a shell, or shows an error without one.
func (m Model) openShell(cmd shellCommand) (Model, tea.Cmd) {
	if m.opts.Shell == nil {
		return m.fail(fmt.Errorf("no shell: urga was started without a terminal")), nil
	}

	return m, m.opts.Shell.Open(cmd)
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

// execer runs a command inside a task, with the terminal connected to it.
type execer interface {
	Exec(
		ctx context.Context,
		namespace, allocID, task string,
		command []string,
		stdin io.Reader,
		stdout, stderr io.Writer,
		sizes <-chan nomad.TerminalSize,
	) (int, error)
}

// shellSession is a running shell. Bubble Tea gives it the terminal while it
// runs and takes it back afterwards.
type shellSession struct {
	client execer
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

	// The task reads keys as they are typed, so the terminal goes into raw
	// mode. Its old state is restored afterwards.
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

	_, height, _ := term.GetSize(fd)

	return s.open(ctx, height, sizes)
}

// open starts the shell in the task and connects the terminal to it. The
// terminal is height rows tall, or says nothing about it.
func (s *shellSession) open(ctx context.Context, height int, sizes <-chan nomad.TerminalSize) error {
	// The shell draws on the screen the terminal had before urga, not on
	// the one urga draws on. Nothing clears that screen between two shells:
	// it still shows what the last one left, in another task.
	if _, err := io.WriteString(s.stdout, freshScreen(height)); err != nil {
		return err
	}

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

// freshScreen leaves a screen of this height with nothing on it and the
// cursor at its top. A line feed for every row pushes what the screen shows
// into the scrollback, where it can still be read; erasing it would lose it.
// What the line feeds leave is erased, and so is a screen of unknown height.
func freshScreen(height int) string {
	return strings.Repeat("\n", max(height, 0)) + ansi.CursorHomePosition + ansi.EraseScreenBelow
}

// shellDone shows that the shell closed, or why it could not open.
func (m Model) shellDone(msg shellDoneMsg) Model {
	if msg.err != nil {
		return m.fail(msg.err)
	}

	return m.say(fmt.Sprintf("Shell in %s closed.", msg.task))
}
