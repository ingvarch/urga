package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/ingvarch/urga/internal/nomad"
	"github.com/stretchr/testify/require"
)

// longLine is 300 cells: the letters in turn, then a marker at the end.
func longLine() string {
	line := make([]byte, 295)
	for i := range line {
		line[i] = byte('a' + i%26)
	}

	return string(line) + "<END>\n"
}

func onLongLine(t *testing.T) Model {
	t.Helper()

	m, _ := onLogs(t, longLine())

	return m
}

func arrow(code rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: code} }

func right(m Model) Model {
	m, _ = m.update(arrow(tea.KeyRight))

	return m
}

func left(m Model) Model {
	m, _ = m.update(arrow(tea.KeyLeft))

	return m
}

// firstRow is the first row of the text on the screen, without colours.
func firstRow(m Model) string {
	first, _, _ := strings.Cut(plain(m.text.view()), "\n")

	return strings.TrimRight(first, " ")
}

func TestSideways_RightMovesByAQuarterOfTheWidth(t *testing.T) {
	r := require.New(t)

	m := onLongLine(t)
	step := m.text.width / 4
	line := strings.TrimSpace(longLine())

	r.True(strings.HasPrefix(firstRow(m), line[:5]))
	r.NotContains(togglesOf(m), "Column")

	m = right(m)
	r.True(strings.HasPrefix(firstRow(m), line[step:step+5]), firstRow(m))
	r.Contains(togglesOf(m), sprintf("Column:%d", step+1))

	m, _ = m.update(key('l'))
	r.True(strings.HasPrefix(firstRow(m), line[2*step:2*step+5]), firstRow(m))
	r.Contains(togglesOf(m), sprintf("Column:%d", 2*step+1))
}

func TestSideways_LeftAndHMoveBackAndStopAtColumnOne(t *testing.T) {
	r := require.New(t)

	m := right(right(onLongLine(t)))

	m = left(m)
	r.Contains(togglesOf(m), sprintf("Column:%d", m.text.width/4+1))

	m, _ = m.update(key('h'))
	r.NotContains(togglesOf(m), "Column")

	m = left(m)
	r.NotContains(togglesOf(m), "Column")
	r.Zero(m.text.shift)
}

func TestSideways_StopsWhereTheWidestLineMeetsTheRightEdge(t *testing.T) {
	r := require.New(t)

	m := onLongLine(t)
	for range 50 {
		m = right(m)
	}

	r.Contains(plain(m.text.view()), "<END>")
	r.Equal(300-m.text.width, m.text.shift)

	shift := m.text.shift
	m = right(m)
	r.Equal(shift, m.text.shift)

	short, _ := onLogs(t, "short\n")
	short = right(short)
	r.Zero(short.text.shift)
	r.NotContains(togglesOf(short), "Column")
}

func TestSideways_AWideCharacterTheLeftEdgeCutsLeavesABlank(t *testing.T) {
	r := require.New(t)

	line := strings.Repeat("日本", 100) + "<END>"

	m, _ := onLogs(t, line+"\n")
	for range 50 {
		m = right(m)
	}

	shift := m.text.shift
	r.Equal(ansi.StringWidth(line)-m.text.width, shift)
	r.Equal(1, shift%2, "the left edge falls inside a character")

	// The half that shows is a blank; the next character is column shift+2.
	r.Equal(" "+string([]rune(line)[(shift+1)/2:]), firstRow(m))
	r.Contains(togglesOf(m), sprintf("Column:%d", shift+1))
}

func TestSideways_HomeGoesBackToTheStartOfTheLinesFirst(t *testing.T) {
	r := require.New(t)

	lines := []string{}
	for range 100 {
		lines = append(lines, longLine())
	}

	m, _ := onLogs(t, lines...)
	m, _ = m.update(key('G'))
	m = right(m)
	top := m.text.top
	r.NotZero(top)

	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyHome})
	r.Zero(m.text.shift)
	r.Equal(top, m.text.top)

	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyHome})
	r.Zero(m.text.top)
}

func TestSideways_GGoesToTheTopAndKeepsTheShift(t *testing.T) {
	r := require.New(t)

	lines := []string{}
	for range 100 {
		lines = append(lines, longLine())
	}

	m, _ := onLogs(t, lines...)
	m, _ = m.update(key('G'))
	m = right(m)
	shift := m.text.shift
	r.NotZero(shift)
	r.NotZero(m.text.top)

	m, _ = m.update(key('g'))
	r.Zero(m.text.top)
	r.Equal(shift, m.text.shift)
}

func TestSideways_WrapPutsItBackAndHoldsIt(t *testing.T) {
	r := require.New(t)

	m := right(onLongLine(t))
	r.NotZero(m.text.shift)

	m, _ = m.update(key('w'))
	r.Zero(m.text.shift)
	r.True(m.text.wrap)

	m = right(m)
	r.Zero(m.text.shift)
	m = left(m)
	r.Zero(m.text.shift)
}

func TestSideways_FindAndFilterLightTheirMatchesWhereTheyAre(t *testing.T) {
	r := require.New(t)

	m := onLongLine(t)
	for range 50 {
		m = right(m)
	}

	found := foundIn(m, "END")
	r.Contains(found.text.view(), styleMatch.Render("END"))

	m, _ = m.update(key('/'))
	m = typeIn(m, "END")
	m, _ = m.update(enter())
	r.Contains(m.text.view(), styleMatch.Render("END"))

	// A match the left edge cuts is lit on the part that shows.
	cut := foundIn(onLongLine(t), "abcdefghij")
	cut.text.shift = 3
	r.Contains(cut.text.view(), styleMatch.Render("defghij"))
}

func TestSideways_ThePlanKeepsLeftAndRightForItsButtons(t *testing.T) {
	r := require.New(t)

	wide := changedEnv()
	wide.Diff = append(wide.Diff, nomad.DiffLine{Kind: nomad.DiffAdded, Indent: 3, Text: longLine()})

	m := planned(t, &fakeClient{jobs: twoJobs(), spec: nomad.JobSource{Source: "job \"web\" {}", Index: 42}, plan: wide})
	r.Greater(m.text.width, 0)

	choice := func(m Model) int { return int(m.screen.page.(planPage).choice) }

	for _, press := range []struct {
		key    tea.KeyPressMsg
		button int
	}{
		{arrow(tea.KeyRight), int(buttonConfirm)},
		{arrow(tea.KeyLeft), int(buttonCancel)},
		{key('l'), int(buttonConfirm)},
		{key('h'), int(buttonCancel)},
	} {
		m, _ = m.update(press.key)
		r.Zero(m.text.shift)
		r.Equal(press.button, choice(m))
	}

	r.NotContains(m.title(), "from column")
}

func TestSideways_ADescriptionMovesToo(t *testing.T) {
	r := require.New(t)

	m := everyScreen(t)["jobs"]
	m, _ = m.update(describeMsg{label: "Job: web", content: longLine()})

	r.Zero(m.toggleRows())

	m = right(m)

	step := m.text.width/4 + 1
	r.Equal(step-1, m.text.shift)
	r.True(strings.HasPrefix(firstRow(m), strings.TrimSpace(longLine())[step-1:step+4]), firstRow(m))
	r.Contains(m.title(), sprintf("from column %d", step))
}

// togglesOf is the line of toggles under the title, without colours.
func togglesOf(m Model) string { return plain(m.logToggles(m.innerWidth())) }

func TestSideways_AStreamSaysTheColumnOnItsTogglesLine(t *testing.T) {
	r := require.New(t)

	m := onEnv(t, &fakeClient{jobs: twoJobs(), allocs: twoAllocs()})
	m, cmd := m.update(fileMsg{
		stream:  written(longLine()),
		allocID: "af1f37df-7b19-6b1c-da67-5e8f482b5a15",
		path:    "/server/local/application.production.settings.properties",
	})
	m = follow(m, cmd, 4)
	r.IsType(filePage{}, m.screen.page)

	m, _ = m.update(tea.WindowSizeMsg{Width: 80, Height: 30})
	r.Greater(ansi.StringWidth(m.title()), 80, "the title is cut on the screen")
	r.NotContains(togglesOf(m), "Column")

	m = right(m)
	column := sprintf("Column:%d", m.text.shift+1)

	r.NotZero(m.text.shift)
	r.Contains(togglesOf(m), column)
	r.Contains(plain(m.render()), column)
	r.NotContains(m.title(), "from column")
}

func TestSideways_SavingAfterMovingSavesTheWholeLine(t *testing.T) {
	r := require.New(t)

	dir := t.TempDir()
	t.Chdir(dir)

	m := right(onLongLine(t))
	r.NotZero(m.text.shift)

	m, cmd := m.update(ctrlKey('s'))
	drain(m, cmd)

	files, err := filepath.Glob(filepath.Join(dir, "*.log"))
	r.NoError(err)
	r.Len(files, 1)

	saved, err := os.ReadFile(files[0])
	r.NoError(err)
	r.Equal(strings.TrimSpace(longLine()), strings.TrimSpace(string(saved)))
}

func TestSideways_HelpNamesTheKeysOnATextScreenOnly(t *testing.T) {
	r := require.New(t)

	navigation := func(m Model) []hint {
		sections := m.helpSections()

		return sections[len(sections)-1].hints
	}

	r.Contains(navigation(onLongLine(t)), hint{Key: "<home>", Description: "Line Start"})
	r.NotContains(navigation(everyScreen(t)["jobs"]), hint{Key: "<home>", Description: "Line Start"})
	r.NotContains(navigation(everyScreen(t)["plan"]), hint{Key: "<home>", Description: "Line Start"})
}

// farRight is a log of a 300-cell line and others, moved as far right as it
// goes.
func farRight(t *testing.T, lines ...string) Model {
	t.Helper()

	m, _ := onLogs(t, append([]string{longLine()}, lines...)...)
	for range 50 {
		m = right(m)
	}

	require.Equal(t, 300-m.text.width, m.text.shift)

	return m
}

func TestSideways_ATextThatNowFitsMovesBackToColumnOne(t *testing.T) {
	steps := []struct {
		name  string
		do    func(m Model) Model
		first string
	}{
		{"a filter that keeps a short line", func(m Model) Model {
			m, _ = m.update(key('/'))
			m = typeIn(m, "short")
			m, _ = m.update(enter())

			return m
		}, "short"},
		{"a wider window", func(m Model) Model {
			m, _ = m.update(tea.WindowSizeMsg{Width: 400, Height: 30})

			return m
		}, strings.TrimSpace(longLine())},
	}

	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			r := require.New(t)

			m := step.do(farRight(t, "short\n"))

			r.Zero(m.text.shift)
			r.NotContains(togglesOf(m), "Column")
			r.Equal(step.first, firstRow(m))
		})
	}
}

func TestSideways_TheWidestLineLeavingTheFrontMovesBackToColumnOne(t *testing.T) {
	r := require.New(t)

	m := farRight(t)
	m, _ = m.update(lineOf(m, strings.Join(linesFrom(0, maxTextLines), "\n")+"\n"))

	r.Equal(1, m.text.dropped)
	r.Zero(m.text.shift)
	r.NotContains(togglesOf(m), "Column")
}

// described is a description of content opened from the jobs.
func described(t *testing.T, content string) Model {
	t.Helper()

	m, _ := everyScreen(t)["jobs"].update(describeMsg{label: "Job spec: web", content: content})

	return m
}

func TestSideways_ATabReadsAsTheFourSpacesItIsDrawnAs(t *testing.T) {
	for _, wrap := range []bool{false, true} {
		t.Run(sprintf("wrap %v", wrap), func(t *testing.T) {
			m := described(t, "job {\t# x")
			if wrap {
				m, _ = m.update(key('w'))
			}

			require.Equal(t, "job {    # x", firstRow(m))
		})
	}
}

func TestSideways_ATabIndentedLineShowsItsEndAsFarRightAsItGoes(t *testing.T) {
	r := require.New(t)

	m := described(t, "\t"+strings.TrimSpace(longLine()))
	for range 50 {
		m = right(m)
	}

	r.Equal(304-m.text.width, m.text.shift)
	r.True(strings.HasSuffix(firstRow(m), "<END>"), firstRow(m))
}

func TestSideways_SavingADescriptionKeepsItsTabs(t *testing.T) {
	r := require.New(t)

	dir := t.TempDir()
	t.Chdir(dir)

	m := described(t, "job {\t# x")

	m, cmd := m.update(ctrlKey('s'))
	drain(m, cmd)

	files, err := filepath.Glob(filepath.Join(dir, "*.txt"))
	r.NoError(err)
	r.Len(files, 1)

	saved, err := os.ReadFile(files[0])
	r.NoError(err)
	r.Equal("job {\t# x", strings.TrimSpace(string(saved)))
}

func TestSideways_AListBackFromAMovedTextIsNotMoved(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), logs: &nomad.LogStream{Lines: make(chan string)}}

	m := openTasks(t, client)
	m, _ = m.update(arrow(tea.KeyDown))
	m, cmd := m.update(enter())
	m = right(withLines(drain(m, cmd), []string{longLine()}))
	r.NotZero(m.text.shift)

	m, _ = m.update(escape())
	r.Equal(1, m.list.table.cursor)
	r.NotContains(m.title(), "from column")

	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyHome})
	r.Zero(m.list.table.cursor)
}
