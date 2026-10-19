package ui

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// drawnInFull are the rows of a text drawn from its lines, whatever the
// window kept.
func drawnInFull(t textModel) []textRow {
	rows, _ := t.draw(0, len(t.lines))

	return rows
}

// widestDrawn is the width in cells of the widest row of a text drawn in full.
func widestDrawn(t textModel) int {
	widest := 0
	for _, row := range drawnInFull(t) {
		widest = max(widest, ansi.StringWidth(row.text))
	}

	return widest
}

// arrivedAt is the time the test lines arrive: fixed, so the stamp never
// holds what a filter looks for.
var arrivedAt = time.Date(2026, 10, 15, 9, 30, 0, 0, time.UTC)

// chunk adds count lines that start with prefix.
func chunk(t *textModel, prefix string, count int, label *tag, style *lipgloss.Style) {
	lines := make([]string, count)
	for i := range lines {
		lines[i] = fmt.Sprintf("%s %d %s", prefix, i, strings.Repeat("x", 30*(i%4)))
	}

	t.add(lines, arrivedAt, label, style)
}

// textSteps are the changes a text goes through, each with its name.
func textSteps() []struct {
	name string
	do   func(t *textModel)
} {
	label := &tag{text: "9a1b2c3d │ ", style: lipgloss.NewStyle().Foreground(colorTitle)}
	other := &tag{text: "4f2a1c9e │ ", style: lipgloss.NewStyle().Foreground(colorTitle)}
	red := lipgloss.NewStyle().Bold(true)

	return []struct {
		name string
		do   func(t *textModel)
	}{
		{"lines without a tag", func(t *textModel) { chunk(t, "plain", 5, nil, nil) }},
		{"lines with a tag", func(t *textModel) { chunk(t, "tagged", 5, label, nil) }},
		{"lines with a tag and a style", func(t *textModel) { chunk(t, "styled", 4, other, &red) }},
		{"more lines", func(t *textModel) { chunk(t, "more", 7, label, nil) }},
		{"times on", func(t *textModel) { t.times = true }},
		{"lines with times", func(t *textModel) { chunk(t, "timed", 3, other, nil) }},
		{"wrap on", func(t *textModel) { t.wrap = true }},
		{"lines wrapped", func(t *textModel) { chunk(t, "wrapped", 6, label, &red) }},
		{"a narrower window", func(t *textModel) { t.setSize(23, 6) }},
		{"a wider window", func(t *textModel) { t.setSize(90, 6) }},
		{"a filter and more lines at once", func(t *textModel) {
			t.filter = "d 2"
			chunk(t, "both", 6, other, nil)
		}},
		{"a narrower window and more lines at once", func(t *textModel) {
			t.setSize(25, 6)
			chunk(t, "narrow", 6, label, nil)
		}},
		{"times off and more lines at once", func(t *textModel) {
			t.times = false
			chunk(t, "untimed", 6, label, nil)
		}},
		{"times on", func(t *textModel) { t.times = true }},
		{"a filter", func(t *textModel) { t.filter = "ed 1" }},
		{"lines past a filter", func(t *textModel) { chunk(t, "filtered", 9, other, nil) }},
		{"another filter", func(t *textModel) { t.filter = "!plain" }},
		{"no filter", func(t *textModel) { t.filter = "" }},
		{"wrap off", func(t *textModel) { t.wrap = false }},
		{"a narrower window without wrap", func(t *textModel) { t.setSize(30, 6) }},
		{"times off", func(t *textModel) { t.times = false }},
		{"a new content of more lines", func(t *textModel) {
			replacement := textModel{}
			chunk(&replacement, "new", 200, label, nil)
			t.textContent = replacement.textContent
		}},
		{"lines on the new content", func(t *textModel) { chunk(t, "after", 3, nil, &red) }},
		{"painted lines", func(t *textModel) {
			t.textContent = paintedText([]paintedLine{{text: "a"}, {text: "b", style: &red}, {text: "c"}}).textContent
		}},
		{"a text read whole", func(t *textModel) {
			t.textContent = newTextModel(strings.Repeat("one line\nanother line\n", 20)).textContent
		}},
		{"lines after a text read whole", func(t *textModel) { chunk(t, "tail", 4, label, nil) }},
	}
}

func TestText_KeptRowsEqualRowsDrawnInFull(t *testing.T) {
	text := textModel{}
	text.setSize(40, 8)

	for _, step := range textSteps() {
		step.do(&text)
		text.follow()

		require.Equal(t, drawnInFull(text), text.rows(), step.name)

		// The rows come from the cache, not from a draw in full.
		require.True(t, text.kept.matches(text), "%s: the rows are kept up to date", step.name)
		require.Equal(t, drawnInFull(text), text.kept.rows, step.name)
		require.Equal(t, widestDrawn(text), text.kept.widest, step.name)
	}
}

// costOfALine is the allocations of a line arriving at a text of the given
// length: it is added, and the window is sized, followed to the end and drawn.
func costOfALine(lines int) float64 {
	label := &tag{text: "9a1b2c3d │ ", style: lipgloss.NewStyle().Foreground(colorTitle)}

	text := textModel{times: true, wrap: true, filter: "7"}
	text.setSize(80, 20)

	for i := 0; i < lines; i += 100 {
		chunk(&text, "line", 100, label, nil)
	}

	text.follow()

	arrived := lines

	return testing.AllocsPerRun(50, func() {
		text.add([]string{fmt.Sprintf("line %d", arrived)}, arrivedAt, label, nil)
		arrived++

		text.setSize(80, 20)
		text.follow()
		text.toEnd()
		_ = text.view()
	})
}

func TestText_ANewLineCostsTheSameHoweverLongTheText(t *testing.T) {
	short, long := costOfALine(1_000), costOfALine(40_000)

	require.LessOrEqual(t, long, short+10, "allocations of a line at 40,000 lines and at 1,000")
}

func TestText_ANewLineCostsTheSameAtTheCap(t *testing.T) {
	short, capped := costOfALine(1_000), costOfALine(maxTextLines)

	require.LessOrEqual(t, capped, short+10, "allocations of a line at the cap and at 1,000")
}

// linesFrom are count lines "line n", from first.
func linesFrom(first, count int) []string {
	lines := make([]string, count)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", first+i)
	}

	return lines
}

func TestText_AStreamKeepsItsLastLines(t *testing.T) {
	r := require.New(t)

	old := &tag{text: "old │ "}
	own := &tag{text: "own │ "}
	bold := lipgloss.NewStyle().Bold(true)
	arrived := arrivedAt

	text := textModel{}
	text.add(linesFrom(0, 10), arrived.Add(-time.Hour), old, nil)

	// The 11th line is the first one kept, with what it came with.
	text.add(linesFrom(10, 1), arrived, own, &bold)

	for next := 11; next < maxTextLines+10; next += 1000 {
		text.add(linesFrom(next, min(1000, maxTextLines+10-next)), arrived.Add(time.Hour), old, nil)
	}

	r.Equal(maxTextLines, len(text.lines))
	r.Equal("line 10", text.lines[0])
	r.Equal(fmt.Sprintf("line %d", maxTextLines+9), text.lines[maxTextLines-1])
	r.Equal(10, text.dropped)
	r.Equal(maxTextLines+10, text.total())

	r.Equal(arrived, text.stampAt(0))
	r.Same(own, text.tagAt(0))
	r.Same(&bold, text.paintAt(0))
	r.Same(old, text.tagAt(1))
	r.Nil(text.paintAt(1))
	r.Equal(maxTextLines, len(text.stamps))
}

func TestText_ALongTextHasItsFrontDroppedWithoutMissingRows(t *testing.T) {
	text := textModel{wrap: true, filter: "5"}
	text.setSize(8, 6)

	steps := []struct {
		name string
		do   func()
	}{
		// "line 1005" is 9 cells and wraps at 8; the rest of the first ten
		// have no 5 and are filtered out.
		{"filled to the cap", func() { text.add(linesFrom(1000, maxTextLines-3), arrivedAt, nil, nil) }},
		{"a few lines past it", func() {
			// The seven lines about to leave the front take no rows and two.
			require.True(t, slices.Contains(text.kept.counts[:7], 0))
			require.True(t, slices.Contains(text.kept.counts[:7], 2))

			text.add(linesFrom(0, 10), arrivedAt, nil, nil)
		}},
		{"one more", func() { text.add(linesFrom(90, 1), arrivedAt, nil, nil) }},
		{"a window of another width", func() { text.setSize(12, 6) }},
		{"more lines than the cap", func() { text.add(linesFrom(0, maxTextLines+5), arrivedAt, nil, nil) }},
		{"another filter", func() { text.filter = "1" }},
		{"lines past it", func() { text.add(linesFrom(0, 7), arrivedAt, nil, nil) }},
	}

	for _, step := range steps {
		step.do()
		text.follow()

		require.True(t, slices.Equal(drawnInFull(text), text.rows()), step.name)
		require.True(t, text.kept.matches(text), "%s: the rows are kept up to date", step.name)
		require.Equal(t, widestDrawn(text), text.kept.widest, step.name)
	}
}

func TestText_AWindowMovesUpByTheRowsOfTheDroppedLines(t *testing.T) {
	text := textModel{wrap: true, filter: "5"}
	text.setSize(8, 6)
	text.add(linesFrom(0, maxTextLines), arrivedAt, nil, nil)
	text.follow()

	const dropped = 3000

	// The rows of the lines that will leave the front, drawn alone.
	front := textModel{wrap: true, filter: "5"}
	front.setSize(8, 6)
	front.add(linesFrom(0, dropped), arrivedAt, nil, nil)
	gone := len(drawnInFull(front))

	require.NotEqual(t, dropped, gone, "the dropped lines take fewer or more rows than there are lines")

	text.top = 5000
	text.add(linesFrom(maxTextLines, dropped), arrivedAt, nil, nil)
	text.refresh()

	require.Equal(t, dropped, text.dropped)
	require.Equal(t, 5000-gone, text.top)
}

func TestText_AWindowThatDoesNotFollowKeepsItsLines(t *testing.T) {
	r := require.New(t)

	stream := &nomad.LogStream{Lines: make(chan string), Err: make(chan error)}
	m := readingEnv(t, &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}, stream)

	m, _ = m.update(logLineMsg{stream: stream, text: strings.Join(linesFrom(0, maxTextLines), "\n") + "\n"})
	r.False(m.text.following)

	top := func() string { return m.text.rows()[m.text.top].text }

	// A line near the top stays at the top as the front is dropped.
	m.text.top = 100
	line := top()

	m, _ = m.update(logLineMsg{stream: stream, text: strings.Join(linesFrom(maxTextLines, 5), "\n") + "\n"})
	r.Equal(5, m.text.dropped)
	r.Equal(line, top())

	// When the line itself is dropped, the window is at the top of what is
	// left.
	m.text.top = 2

	m, _ = m.update(logLineMsg{stream: stream, text: strings.Join(linesFrom(maxTextLines+5, 5), "\n") + "\n"})
	r.Equal(10, m.text.dropped)
	r.Equal(0, m.text.top)
}

func TestLogs_FollowingGoesOnAtTheCap(t *testing.T) {
	r := require.New(t)

	stream := &nomad.LogStream{Lines: make(chan string), Err: make(chan error)}
	m := openLog(t, &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}, stream)

	m, _ = m.update(logLineMsg{stream: stream, text: strings.Join(linesFrom(0, maxTextLines), "\n") + "\n"})
	r.Contains(plain(m.render()), fmt.Sprintf("line %d", maxTextLines-1))

	m, _ = m.update(logLineMsg{stream: stream, text: "the newest line\n"})

	// The window is still at the end: the newest line is its last row.
	rows := strings.Split(plain(m.text.view()), "\n")
	r.Contains(rows[len(rows)-1], "the newest line")
}
