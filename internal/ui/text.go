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

	// kept are the rows drawn so far, so that a line that arrives is drawn
	// alone.
	kept rowCache
}

// textID tells one text from another. It has a byte so that two of them
// never share an address.
type textID struct{ _ byte }

// rowSettings are what the rows of a text depend on, besides its lines.
type rowSettings struct {
	filter string
	wrap   bool
	times  bool

	// width counts only when wrap is on.
	width int
}

// rowCache is the rows drawn for the text id with settings. lines is the
// number of the line after the last one drawn, counted from the first line
// the text had, the dropped ones too.
type rowCache struct {
	id       *textID
	settings rowSettings
	lines    int

	// counts are the rows of each of those lines: none when filtered out.
	counts []int
	rows   []textRow
}

// maxTextLines is how many lines a text keeps. The tails of the logs fit,
// and so does the last MiB a file opens with when its lines average 21 bytes
// or more.
const maxTextLines = 50_000

// textContent is what a text holds, apart from how it is shown: a text page
// keeps it, and the root model keeps the window over it.
type textContent struct {
	// id tells this text from another one, and stays through copies of it.
	// A text with none is drawn afresh each time.
	id *textID

	lines []string

	// dropped are the lines taken off the front so far.
	dropped int

	// stamps are when each line arrived, for the lines that arrived while
	// the screen was open. A line that was already there when the screen
	// opened has the zero time: what a task writes carries no time of its
	// own.
	stamps []time.Time

	// paint is the style of the lines urga drew in a colour of their own,
	// by line; nil is the style of text. The lines stay plain text: the
	// filter matches them and a saved file keeps them, and the colour is
	// added only when drawn.
	paint []*lipgloss.Style

	// tags go in front of lines to say where each came from, in a colour
	// of their own; nil is no tag. The three slices may be shorter than
	// lines: what is past the end is unset. The lines of a chunk share one
	// tag and one style, so a line costs a pointer, not a copy.
	tags []*tag
}

// total is how many lines the text has had, the dropped ones too.
func (t textContent) total() int {
	return t.dropped + len(t.lines)
}

// stampAt is the time line i arrived, zero when it has none.
func (t textContent) stampAt(i int) time.Time {
	if i < len(t.stamps) {
		return t.stamps[i]
	}

	return time.Time{}
}

// paintAt is the style of line i, nil when it is drawn as text.
func (t textContent) paintAt(i int) *lipgloss.Style {
	if i < len(t.paint) {
		return t.paint[i]
	}

	return nil
}

// tagAt is the tag of line i, nil when it has none.
func (t textContent) tagAt(i int) *tag {
	if i < len(t.tags) {
		return t.tags[i]
	}

	return nil
}

// add puts lines read from a stream at the end, stamped with when they
// arrived, behind a tag and in a style of their own when they have one.
func (t *textContent) add(lines []string, arrived time.Time, label *tag, style *lipgloss.Style) {
	t.identify()

	// Lines that were there before have no time, tag or style.
	t.stamps = grown(t.stamps, len(t.lines))

	tagged := label != nil || t.tags != nil
	painted := style != nil || t.paint != nil

	if tagged {
		t.tags = grown(t.tags, len(t.lines))
	}

	if painted {
		t.paint = grown(t.paint, len(t.lines))
	}

	for _, line := range lines {
		t.lines = append(t.lines, cleanLine(line))
		t.stamps = append(t.stamps, arrived)

		if tagged {
			t.tags = append(t.tags, label)
		}

		if painted {
			t.paint = append(t.paint, style)
		}
	}

	t.dropOld()
}

// dropOld takes the lines past maxTextLines off the front. The slices are cut,
// not copied: the next append grows them anew and frees the old front.
func (t *textContent) dropOld() {
	n := len(t.lines) - maxTextLines
	if n <= 0 {
		return
	}

	t.lines = t.lines[n:]
	t.stamps = dropFront(t.stamps, n)
	t.tags = dropFront(t.tags, n)
	t.paint = dropFront(t.paint, n)
	t.dropped += n
}

// dropFront cuts n items off the front of s, which may be shorter than n.
func dropFront[T any](s []T, n int) []T {
	if len(s) <= n {
		return nil
	}

	return s[n:]
}

// identify gives the text an id when it has none.
func (t *textContent) identify() {
	if t.id == nil {
		t.id = &textID{}
	}
}

// grown grows s with zero values to n items.
func grown[T any](s []T, n int) []T {
	if len(s) < n {
		s = append(s, make([]T, n-len(s))...)
	}

	return s
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
	style *lipgloss.Style

	// tagFrom and tagTo are where the tag of the line is in text, and
	// tagStyle what it is drawn in. A row without one has them empty. They
	// count cells, not bytes: the edge of the screen cuts a row by its
	// width on screen.
	tagFrom, tagTo int
	tagStyle       *lipgloss.Style
}

// paintedLine is a line to put on a text screen, in a style of its own when
// it has one.
type paintedLine struct {
	text  string
	style *lipgloss.Style
}

// paintedText holds lines that urga drew, some in a colour of their own.
func paintedText(lines []paintedLine) textModel {
	t := textModel{}
	t.identify()

	for _, line := range lines {
		t.lines = append(t.lines, line.text)
		t.paint = append(t.paint, line.style)
	}

	return t
}

// draw is the rows of the lines from up to, not with, to, as they are shown:
// with the time a line arrived when times are on, in its own colour, and laid over as many
// rows as it takes when wrap is on. counts has how many rows each line took:
// none when the filter drops it.
func (t textModel) draw(from, to int) (rows []textRow, counts []int) {
	match := func(string) bool { return true }
	if t.filter != "" {
		match = matcher(t.filter)
	}

	rows = make([]textRow, 0, to-from)
	counts = make([]int, 0, to-from)

	for i := from; i < to; i++ {
		line, label := t.lines[i], t.tagAt(i)

		labelText := ""
		if label != nil {
			labelText = label.text
		}

		if !match(labelText + line) {
			counts = append(counts, 0)

			continue
		}

		// The time a line arrived comes first, then where it came from.
		stamp := t.stamp(i)
		start := ansi.StringWidth(stamp)

		row := textRow{
			text:     stamp + labelText + line,
			style:    t.paintAt(i),
			tagFrom:  start,
			tagTo:    start + ansi.StringWidth(labelText),
			tagStyle: tagStyleOf(label),
		}

		if !t.wrap {
			rows = append(rows, row)
			counts = append(counts, 1)

			continue
		}

		// Every row of a wrapped line is in the colour of the line; the
		// tag is on the first of them.
		parts := wrapLine(row.text, t.width)
		for n, part := range parts {
			out := textRow{text: part, style: row.style}
			if n == 0 {
				out.tagFrom, out.tagTo, out.tagStyle = row.tagFrom, row.tagTo, row.tagStyle
			}

			rows = append(rows, out)
		}

		counts = append(counts, len(parts))
	}

	return rows, counts
}

// tagStyleOf is the style of a tag, nil for no tag.
func tagStyleOf(label *tag) *lipgloss.Style {
	if label == nil {
		return nil
	}

	return &label.style
}

// stamp is the time a line arrived, to go in front of it, where there is one.
func (t textModel) stamp(at int) string {
	if !t.times {
		return ""
	}

	arrived := t.stampAt(at)
	if arrived.IsZero() {
		return strings.Repeat(" ", len(logTimeFormat)+2)
	}

	return arrived.Format(logTimeFormat) + "  "
}

// logTimeFormat is how the time a line arrived reads.
const logTimeFormat = "15:04:05"

// rows are the lines as they go on the screen: cut at the edge, or laid over
// as many rows as they take. The kept rows are those when they are up to
// date; otherwise they are drawn in full.
func (t textModel) rows() []textRow {
	if t.kept.matches(t) {
		return t.kept.rows
	}

	rows, _ := t.draw(0, len(t.lines))

	return rows
}

// settings are what the rows of the text depend on now.
func (t textModel) settings() rowSettings {
	settings := rowSettings{filter: t.filter, wrap: t.wrap, times: t.times}
	if t.wrap {
		settings.width = t.width
	}

	return settings
}

// matches tells whether the rows are of all the lines of t, as t draws them.
func (c rowCache) matches(t textModel) bool {
	return c.id != nil && c.id == t.id && c.lines == t.total() &&
		len(c.counts) == len(t.lines) && c.settings == t.settings()
}

// refresh brings the kept rows up to date: the lines taken off the front are
// forgotten, the lines that arrived since are drawn and added, and anything
// else that changed is drawn again in full. A window that is not at the top
// keeps its lines when rows leave the front.
func (t *textModel) refresh() {
	if t.kept.matches(*t) {
		return
	}

	k := &t.kept
	settings := t.settings()
	first := k.lines - len(k.counts)

	if t.id != nil && k.id == t.id && k.settings == settings &&
		first <= t.dropped && t.dropped <= k.lines && k.lines <= t.total() {
		gone := 0
		for _, count := range k.counts[:t.dropped-first] {
			gone += count
		}

		k.rows = k.rows[gone:]
		k.counts = k.counts[t.dropped-first:]
		t.top = max(t.top-gone, 0)

		rows, counts := t.draw(k.lines-t.dropped, len(t.lines))
		k.rows = append(k.rows, rows...)
		k.counts = append(k.counts, counts...)
		k.lines = t.total()

		return
	}

	rows, counts := t.draw(0, len(t.lines))
	*k = rowCache{id: t.id, settings: settings, lines: t.total(), counts: counts, rows: rows}
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
	t := textModel{}
	t.identify()

	if content != "" {
		t.lines = strings.Split(content, "\n")
	}

	return t
}

func (t *textModel) setSize(width, height int) {
	t.width, t.height = width, max(height, 1)
	t.follow()
}

func (t *textModel) move(delta int) {
	t.refresh()
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
	style := styleText
	if row.style != nil {
		style = *row.style
	}

	var label lipgloss.Style
	if row.tagStyle != nil {
		label = *row.tagStyle
	}

	return t.lit(ansi.Cut(line, 0, row.tagFrom), style) +
		t.lit(ansi.Cut(line, row.tagFrom, row.tagTo), label) +
		t.lit(ansi.TruncateLeft(line, row.tagTo, ""), style) + style.Render(gap)
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
