package ui

import (
	tea "charm.land/bubbletea/v2"
)

// scrollKey moves whatever the screen shows, a list of rows or a block of
// text. The keys are named once: a screen that scrolls does not get to
// invent its own.
func (m Model) scrollKey(msg tea.KeyPressMsg) (Model, bool) {
	page, end := m.pageHeight(), m.contentLength()

	switch msg.String() {
	case "up", "k":
		return m.scroll(-1), true
	case "down", "j":
		return m.scroll(1), true
	case "pgup", "ctrl+b":
		return m.scroll(-page), true
	case "pgdown", "ctrl+f":
		return m.scroll(page), true
	case "home", "g":
		return m.scroll(-end), true
	case "end", "G":
		return m.scroll(end), true
	}

	return m, false
}

// scroll moves the screen by that many lines.
func (m Model) scroll(by int) Model {
	if m.readsAsText() {
		m.text.move(by)

		// Scrolling by hand means the end of the output is no longer being
		// watched.
		m.following = false

		return m
	}

	m.table.move(by)

	return m
}

// readsAsText says the screen shows a block of text rather than a list.
func (m Model) readsAsText() bool {
	return m.screen.kind == screenDescribe || m.screen.kind == screenLogs
}

func (m Model) pageHeight() int {
	if m.readsAsText() {
		return m.text.height
	}

	return m.table.height
}

func (m Model) contentLength() int {
	if m.readsAsText() {
		return m.text.length()
	}

	return len(m.table.rows)
}
