package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// modalPadX is the space between the text of a dialog and its border.
const modalPadX = 3

// modal draws a box over what is already on the screen, under the line it is
// about. What the box does not cover stays visible: a question about one row
// is no reason to hide the rest.
func modal(base, box string, near int) string {
	under := strings.Split(base, "\n")
	over := strings.Split(box, "\n")

	if len(over) == 0 || len(under) == 0 {
		return base
	}

	// A space on each side, so the rows behind do not run into the border.
	width := blockWidth(box) + 2
	left := max((blockWidth(base)-width)/2, 0)
	top := placeBelow(near, len(over), len(under))

	for i, line := range over {
		row := top + i
		if row >= len(under) {
			break
		}

		under[row] = spliceLine(under[row], " "+line+" ", left, width)
	}

	return strings.Join(under, "\n")
}

// placeBelow puts the box right under the line it is about, or above it when
// there is no room, and always inside the screen.
func placeBelow(near, height, screen int) int {
	// The first and the last line are the border of what is underneath.
	first, last := 1, screen-1-height

	if last < first {
		return max(first, 0)
	}

	if near < 0 {
		return (screen - height) / 2
	}

	top := near + 1
	if top > last {
		top = near - height
	}

	return max(first, min(top, last))
}

// spliceLine puts a piece of one line inside another, keeping both sides of
// it and the width of the whole.
func spliceLine(under, over string, at, width int) string {
	before := ansi.Truncate(under, at, "")
	after := ansi.TruncateLeft(under, at+width, "")

	return pad(before, at) + over + after
}

// dialog is a question in a box of its own.
func dialog(title string, lines []string, maxWidth int) string {
	width := 0
	for _, line := range lines {
		width = max(width, ansi.StringWidth(line))
	}

	width = min(width+2*modalPadX+2, maxWidth)

	// A blank line above and below the words, then the two border lines.
	body := "\n" + center(lines, width-2) + "\n"
	height := len(lines) + 2 + 2

	return frame(title, body, width, height)
}
