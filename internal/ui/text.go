package ui

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/charmbracelet/x/ansi"
)

// textModel is a block of text with a window over it: what a description or
// a job file is read through.
type textModel struct {
	lines []string

	// stamps are when each line arrived, for the lines that were watched
	// arriving. A line that was already there when the screen opened has
	// none: what a task writes carries no time of its own.
	stamps map[int]time.Time

	// filter keeps only the lines that say it.
	filter string

	// wrap lays a long line over as many rows as it takes, instead of
	// cutting it at the edge of the screen.
	wrap bool

	// times puts the time a line arrived in front of it.
	times bool

	// paint is the style of the lines urga drew in a colour of their own,
	// by line. The lines stay words: the filter reads them and a file keeps
	// them, and the colour goes on only when they are drawn.
	paint map[int]lipgloss.Style

	top int

	width  int
	height int
}

// emptied is the same window with nothing in it: another text, read the way
// this one was.
func (t textModel) emptied() textModel {
	t.lines, t.stamps, t.top = nil, nil, 0

	return t
}

// textRow is a line, or the part of one that fits a row, and the style it is
// drawn in.
type textRow struct {
	text  string
	style lipgloss.Style
}

// paintedLine is a line to put on a text screen, in a style of its own when
// it has one.
type paintedLine struct {
	text  string
	style *lipgloss.Style
}

// paintedText holds lines that urga drew, some in a colour of their own.
func paintedText(lines []paintedLine) textModel {
	t := textModel{paint: map[int]lipgloss.Style{}}

	for i, line := range lines {
		t.lines = append(t.lines, line.text)

		if line.style != nil {
			t.paint[i] = *line.style
		}
	}

	return t
}

// visible are the lines the filter leaves, as they are read: with the time
// they arrived when that is asked for, and in their own colour.
func (t textModel) visible() []textRow {
	match := func(string) bool { return true }
	if t.filter != "" {
		match = matcher(t.filter)
	}

	kept := make([]textRow, 0, len(t.lines))

	for i, line := range t.lines {
		if !match(line) {
			continue
		}

		style, painted := t.paint[i]
		if !painted {
			style = styleText
		}

		kept = append(kept, textRow{text: t.stamped(i, line), style: style})
	}

	return kept
}

// stamped puts the time a line arrived in front of it, where there is one.
func (t textModel) stamped(at int, line string) string {
	if !t.times {
		return line
	}

	stamp, ok := t.stamps[at]
	if !ok {
		return strings.Repeat(" ", len(logTimeFormat)+2) + line
	}

	return stamp.Format(logTimeFormat) + "  " + line
}

// logTimeFormat is how the time a line arrived reads.
const logTimeFormat = "15:04:05"

// rows are the lines as they go on the screen: cut at the edge, or laid over
// as many rows as they take.
func (t textModel) rows() []textRow {
	lines := t.visible()
	if !t.wrap {
		return lines
	}

	out := make([]textRow, 0, len(lines))
	for _, line := range lines {
		// Every row of a wrapped line is in the colour of the line.
		for _, part := range wrapLine(line.text, t.width) {
			out = append(out, textRow{text: part, style: line.style})
		}
	}

	return out
}

// wrapLine breaks a line into rows of the given width, by what the line
// looks like rather than by the bytes it takes: the colour a task writes in
// is not width.
func wrapLine(line string, width int) []string {
	if width < 1 || ansi.StringWidth(line) <= width {
		return []string{line}
	}

	return strings.Split(ansi.Hardwrap(line, width, true), "\n")
}

// newTextModel holds content as lines. Nothing is no line at all, not one
// empty line: a log that starts empty starts with what the task writes next.
func newTextModel(content string) textModel {
	if content == "" {
		return textModel{}
	}

	return textModel{lines: strings.Split(content, "\n")}
}

func (t *textModel) setSize(width, height int) {
	t.width, t.height = width, max(height, 1)
	t.follow()
}

func (t *textModel) move(delta int) {
	t.top = clamp(t.top+delta, 0, max(t.length()-t.height, 0))
}

// length is how many rows the window moves over: wrapped lines take more
// than one, filtered ones take none.
func (t textModel) length() int {
	return len(t.rows())
}

func (t *textModel) toEnd() {
	t.move(t.length())
}

// follow pulls the window back over the lines, which is what a move of
// nothing does.
func (t *textModel) follow() {
	t.move(0)
}

func (t textModel) view() string {
	lines := t.rows()

	rows := make([]string, 0, t.height)

	for i := t.top; i < len(lines) && i < t.top+t.height; i++ {
		rows = append(rows, t.line(lines[i]))
	}

	return strings.Join(rows, "\n")
}

// line draws one row in its colour, with what the filter matched lit up in
// it.
func (t textModel) line(row textRow) string {
	line := truncate(row.text, t.width)
	gap := strings.Repeat(" ", max(t.width-ansi.StringWidth(line), 0))

	if t.filter == "" {
		return row.style.Render(line + gap)
	}

	out := strings.Builder{}

	for rest := line; rest != ""; {
		at := matchIn(rest, t.filter)
		if at == nil {
			out.WriteString(row.style.Render(rest))

			break
		}

		out.WriteString(row.style.Render(rest[:at[0]]))
		out.WriteString(styleMatch.Render(rest[at[0]:at[1]]))

		rest = rest[at[1]:]
	}

	return out.String() + row.style.Render(gap)
}
