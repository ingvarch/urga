package ui

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
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

// fakeTerminal is the screen of a terminal, as much of one as a shell needs: it
// prints, feeds lines, homes the cursor and erases below it. A line fed off
// the bottom pushes the top one into the scrollback, the way a terminal keeps
// what scrolls away.
type fakeTerminal struct {
	rows       [][]rune
	row, col   int
	scrollback []string
}

func newTerminal(height int) *fakeTerminal { return &fakeTerminal{rows: make([][]rune, height)} }

func (s *fakeTerminal) Write(p []byte) (int, error) {
	for text := string(p); len(text) > 0; {
		switch {
		case strings.HasPrefix(text, ansi.CursorHomePosition):
			s.row, s.col = 0, 0
			text = text[len(ansi.CursorHomePosition):]

		case strings.HasPrefix(text, ansi.EraseScreenBelow):
			s.rows[s.row] = s.rows[s.row][:min(s.col, len(s.rows[s.row]))]
			for row := s.row + 1; row < len(s.rows); row++ {
				s.rows[row] = nil
			}

			text = text[len(ansi.EraseScreenBelow):]

		case text[0] == '\n':
			s.feed()
			text = text[1:]

		case text[0] == '\r':
			s.col = 0
			text = text[1:]

		default:
			mark, size := utf8.DecodeRuneInString(text)
			s.put(mark)
			text = text[size:]
		}
	}

	return len(p), nil
}

// feed moves the cursor a line down, and scrolls the screen when it is on
// the last one.
func (s *fakeTerminal) feed() {
	if s.row < len(s.rows)-1 {
		s.row++

		return
	}

	s.scrollback = append(s.scrollback, string(s.rows[0]))
	s.rows = append(s.rows[1:], nil)
}

func (s *fakeTerminal) put(mark rune) {
	for len(s.rows[s.row]) <= s.col {
		s.rows[s.row] = append(s.rows[s.row], ' ')
	}

	s.rows[s.row][s.col] = mark
	s.col++
}

// lines are the rows the screen shows.
func (s *fakeTerminal) lines() []string {
	out := make([]string, 0, len(s.rows))
	for _, row := range s.rows {
		out = append(out, string(row))
	}

	return out
}

// terminalAfter is a screen that already shows these lines, with the cursor
// under them: what an earlier shell left behind.
func terminalAfter(height int, lines ...string) *fakeTerminal {
	s := newTerminal(height)
	for _, line := range lines {
		_, _ = io.WriteString(s, line+"\r\n")
	}

	return s
}

// fakeExec is a task that prints what its shell would, without running one.
type fakeExec struct {
	says string
}

func (f fakeExec) Exec(
	_ context.Context, _, _, _ string, _ []string,
	_ io.Reader, stdout, _ io.Writer, _ <-chan nomad.TerminalSize,
) (int, error) {
	_, err := io.WriteString(stdout, f.says)

	return 0, err
}

// shellOn opens a shell that draws on a screen.
func shellOn(t *testing.T, on *fakeTerminal) {
	t.Helper()

	session := &shellSession{client: fakeExec{says: "new$ "}}
	session.SetStdout(on)

	require.NoError(t, session.open(context.Background(), len(on.rows), nil))
}

func TestShell_OpensOnACleanScreen(t *testing.T) {
	r := require.New(t)

	// The terminal still shows the shell of another task.
	term := terminalAfter(6, "host$ urga", "old$ echo first", "first", "old$ exit")

	shellOn(t, term)

	// The new shell has the screen to itself, from the top.
	r.Equal([]string{"new$ ", "", "", "", "", ""}, term.lines())
}

func TestShell_KeepsWhatTheScreenHeldInTheScrollback(t *testing.T) {
	r := require.New(t)

	term := terminalAfter(6, "host$ urga", "old$ echo first", "first", "old$ exit")

	shellOn(t, term)

	// Nothing the terminal showed is lost: it is a scroll up away, down to
	// the empty line the cursor was on.
	r.Equal([]string{"host$ urga", "old$ echo first", "first", "old$ exit", ""}, term.scrollback)
}

func TestShell_KeepsAFullScreenInTheScrollback(t *testing.T) {
	r := require.New(t)

	// More lines than the screen is tall: the first has scrolled away, and
	// the cursor is on the last row.
	term := terminalAfter(3, "one", "two", "three")

	shellOn(t, term)

	r.Equal([]string{"new$ ", "", ""}, term.lines())
	r.Equal([]string{"one", "two", "three", ""}, term.scrollback)
}

func TestShell_ErasesWhatIsBelowTheCursor(t *testing.T) {
	r := require.New(t)

	// A program that left the cursor at the top of what it drew.
	term := terminalAfter(4, "left", "by", "a program")
	_, _ = io.WriteString(term, ansi.CursorHomePosition)

	shellOn(t, term)

	r.Equal([]string{"new$ ", "", "", ""}, term.lines())
}

func TestShell_ErasesAScreenOfUnknownHeight(t *testing.T) {
	r := require.New(t)

	term := terminalAfter(6, "old$ echo first", "first")

	session := &shellSession{client: fakeExec{says: "new$ "}}
	session.SetStdout(term)

	// A terminal that does not say how tall it is cannot be scrolled by
	// the right amount. The shell still gets a clean screen.
	r.NoError(session.open(context.Background(), 0, nil))
	r.Equal([]string{"new$ ", "", "", "", "", ""}, term.lines())
}

func TestShellRunner_AsksInTheRegionOfTheTask(t *testing.T) {
	r := require.New(t)

	client, err := nomad.New(nomad.Config{Address: "https://nomad.example.com", Region: "eu"})
	r.NoError(err)

	session := shellRunner{client: client}.session(shellCommand{Region: "us", Task: "server"})

	// The runner has the client urga started with, in eu; the shell opens in
	// the region the session uses now.
	r.Equal("us", session.client.(*nomad.Client).Region())
}
