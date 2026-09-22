package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// helpSection is one column of the help window.
type helpSection struct {
	title string
	hints []hint
}

// generalHints are the keys that work on every screen.
var generalHints = []hint{
	{Key: "<:>", Description: "Command"},
	{Key: "</>", Description: "Filter"},
	{Key: "<?>", Description: "Help"},
	{Key: "<0-9>", Description: "Switch namespace"},
	{Key: "<A-Z>", Description: "Sort by that column"},
	{Key: "<q>", Description: "Quit"},
}

// navigationHints are how to move around.
var navigationHints = []hint{
	{Key: "<enter>", Description: "Open"},
	{Key: "<esc>", Description: "Back"},
	{Key: "<k/up>", Description: "Up"},
	{Key: "<j/down>", Description: "Down"},
	{Key: "<pgup/pgdn>", Description: "Page"},
	{Key: "<g/G>", Description: "First, last"},
}

// helpSections are what the help window holds: what this screen can do, then
// what works everywhere.
func (m Model) helpSections() []helpSection {
	sections := []helpSection{}

	if hints := m.screen.hints(); len(hints) > 0 {
		sections = append(sections, helpSection{title: "RESOURCE", hints: hints})
	}

	return append(sections,
		helpSection{title: "GENERAL", hints: generalHints},
		helpSection{title: "NAVIGATION", hints: navigationHints},
	)
}

// renderHelp lays the sections out side by side.
func renderHelp(sections []helpSection, width int) string {
	columns := make([][]string, 0, len(sections))
	height := 0

	for _, section := range sections {
		column := []string{styleLabel.Render(section.title), ""}

		keyWidth, descriptionWidth := 0, 0
		for _, h := range section.hints {
			keyWidth = max(keyWidth, ansi.StringWidth(h.Key))
			descriptionWidth = max(descriptionWidth, ansi.StringWidth(h.Description))
		}

		for _, h := range section.hints {
			column = append(column,
				styleKey.Render(pad(h.Key, keyWidth))+" "+styleText.Render(pad(h.Description, descriptionWidth)))
		}

		height = max(height, len(column))
		columns = append(columns, column)
	}

	widths := make([]int, len(columns))
	for i, column := range columns {
		for _, line := range column {
			widths[i] = max(widths[i], ansi.StringWidth(line))
		}
	}

	rows := make([]string, 0, height)
	for i := 0; i < height; i++ {
		row := ""
		for c, column := range columns {
			cell := ""
			if i < len(column) {
				cell = column[i]
			}

			row += pad(cell, widths[c]) + strings.Repeat(" ", columnGap*2)
		}

		rows = append(rows, ansi.Truncate(strings.TrimRight(row, " "), width, "…"))
	}

	return strings.Join(rows, "\n")
}
