package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

// errorLog is a log of 100 lines, with "error" on lines 20, 50 and 80, in a
// window of 10 rows.
func errorLog(t *testing.T) Model {
	t.Helper()

	lines := make([]string, 0, 100)
	for i := range 100 {
		line := fmt.Sprintf("line %03d", i)
		if i == 20 || i == 50 || i == 80 {
			line += " error"
		}

		lines = append(lines, line+"\n")
	}

	m, _ := onLogs(t, lines...)

	return windowOf(m, 10)
}

// windowOf resizes the screen so that the window over the text has that many
// rows.
func windowOf(m Model, rows int) Model {
	m, _ = m.update(tea.WindowSizeMsg{Width: 120, Height: 30 - (m.text.height - rows)})

	return m
}

func foundIn(m Model, word string) Model {
	m, _ = m.update(key('f'))
	m = typeIn(m, word)
	m, _ = m.update(enter())

	return m
}

func status(m Model) string { return plain(statusLine(m)) }

func TestFind_KeepsEveryLineAndLightsTheWord(t *testing.T) {
	r := require.New(t)

	m := errorLog(t)
	r.Equal(10, m.text.height)

	m, _ = m.update(key('f'))
	r.Equal(overlayAnswer, m.overlay)
	r.Contains(plain(m.render()), "find: ")

	m = typeIn(m, "error")
	m, _ = m.update(enter())

	r.Equal(100, m.text.length())

	// The window had no match at or below its top row, so it goes round to
	// the first one, a third of the way down.
	out := plain(m.render())
	r.Contains(out, "line 020 error")
	r.Contains(out, "line 017")
	r.NotContains(out, "line 016")
	r.Contains(status(m), `Match 1 of 3 for "error".`)
	r.Contains(m.render(), styleMatch.Render("error"))
}

func TestFind_StartsFromTheTopOfTheWindow(t *testing.T) {
	r := require.New(t)

	m := errorLog(t)
	m.text.top = 40

	m = foundIn(m, "error")

	r.Contains(plain(m.render()), "line 050 error")
	r.Contains(status(m), `Match 2 of 3 for "error".`)
}

func TestFind_NextAndPreviousGoRound(t *testing.T) {
	r := require.New(t)

	m := foundIn(errorLog(t), "error")
	r.Contains(plain(m.render()), "line 020 error")

	for _, want := range []struct{ line, says string }{
		{"line 050 error", "Match 2 of 3"},
		{"line 080 error", "Match 3 of 3"},
		{"line 020 error", "Match 1 of 3"},
	} {
		m, _ = m.update(key('n'))

		r.Contains(plain(m.render()), want.line)
		r.Contains(status(m), want.says+` for "error".`)
	}

	m, _ = m.update(key('N'))

	r.Contains(plain(m.render()), "line 080 error")
	r.Contains(status(m), `Match 3 of 3 for "error".`)
}

func TestFind_NextAndPreviousStartFromAWindowScrolledAway(t *testing.T) {
	r := require.New(t)

	// The match found is at line 20; the window goes to the top, where line
	// 20 is out of sight, and n starts from what is on the screen.
	m := foundIn(errorLog(t), "error")
	m, _ = m.update(key('g'))
	m, _ = m.update(key('n'))

	r.Contains(plain(m.render()), "line 020 error")
	r.Contains(status(m), `Match 1 of 3 for "error".`)

	// From the end, N goes to the last match above the bottom of the window.
	m, _ = m.update(key('G'))
	m, _ = m.update(key('N'))

	r.Contains(plain(m.render()), "line 080 error")
	r.Contains(status(m), `Match 3 of 3 for "error".`)
}

func TestFind_NextAndPreviousAreOfferedWhileAWordIsFound(t *testing.T) {
	r := require.New(t)

	m := errorLog(t)
	r.True(offers(m, "f"))
	r.False(offers(m, "n"))
	r.False(offers(m, "N"))

	m = foundIn(m, "error")
	r.True(offers(m, "n"))
	r.True(offers(m, "N"))

	// An empty word drops the find.
	m, _ = m.update(key('f'))
	m, _ = m.update(enter())

	r.False(offers(m, "n"))
	r.False(offers(m, "N"))
	r.NotContains(m.render(), styleMatch.Render("error"))
}

func TestFind_AWordThatMatchesNothingLeavesTheWindow(t *testing.T) {
	r := require.New(t)

	m := errorLog(t)
	top := m.text.top

	m = foundIn(m, "nothing like it")

	r.Equal(top, m.text.top)
	r.Equal(flashWarn, m.flash.level)
	r.Contains(status(m), `No line matches "nothing like it".`)
	r.False(offers(m, "n"))
}

func TestFind_StopsFollowingAStream(t *testing.T) {
	r := require.New(t)

	m := errorLog(t)
	r.True(m.text.following)

	m = foundIn(m, "error")
	r.False(m.text.following)

	m.text.following = true
	m, _ = m.update(key('n'))

	r.False(m.text.following)
}

func TestFind_CountsTheRowsOfAWrappedLine(t *testing.T) {
	r := require.New(t)

	lines := make([]string, 0, 61)
	for range 30 {
		lines = append(lines, "short\n")
	}

	lines = append(lines, strings.Repeat("a", 200)+" target\n")

	for range 30 {
		lines = append(lines, "short\n")
	}

	m, _ := onLogs(t, lines...)
	m = windowOf(m, 10)
	m, _ = m.update(key('w'))
	m = foundIn(m, "target")

	// The second row of the long line is row 31.
	r.Equal(31-10/3, m.text.top)
	r.Contains(plain(m.render()), "target")
	r.Contains(status(m), `Match 1 of 1 for "target".`)
}

func TestFind_LooksWithinWhatTheFilterLeft(t *testing.T) {
	r := require.New(t)

	m := errorLog(t)
	m, _ = m.update(key('/'))
	m = typeIn(m, "line 0[0-5]")
	m, _ = m.update(enter())

	m = foundIn(m, "error")

	r.Contains(status(m), `Match 1 of 2 for "error".`)
}

func TestFind_EscapeKeepsTheWordFoundBefore(t *testing.T) {
	r := require.New(t)

	m := foundIn(errorLog(t), "error")

	m, _ = m.update(key('f'))
	m = typeIn(m, "other")
	m, _ = m.update(escape())

	r.Equal("error", m.text.found)
	r.True(offers(m, "n"))
	r.Contains(m.render(), styleMatch.Render("error"))
}

func TestFind_TakesAWordAsWrittenWhereAFilterPrefixMeansSomething(t *testing.T) {
	for _, word := range []string{"!boom", "-f config"} {
		t.Run(word, func(t *testing.T) {
			r := require.New(t)

			m, _ := onLogs(t, "quiet\n", "a line with "+word+" in it\n", "quiet\n")
			m = foundIn(m, word)

			r.Contains(status(m), `Match 1 of 1 for "`+word+`".`)
			r.Contains(m.render(), styleMatch.Render(word))
		})
	}
}

func TestFind_PreviousStepsBackWithoutGoingRound(t *testing.T) {
	r := require.New(t)

	m := foundIn(errorLog(t), "error")

	m, _ = m.update(key('n'))
	m, _ = m.update(key('n'))
	r.Contains(status(m), `Match 3 of 3 for "error".`)

	m, _ = m.update(key('N'))

	r.Contains(plain(m.render()), "line 050 error")
	r.Contains(status(m), `Match 2 of 3 for "error".`)
}

func TestFind_AWordThatMatchesNothingKeepsTheWordFoundBefore(t *testing.T) {
	r := require.New(t)

	m := foundIn(errorLog(t), "error")
	m = foundIn(m, "nothing like it")

	r.Equal(flashWarn, m.flash.level)
	r.True(offers(m, "n"))

	m, _ = m.update(key('n'))

	r.Contains(plain(m.render()), "line 050 error")
	r.Contains(status(m), `Match 2 of 3 for "error".`)
}

func TestFind_NextWarnsWhenTheFilterLeftNoMatch(t *testing.T) {
	r := require.New(t)

	m := foundIn(errorLog(t), "error")
	m, _ = m.update(key('/'))
	m = typeIn(m, "line 01")
	m, _ = m.update(enter())

	top, at := m.text.top, m.text.at

	m, _ = m.update(key('n'))

	r.Equal(flashWarn, m.flash.level)
	r.Contains(status(m), `No line matches "error".`)
	r.Equal(top, m.text.top)
	r.Equal(at, m.text.at)
}

func TestFind_KeepsItsPlaceWhenLinesLeaveTheFront(t *testing.T) {
	r := require.New(t)

	lines := func(first, count int) []string {
		out := linesFrom(first, count)
		for i := range out {
			if (first+i)%1000 == 0 {
				out[i] += " error"
			}
		}

		return out
	}

	text := textModel{found: "error"}
	text.setSize(40, 6)
	text.add(lines(0, maxTextLines), arrivedAt, nil, nil)
	text.refresh()

	// The match on line 5000, the sixth.
	nth, _ := text.seek(4001, false)
	r.Equal(6, nth)
	r.Equal(5000, text.at)

	text.add(lines(maxTextLines, 3000), arrivedAt, nil, nil)
	text.refresh()
	r.Equal(3000, text.dropped)

	// Lines 0, 1000 and 2000 are gone, so the match on line 5000 is the third
	// and the next one is the fourth.
	nth, _ = text.seek(text.at+1, false)
	r.Equal(4, nth)
}
