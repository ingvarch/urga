package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// ranCommand asks for a command on the task under the cursor and enters it.
func ranCommand(m Model, line string) (Model, tea.Cmd) {
	m, _ = m.update(ctrl('x'))
	m = typeIn(m, line)

	return m.update(enter())
}

func TestTaskRun_RunTheCommandAndOpenWhatItPrinted(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), output: nomad.CommandOutput{Text: "nameserver 10.0.0.2\n", Size: 20}}
	m := openTasks(t, client)

	m, _ = m.update(ctrl('x'))

	r.Equal(overlayAnswer, m.overlay)
	r.Contains(plain(m.render()), "run in server: ")
	r.Empty(m.prompt.text)

	m = typeIn(m, "cat /etc/resolv.conf")
	m, cmd := m.update(enter())

	// No question first: the command was typed and entered.
	r.NotEqual(overlayConfirm, m.overlay)

	m = follow(m, cmd, 2)

	r.Equal([]string{"/bin/sh", "-c", "cat /etc/resolv.conf"}, client.ranCommand)
	r.Equal("server", client.ranTask)
	r.Equal(frontendAlloc, client.askedID)
	r.Equal("production", client.askedNamespace)

	r.IsType(describePage{}, m.screen.page)
	r.Equal("Output (Task: server, Allocation: af1f37df) [exit 0]: cat /etc/resolv.conf", m.title())
	r.Contains(plain(m.render()), "nameserver 10.0.0.2")
}

func TestTaskRun_TheSpacesAroundTheCommandAreDropped(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), output: nomad.CommandOutput{Text: "PATH=/bin\n", Size: 10}}
	m := openTasks(t, client)

	m, _ = m.update(ctrl('x'))
	m = typeIn(m, "  env  ")
	m, cmd := m.update(enter())
	m = follow(m, cmd, 2)

	r.Equal([]string{"/bin/sh", "-c", "env"}, client.ranCommand)
	r.Equal("Output (Task: server, Allocation: af1f37df) [exit 0]: env", m.title())
}

func TestTaskRun_TheExitCodeIsInTheTitleAndTheTextStillOpens(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), output: nomad.CommandOutput{Text: "no such file\n", ExitCode: 2}}
	m := openTasks(t, client)

	m, cmd := ranCommand(m, "cat /nope")
	m = follow(m, cmd, 2)

	r.Contains(plain(m.render()), "Output (Task: server, Allocation: af1f37df) [exit 2]: cat /nope")
	r.Contains(plain(m.render()), "no such file")
}

func TestTaskRun_TheTitleSaysWhenOnlyTheEndOfALongOutputIsKept(t *testing.T) {
	r := require.New(t)

	output := nomad.CommandOutput{Text: "y\ny\n", Size: 37 << 20, From: 36 << 20}
	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), output: output}
	m := openTasks(t, client)

	m, cmd := ranCommand(m, "yes")
	m = follow(m, cmd, 2)

	r.Equal("Output (Task: server, Allocation: af1f37df) [exit 0, last 1.0 MiB of 37.0 MiB]: yes", m.title())
}

func TestTaskRun_ALongCommandDoesNotHideTheExitCode(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), output: nomad.CommandOutput{Text: "done\n", ExitCode: 3}}
	m := openTasks(t, client)
	m, _ = m.update(tea.WindowSizeMsg{Width: 80, Height: 30})

	command := "echo " + strings.Repeat("x", 55)
	r.Len(command, 60)

	m, cmd := ranCommand(m, command)
	m = follow(m, cmd, 2)

	// A title too long for the border is cut from its end, so the command
	// goes last.
	r.Contains(outputTitle(m), "[exit 3]")
}

func TestTaskRun_MovedSidewaysTheTitleShowsTheColumnBeforeTheCommand(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), output: nomad.CommandOutput{Text: longLine()}}
	m := openTasks(t, client)
	m, _ = m.update(tea.WindowSizeMsg{Width: 80, Height: 30})

	m, cmd := ranCommand(m, "cat /etc/resolv.conf")
	m = follow(m, cmd, 2)
	m = right(m)

	column := sprintf("from column %d", m.text.shift+1)

	r.Equal("Output (Task: server, Allocation: af1f37df) [exit 0], "+column+": cat /etc/resolv.conf", m.title())
	r.Contains(outputTitle(m), column)
}

// outputTitle is the line of the screen that holds the title of an output.
func outputTitle(m Model) string {
	title := ""

	for _, line := range strings.Split(plain(m.render()), "\n") {
		if strings.Contains(line, "Output") {
			title = line
		}
	}

	return title
}

func TestTaskRun_CopyTheOutput(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), output: nomad.CommandOutput{Text: "A=1\nB=2\n"}}
	m := openTasks(t, client)

	m, cmd := ranCommand(m, "env")
	m = follow(m, cmd, 2)

	_, text, ok := clipped(m)

	r.True(ok)
	r.Equal("A=1\nB=2", text)
}

func TestTaskRun_SaveTheOutputOfALongCommand(t *testing.T) {
	r := require.New(t)

	dir := t.TempDir()
	t.Chdir(dir)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), output: nomad.CommandOutput{Text: "done\n"}}
	m := openTasks(t, client)

	m, cmd := ranCommand(m, "echo "+strings.Repeat("x", 300))
	m = follow(m, cmd, 2)

	m, _ = m.update(ctrlKey('s'))
	m, cmd = m.update(enter())
	m = drain(m, cmd)

	files, err := os.ReadDir(dir)
	r.NoError(err)
	r.Len(files, 1, m.flash.text)

	r.Contains(files[0].Name(), "-echo-xxx")

	saved, err := os.ReadFile(filepath.Join(dir, files[0].Name()))
	r.NoError(err)
	r.Equal("done", strings.TrimSpace(string(saved)))
}

func TestTaskRun_ABlankLineRunsNothing(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openTasks(t, client)

	m, cmd := ranCommand(m, "   ")

	r.Nil(cmd)
	r.Empty(client.writes)
	r.Equal(flashWarn, m.flash.level)
	r.Contains(plain(m.render()), "No command to run.")
}

func TestTaskRun_ACommandThatDoesNotEndShowsTheLimit(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), actionErr: context.DeadlineExceeded}
	m := openTasks(t, client)

	m, cmd := ranCommand(m, "sleep 60")
	m = follow(m, cmd, 2)

	r.Contains(plain(m.render()), "sleep 60 did not end within 10s")
	r.IsType(tasksPage{}, m.screen.page)
}

func TestTaskRun_AnotherErrorShowsAsItIs(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), actionErr: context.Canceled}
	m := openTasks(t, client)

	m, cmd := ranCommand(m, "env")
	m = follow(m, cmd, 2)

	r.Contains(plain(m.render()), "context canceled")
}

func TestTaskRun_OnlyARunningTaskRunsACommand(t *testing.T) {
	r := require.New(t)

	m := openTasks(t, &fakeClient{jobs: twoJobs(), allocs: twoAllocs()})
	r.True(offers(m, "ctrl-x"))

	// The sidecar is dead.
	m, _ = m.update(key('j'))

	r.False(offers(m, "ctrl-x"))
}

func TestTaskRun_OnTheTasksOfTheNamespaceItRunsInTheAllocationOfTheRow(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{allocs: staged(), output: nomad.CommandOutput{Text: "ok\n"}}
	m := onNamespaceTasks(t, client, nomad.AllNamespaces)

	m, cmd := ranCommand(m, "env")
	follow(m, cmd, 2)

	r.Equal("server", client.ranTask)
	r.Equal(frontendAlloc, client.askedID)
	r.Equal("staging", client.askedNamespace)
}

func TestPrintedText(t *testing.T) {
	r := require.New(t)

	// Colours stay, other escape sequences and control characters go, and
	// the last newline does not make an empty line.
	r.Equal("\x1b[31mred\x1b[0m\nplain\n", printedText("\x1b[31mred\x1b[0m\x00\nplain\x07\n\n"))
	r.Equal("a   b", printedText("a\tb\n"))
	r.Equal("", printedText(""))
}
