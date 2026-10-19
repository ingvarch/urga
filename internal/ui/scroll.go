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
	case "home":
		if m.text.shift > 0 && m.readsAsText() {
			m.text.shift = 0

			return m, true
		}

		return m.scroll(-end), true
	case "g":
		return m.scroll(-end), true
	case "left", "h":
		return m.slide(-1)
	case "right", "l":
		return m.slide(1)
	case "end", "G":
		return m.scroll(end), true
	}

	return m, false
}

// slide moves a text without wrap sideways by a quarter of its width. On a
// list, and with wrap on, the key is not taken.
func (m Model) slide(direction int) (Model, bool) {
	if !m.readsAsText() || m.text.wrap {
		return m, false
	}

	m.text.slide(direction * max(m.text.width/4, 1))

	return m, true
}

// scroll moves the screen by that many lines.
func (m Model) scroll(by int) Model {
	if m.readsAsText() {
		m.text.move(by)

		// Scrolling by hand stops following the end of the output.
		m.text.following = false

		return m
	}

	m.list.table.move(by)

	return m
}

// readsAsText says the screen shows a block of text rather than a list.
func (m Model) readsAsText() bool {
	_, ok := m.screen.page.(reader)

	return ok
}

func (m Model) pageHeight() int {
	if m.readsAsText() {
		return m.text.height
	}

	return m.list.table.height
}

func (m Model) contentLength() int {
	if m.readsAsText() {
		return m.text.length()
	}

	return len(m.list.table.rows)
}
