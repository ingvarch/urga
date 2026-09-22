package ui

import "charm.land/lipgloss/v2"

// The palette. Green carries the cluster: borders, values, the header of a
// table. Cyan marks what is named or in use. Lime labels a field.
var (
	colorAccent = lipgloss.Color("#00b57c")
	colorTitle  = lipgloss.Color("#26ffe6")
	colorLabel  = lipgloss.Color("#baff26")
	colorText   = lipgloss.Color("#cccccc")
	colorMuted  = lipgloss.Color("#7f868e")
	colorActive = lipgloss.Color("#b3f1ff")
)

// Row colors say what state a resource is in, at a glance down the list.
var (
	colorAttention = lipgloss.Color("#d98b6a")
	colorPending   = lipgloss.Color("#e5c07b")
	colorDead      = lipgloss.Color("#e06c75")
	colorSpent     = lipgloss.Color("#6b7178")
)

var (
	styleBorder = lipgloss.NewStyle().Foreground(colorAccent)
	styleLogo   = lipgloss.NewStyle().Foreground(colorAccent)
	styleTitle  = lipgloss.NewStyle().Foreground(colorTitle)
	styleLabel  = lipgloss.NewStyle().Foreground(colorLabel)
	styleValue  = lipgloss.NewStyle().Foreground(colorAccent)
	styleKey    = lipgloss.NewStyle().Foreground(colorTitle)
	styleText   = lipgloss.NewStyle().Foreground(colorText)
	styleMuted  = lipgloss.NewStyle().Foreground(colorMuted)
	styleWarn   = lipgloss.NewStyle().Foreground(colorAttention)
	styleError  = lipgloss.NewStyle().Foreground(colorDead)

	styleTableHeader = lipgloss.NewStyle().Foreground(colorAccent)

	// The row under the cursor is painted end to end, so it reads whatever
	// color the resource itself has.
	styleSelected = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#1c1f24")).
			Background(colorActive)
)
