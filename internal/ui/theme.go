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

	// colorSurface is the shade of a control that sits on the background: a
	// button the cursor is not on.
	colorSurface = lipgloss.Color("#3a4149")

	// colorPanel is the shade a chart sits on, so the area it covers reads
	// as one block against the screen.
	colorPanel = lipgloss.Color("#262b31")

	// colorMark is a row an action is to take. It outranks the color the
	// row carries for its state: a mark is what the eye is looking for.
	colorMark = lipgloss.Color("#d7a13b")

	// chartLine is the scale drawn across a chart: quiet enough to read as
	// a hairline, whether it crosses the air or a reading.
	chartLine = lipgloss.Color("#525a63")
)

// Row colors say what state a resource is in, at a glance down the list.
var (
	colorAttention = lipgloss.Color("#d98b6a")
	colorPending   = lipgloss.Color("#e5c07b")
	colorDead      = lipgloss.Color("#e06c75")
	colorSpent     = lipgloss.Color("#6b7178")

	// colorCanary is an allocation put out to try a change before the rest.
	colorCanary = lipgloss.Color("#c678dd")
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

	// A line a diff adds, and one it takes away.
	styleAdded   = lipgloss.NewStyle().Foreground(colorAccent)
	styleDeleted = lipgloss.NewStyle().Foreground(colorDead)

	// What a plan would do to allocations, each kind in a colour of its own.
	stylePending     = lipgloss.NewStyle().Foreground(colorPending)
	styleDestructive = lipgloss.NewStyle().Foreground(colorAttention)
	styleCanary      = lipgloss.NewStyle().Foreground(colorCanary)

	styleTableHeader = lipgloss.NewStyle().Foreground(colorAccent)

	// styleMatch lights up what a filter matched inside a line of text.
	styleMatch = lipgloss.NewStyle().Foreground(lipgloss.Color("#1c1f24")).Background(colorLabel)

	// The row under the cursor is painted end to end, so it reads whatever
	// color the resource itself has.
	styleSelected = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#1c1f24")).
			Background(colorActive)

	// A marked row under the cursor is painted in the color of the mark:
	// standing on a mark must not hide it.
	styleSelectedMark = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#1c1f24")).
				Background(colorMark)

	// A button of a dialog. The one under the cursor is filled, which is the
	// only thing that tells the two of them apart.
	styleButton = lipgloss.NewStyle().
			Foreground(colorText).
			Background(colorSurface)

	styleButtonOn = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#1c1f24")).
			Background(colorActive).
			Bold(true)

	// styleButtonOff is a button that does nothing, and says why.
	styleButtonOff = lipgloss.NewStyle().
			Foreground(colorMuted).
			Background(colorPanel)
)
