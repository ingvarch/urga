package ui

import (
	"strings"
	"time"

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

	top int

	width  int
	height int
}

// visible are the lines the filter leaves, as they are read: with the time
// they arrived when that is asked for.
func (t textModel) visible() []string {
	// A log arrives a line at a time and is read as often as it arrives:
	// with nothing to filter and nothing to stamp, the lines are the answer.
	if t.filter == "" && !t.times {
		return t.lines
	}

	kept := make([]string, 0, len(t.lines))

	match := func(string) bool { return true }
	if t.filter != "" {
		match = matcher(t.filter)
	}

	for i, line := range t.lines {
		if !match(line) {
			continue
		}

		kept = append(kept, t.stamped(i, line))
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
func (t textModel) rows() []string {
	lines := t.visible()
	if !t.wrap {
		return lines
	}

	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, wrapLine(line, t.width)...)
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

func newTextModel(content string) textModel {
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

// line draws one row, with what the filter matched lit up in it.
func (t textModel) line(line string) string {
	line = truncate(line, t.width)
	gap := strings.Repeat(" ", max(t.width-ansi.StringWidth(line), 0))

	if t.filter == "" {
		return styleText.Render(line + gap)
	}

	out := strings.Builder{}

	for rest := line; rest != ""; {
		at := matchIn(rest, t.filter)
		if at == nil {
			out.WriteString(styleText.Render(rest))

			break
		}

		out.WriteString(styleText.Render(rest[:at[0]]))
		out.WriteString(styleMatch.Render(rest[at[0]:at[1]]))

		rest = rest[at[1]:]
	}

	return out.String() + styleText.Render(gap)
}
