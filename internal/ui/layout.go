package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// frame draws a box of exactly width by height with the title in its top
// border. The body is cut to fit, a line that wraps shifts everything below
// it.
func frame(title, body string, width, height int) string {
	if width < 2 || height < 2 {
		return ""
	}

	inner := width - 2

	rows := make([]string, 0, height)
	rows = append(rows, styleBorder.Render("╭")+titleBorder(title, inner)+styleBorder.Render("╮"))

	side := styleBorder.Render("│")
	for _, line := range bodyLines(body, inner, height-2) {
		rows = append(rows, side+line+side)
	}

	rows = append(rows, styleBorder.Render("╰"+strings.Repeat("─", inner)+"╯"))

	return strings.Join(rows, "\n")
}

// indent moves a block in from the left edge.
func indent(block string, by int) string {
	prefix := strings.Repeat(" ", by)

	rows := strings.Split(block, "\n")
	for i, row := range rows {
		rows[i] = prefix + row
	}

	return strings.Join(rows, "\n")
}

// titleBorder centers the title in a line of dashes.
func titleBorder(title string, width int) string {
	if title == "" {
		return styleBorder.Render(strings.Repeat("─", width))
	}

	label := " " + title + " "
	if ansi.StringWidth(label) > width {
		label = truncate(label, width)
	}

	left := (width - ansi.StringWidth(label)) / 2
	right := width - ansi.StringWidth(label) - left

	return styleBorder.Render(strings.Repeat("─", left)) +
		styleTitle.Render(label) +
		styleBorder.Render(strings.Repeat("─", right))
}

// bodyLines cuts the body to the box and pads what is short, so that every
// line is the same width.
func bodyLines(body string, width, height int) []string {
	rows := strings.Split(body, "\n")

	out := make([]string, 0, height)
	for i := 0; i < height; i++ {
		line := ""
		if i < len(rows) {
			line = truncate(rows[i], width)
		}

		out = append(out, line+strings.Repeat(" ", width-ansi.StringWidth(line)))
	}

	return out
}

// center puts lines in the middle of a block of the given width.
func center(lines []string, width int) string {
	out := make([]string, 0, len(lines))

	for _, line := range lines {
		gap := max((width-ansi.StringWidth(line))/2, 0)
		out = append(out, strings.Repeat(" ", gap)+line)
	}

	return strings.Join(out, "\n")
}
