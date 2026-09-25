package ui

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/charmbracelet/x/ansi"
)

// textModel is a block of text with a window over it, used to read a
// description or a job file.
type textModel struct {
	textContent

	// filter keeps only the lines that match it.
	filter string

	// wrap lays a long line over as many rows as it takes, instead of
	// cutting it at the edge of the screen.
	wrap bool

	// times puts the time a line arrived in front of it.
	times bool

	// following keeps the window at the end of a stream as it grows.
	// Scrolling by hand stops it.
	following bool

	top int

	width  int
	height int
}

// textContent is what a text holds, apart from how it is shown: a text page
// keeps it, and the root model keeps the window over it.
type textContent struct {
	lines []string

	// stamps are when each line arrived, for the lines that arrived while
	// the screen was open. A line that was already there when the screen
	// opened has none: what a task writes carries no time of its own.
	stamps map[int]time.Time

	// paint is the style of the lines urga drew in a colour of their own,
	// by line. The lines stay plain text: the filter matches them and a
	// saved file keeps them, and the colour is added only when drawn.
	paint map[int]lipgloss.Style

	// tags go in front of lines to say where each came from, in a colour
	// of their own.
	tags map[int]tag
}

// add puts lines read from a stream at the end, stamped with when they
// arrived, behind a tag and in a style of their own when they have one.
func (t *textContent) add(lines []string, arrived time.Time, label *tag, style *lipgloss.Style) {
	if t.stamps == nil {
		t.stamps = map[int]time.Time{}
	}

	if label != nil && t.tags == nil {
		t.tags = map[int]tag{}
	}

	if style != nil && t.paint == nil {
		t.paint = map[int]lipgloss.Style{}
	}

	for _, line := range lines {
		at := len(t.lines)
		t.lines = append(t.lines, line)
		t.stamps[at] = arrived

		if label != nil {
			t.tags[at] = *label
		}

		if style != nil {
			t.paint[at] = *style
		}
	}
}

// tag is a label in front of a line: the allocation it came from. The
// filter matches it with the line, and it is drawn in a style of its own.
type tag struct {
	text  string
	style lipgloss.Style
}

// textRow is a line, or the part of one that fits a row, and the style it is
// drawn in.
type textRow struct {
	text  string
	style lipgloss.Style

	// tagFrom and tagTo are where the tag of the line is in text, and
	// tagStyle what it is drawn in. A row without one has them empty. They
	// count cells, not bytes: the edge of the screen cuts a row by its
	// width on screen.
	tagFrom, tagTo int
	tagStyle       lipgloss.Style
}

// paintedLine is a line to put on a text screen, in a style of its own when
// it has one.
type paintedLine struct {
	text  string
	style *lipgloss.Style
}

// paintedText holds lines that urga drew, some in a colour of their own.
func paintedText(lines []paintedLine) textModel {
	t := textModel{textContent: textContent{paint: map[int]lipgloss.Style{}}}

	for i, line := range lines {
		t.lines = append(t.lines, line.text)

		if line.style != nil {
			t.paint[i] = *line.style
		}
	}

	return t
}

// visible are the lines the filter keeps, as they are shown: with the time
// they arrived when times are on, and in their own colour.
func (t textModel) visible() []textRow {
	match := func(string) bool { return true }
	if t.filter != "" {
		match = matcher(t.filter)
	}

	kept := make([]textRow, 0, len(t.lines))

	for i, line := range t.lines {
		label := t.tags[i]
		if !match(label.text + line) {
			continue
		}

		style, painted := t.paint[i]
		if !painted {
			style = styleText
		}

		// The time a line arrived comes first, then where it came from.
		stamp := t.stamp(i)
		from := ansi.StringWidth(stamp)

		kept = append(kept, textRow{
			text:     stamp + label.text + line,
			style:    style,
			tagFrom:  from,
			tagTo:    from + ansi.StringWidth(label.text),
			tagStyle: label.style,
		})
	}

	return kept
}

// stamp is the time a line arrived, to go in front of it, where there is one.
func (t textModel) stamp(at int) string {
	if !t.times {
		return ""
	}

	arrived, ok := t.stamps[at]
	if !ok {
		return strings.Repeat(" ", len(logTimeFormat)+2)
	}

	return arrived.Format(logTimeFormat) + "  "
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
		// Every row of a wrapped line is in the colour of the line; the
		// tag is on the first of them.
		for i, part := range wrapLine(line.text, t.width) {
			row := textRow{text: part, style: line.style}
			if i == 0 {
				row.tagFrom, row.tagTo, row.tagStyle = line.tagFrom, line.tagTo, line.tagStyle
			}

			out = append(out, row)
		}
	}

	return out
}

// wrapLine breaks a line into rows of the given width, counted in cells on
// screen, not in bytes: the colour codes a task writes take no width.
func wrapLine(line string, width int) []string {
	if width < 1 || ansi.StringWidth(line) <= width {
		return []string{line}
	}

	return strings.Split(ansi.Hardwrap(line, width, true), "\n")
}

// newTextModel holds content as lines. Empty content is no line, not one
// empty line: a log that starts empty starts with what the task writes next.
func newTextModel(content string) textModel {
	if content == "" {
		return textModel{}
	}

	return textModel{textContent: textContent{lines: strings.Split(content, "\n")}}
}

func (t *textModel) setSize(width, height int) {
	t.width, t.height = width, max(height, 1)
	t.follow()
}

func (t *textModel) move(delta int) {
	t.top = max(0, min(t.top+delta, t.length()-t.height))
}

// length is how many rows the window moves over: wrapped lines take more
// than one, filtered ones take none.
func (t textModel) length() int {
	return len(t.rows())
}

func (t *textModel) toEnd() {
	t.move(t.length())
}

// follow moves the window back inside the lines: a move by zero does that.
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

// line draws one row in its colour, with the filter matches highlighted.
func (t textModel) line(row textRow) string {
	line := truncate(row.text, t.width)
	gap := strings.Repeat(" ", max(t.width-ansi.StringWidth(line), 0))

	// The edge may cut the row inside the tag, in the middle of the "…"
	// mark: splitting the row by cells ends every piece on a whole
	// character.
	return t.lit(ansi.Cut(line, 0, row.tagFrom), row.style) +
		t.lit(ansi.Cut(line, row.tagFrom, row.tagTo), row.tagStyle) +
		t.lit(ansi.TruncateLeft(line, row.tagTo, ""), row.style) + row.style.Render(gap)
}

// lit is a piece of a row in its style, with the filter matches highlighted.
func (t textModel) lit(piece string, style lipgloss.Style) string {
	if piece == "" {
		return ""
	}

	if t.filter == "" {
		return style.Render(piece)
	}

	out := strings.Builder{}

	for rest := piece; rest != ""; {
		at := matchIn(rest, t.filter)
		if at == nil {
			out.WriteString(style.Render(rest))

			break
		}

		out.WriteString(style.Render(rest[:at[0]]))
		out.WriteString(styleMatch.Render(rest[at[0]:at[1]]))

		rest = rest[at[1]:]
	}

	return out.String()
}
