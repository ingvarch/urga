package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// copyLimit is 64 KiB: OSC52 sends the text as base64 in one escape
// sequence, and terminals drop a sequence much longer than this.
const copyLimit = 64 << 10

// tail is the last whole lines that fit in limit bytes once joined by
// newlines.
func tail(lines []string, limit int) []string {
	size, from := 0, len(lines)

	for from > 0 {
		next := len(lines[from-1])
		if size > 0 {
			next++
		}

		if size+next > limit {
			break
		}

		size += next
		from--
	}

	return lines[from:]
}

// copyText puts what the screen shows on the clipboard. A text over the
// limit is cut to its last lines, since the end of a log is what is read.
func (m Model) copyText() (Model, tea.Cmd) {
	if !m.readsAsText() {
		return m, nil
	}

	lines := m.shownLines()
	if len(lines) == 0 {
		return m.say("Nothing to copy."), nil
	}

	kept := tail(lines, copyLimit)
	if len(kept) == 0 {
		return m.warn("The last line is too long to copy."), nil
	}

	if len(kept) < len(lines) {
		m = m.say(sprintf("Copied last %d of %s.", len(kept), plural(len(lines), "line")))
	} else {
		m = m.say(sprintf("Copied %s.", plural(len(kept), "line")))
	}

	return m, tea.SetClipboard(strings.Join(kept, "\n"))
}
