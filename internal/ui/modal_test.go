package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

// keyRune is a letter key, spelled out so the button tests read plainly.
func keyRune(r rune) tea.KeyPressMsg { return key(r) }

func confirmingModel(t *testing.T) Model {
	t.Helper()

	jobs := manyJobs(12)

	m := newTestModel(&fakeClient{jobs: jobs})
	m, _ = m.update(jobsMsg(jobs))
	m, _ = m.update(ctrlKey('s'))

	return m
}

func TestModal_FloatsOverTheList(t *testing.T) {
	r := require.New(t)

	m := confirmingModel(t)
	rows := lines(m.render())

	// The question is on the screen.
	out := strings.Join(rows, "\n")
	r.Contains(out, "Really stop the job job-00?")
	r.Contains(out, "cancel")
	r.Contains(out, "confirm")

	// And the list is still behind it: a question about one row is not a
	// reason to hide the others.
	r.Contains(out, "job-00")
	r.Contains(out, "job-11")
	r.Contains(out, "Jobs (production) [12]")

	// Nothing moved: every line is as wide as it was.
	for i, row := range rows {
		r.LessOrEqual(ansi.StringWidth(row), 120, "line %d", i)
	}
}

func TestModal_IsABoxNotAPanel(t *testing.T) {
	r := require.New(t)

	m := confirmingModel(t)
	rows := lines(m.render())

	// The dialog has its own border somewhere in the middle of the screen,
	// with the list showing on both sides of it.
	var top string

	for _, row := range rows {
		if strings.Contains(row, "Confirm") && strings.Contains(row, "╭") {
			top = row
		}
	}

	r.NotEmpty(top, "the dialog has a top border")

	at := strings.Index(top, "╭")
	r.Greater(at, 4, "it starts away from the left edge")
	r.Less(ansi.StringWidth(top[at:]), 120-8, "and ends away from the right one")
}

func TestModal_ClosesAndLeavesTheListAlone(t *testing.T) {
	r := require.New(t)

	m := confirmingModel(t)

	m, _ = m.update(escape())

	out := plain(m.render())
	r.NotContains(out, "Really stop")
	r.Contains(out, "job-00")
}

func TestModal_ButtonsAreChosenWithTheCursor(t *testing.T) {
	r := require.New(t)

	m := confirmingModel(t)

	out := plain(m.render())
	r.Contains(out, "cancel")
	r.Contains(out, "confirm")

	// Cancel is where the cursor starts: the safe one, so that enter by
	// habit changes nothing in the cluster.
	r.Equal(0, m.confirm.choice)

	raw := m.render()
	r.Contains(raw, styleButtonOn.Render(" cancel "))
	r.Contains(raw, styleButton.Render(" confirm "))

	// Right, tab or l walks to the other one, left walks back.
	for _, key := range []tea.KeyPressMsg{
		{Code: tea.KeyRight},
		{Code: tea.KeyTab},
		keyRune('l'),
	} {
		next, _ := m.update(key)
		r.Equal(1, next.confirm.choice, key.String())
	}

	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyRight})
	r.Contains(m.render(), styleButtonOn.Render(" confirm "))

	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyLeft})
	r.Equal(0, m.confirm.choice)
}

func TestModal_EnterTakesTheButtonUnderTheCursor(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: manyJobs(3)}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(client.jobs))
	m, _ = m.update(ctrlKey('s'))

	// Enter on cancel closes the question and leaves the cluster alone.
	m, cmd := m.update(enter())
	r.Equal(overlayNone, m.overlay)
	r.Nil(cmd)
	r.Zero(client.stopped)

	// Enter on confirm does the thing.
	m, _ = m.update(ctrlKey('s'))
	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyRight})

	_, cmd = m.update(enter())
	r.NotNil(cmd)
	drain(m, cmd)

	r.Equal(1, client.stopped)
}

func TestModal_EscapeAlwaysCancels(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: manyJobs(3)}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(client.jobs))
	m, _ = m.update(ctrlKey('s'))
	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyRight})

	// Even with the cursor on confirm, escape is a way out.
	m, cmd := m.update(escape())

	r.Equal(overlayNone, m.overlay)
	r.Nil(cmd)
	r.Zero(client.stopped)
}

// dialogTop is the line the dialog starts on, and the line the cursor row is
// on, both counted from the top of the screen.
func dialogTop(m Model) (dialog, cursor int) {
	dialog, cursor = -1, -1

	for i, line := range lines(m.render()) {
		if strings.Contains(line, "Confirm") && strings.Contains(line, "╭") {
			dialog = i
		}

		// The name is in the question too, the row is the first one.
		if cursor == -1 && strings.Contains(line, "job-00") {
			cursor = i
		}
	}

	return dialog, cursor
}

func TestModal_OpensNextToTheRowItAsksAbout(t *testing.T) {
	r := require.New(t)

	jobs := manyJobs(5)

	m := newTestModel(&fakeClient{jobs: jobs})
	m, _ = m.update(tea.WindowSizeMsg{Width: 120, Height: 60})
	m, _ = m.update(jobsMsg(jobs))
	m, _ = m.update(ctrlKey('s'))

	dialog, cursor := dialogTop(m)
	r.NotEqual(-1, dialog)
	r.NotEqual(-1, cursor)

	// On a tall window the question belongs next to the row it is about, not
	// in the middle of the empty space below it.
	r.Greater(dialog, cursor, "the dialog opens under the row")
	r.Less(dialog-cursor, 3, "and right under it")
}

func TestModal_MovesAboveTheRowWhenThereIsNoRoomBelow(t *testing.T) {
	r := require.New(t)

	jobs := manyJobs(20)

	m := newTestModel(&fakeClient{jobs: jobs})
	m, _ = m.update(tea.WindowSizeMsg{Width: 120, Height: 20})
	m, _ = m.update(jobsMsg(jobs))

	// The cursor is on the last row that fits.
	m, _ = m.update(key('G'))
	m, _ = m.update(ctrlKey('s'))

	rows := lines(m.render())

	var top, bottom int

	for i, line := range rows {
		if strings.Contains(line, "Confirm") && strings.Contains(line, "╭") {
			top = i
		}

		if strings.Contains(line, "╰") && strings.Contains(line, "─╯") {
			bottom = i
		}
	}

	r.NotZero(top)

	// The whole dialog is inside the box, it does not hang off the bottom.
	r.Less(bottom, len(rows)-1)
	r.Greater(top, 0)
}
