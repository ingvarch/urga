package ui

import (
	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
)

// The palette. Green is the color Nomad carries, it labels what belongs to
// the cluster: borders, titles, headers.
var (
	colorAccent = lipgloss.Color("#00ca8e")
	colorKey    = lipgloss.Color("#26ffe6")
	colorText   = lipgloss.Color("#d0d4d8")
	colorMuted  = lipgloss.Color("#7f868e")
	colorError  = lipgloss.Color("#e06c75")
)

var (
	styleBorder   = lipgloss.NewStyle().Foreground(colorAccent)
	styleLogo     = lipgloss.NewStyle().Foreground(colorAccent)
	styleTitle    = lipgloss.NewStyle().Foreground(colorKey)
	styleLabel    = lipgloss.NewStyle().Foreground(colorAccent)
	styleValue    = lipgloss.NewStyle().Foreground(colorText)
	styleKey      = lipgloss.NewStyle().Foreground(colorKey)
	styleMuted    = lipgloss.NewStyle().Foreground(colorMuted)
	styleError    = lipgloss.NewStyle().Foreground(colorError)
	styleSelected = lipgloss.NewStyle().Foreground(lipgloss.Color("#1c1f24")).Background(colorKey)
)

// tableStyles keeps the table flat: the box around it draws the border, the
// header carries the accent.
func tableStyles() table.Styles {
	return table.Styles{
		Header:   lipgloss.NewStyle().Foreground(colorAccent).Padding(0, 1),
		Cell:     lipgloss.NewStyle().Foreground(colorText).Padding(0, 1),
		Selected: styleSelected,
	}
}
