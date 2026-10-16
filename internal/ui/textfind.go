package ui

import (
	tea "charm.land/bubbletea/v2"
)

// askFind opens the line that asks for the word to find.
func (m Model) askFind() (Model, tea.Cmd) {
	return m.askLine(lineMsg{prefix: "find: ", answer: findWord})
}

// findWord lights the word and goes to its first match from the top of the
// window. An empty word drops the find; a word that matches nothing leaves
// the one found before.
func findWord(m Model, word string) (Model, tea.Cmd) {
	if word == "" {
		m.text.found = ""

		return m, nil
	}

	before := m.text.found
	m.text.found = word

	nth, of := m.text.seek(m.text.top, false)
	if of == 0 {
		m.text.found = before

		return m.warn(sprintf(`No line matches "%s".`, word)), nil
	}

	return m.say(sprintf(`Match %d of %d for "%s".`, nth, of, word)), nil
}

// goToMatch moves the window to the next match of the found word, or the
// previous one when by is negative. A window scrolled away from the match it
// was on starts from what it shows: its top going down, its bottom going up.
func (m Model) goToMatch(by int) (Model, tea.Cmd) {
	from := m.text.at + by

	if m.text.at < m.text.top || m.text.at >= m.text.top+m.text.height {
		from = m.text.top
		if by < 0 {
			from = m.text.top + m.text.height - 1
		}
	}

	nth, of := m.text.seek(from, by < 0)
	if of == 0 {
		return m.warn(sprintf(`No line matches "%s".`, m.text.found)), nil
	}

	return m.say(sprintf(`Match %d of %d for "%s".`, nth, of, m.text.found)), nil
}
