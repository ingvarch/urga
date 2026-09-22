package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// headerHeight is fixed so that the list below never moves, whatever the
// header has to show.
const headerHeight = 3

// hint is one key of the open screen.
type hint struct {
	Key         string
	Description string
}

// header is what the top of the screen shows: where we are and what this
// screen can do. Keys that work everywhere are not here, they are in help.
type header struct {
	address      string
	version      string
	nomadVersion string
	hints        []hint
}

func renderHeader(h header, width int) string {
	info := infoColumn(h, infoWidth(width))
	hints := hintColumns(h.hints, width-ansi.StringWidth(firstLine(info))-columnGap)

	rows := make([]string, 0, headerHeight)
	for i := 0; i < headerHeight; i++ {
		row := lineAt(info, i)
		if hint := lineAt(hints, i); hint != "" {
			row += strings.Repeat(" ", columnGap) + hint
		}

		rows = append(rows, ansi.Truncate(row, width, "…"))
	}

	return strings.Join(rows, "\n")
}

const (
	columnGap  = 2
	labelWidth = 11
)

func infoWidth(width int) int {
	return min(width/2, 46)
}

// infoColumn is the cluster the session talks to.
func infoColumn(h header, width int) string {
	rows := []struct {
		label string
		value string
	}{
		{"Address:", h.address},
		{"urga Rev:", h.version},
		{"Nomad Rev:", h.nomadVersion},
	}

	out := make([]string, 0, len(rows))
	for _, row := range rows {
		label := styleLabel.Render(pad(row.label, labelWidth))
		value := styleValue.Render(truncate(row.value, max(width-labelWidth, 1)))

		out = append(out, label+value)
	}

	return strings.Join(out, "\n")
}

// hintColumns lays the keys out in columns of headerHeight rows, the way a
// key list reads.
func hintColumns(hints []hint, width int) string {
	if len(hints) == 0 || width <= 0 {
		return ""
	}

	keyWidth, descriptionWidth := 0, 0
	for _, h := range hints {
		keyWidth = max(keyWidth, ansi.StringWidth(h.Key))
		descriptionWidth = max(descriptionWidth, ansi.StringWidth(h.Description))
	}

	rows := make([]string, headerHeight)
	for i, h := range hints {
		row := i % headerHeight

		if rows[row] != "" {
			rows[row] += strings.Repeat(" ", columnGap)
		}

		rows[row] += styleKey.Render(pad(h.Key, keyWidth)) +
			" " + styleValue.Render(pad(h.Description, descriptionWidth))
	}

	for i, row := range rows {
		rows[i] = ansi.Truncate(row, width, "…")
	}

	return strings.Join(rows, "\n")
}

func pad(s string, width int) string {
	if gap := width - ansi.StringWidth(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}

	return s
}

func lineAt(block string, i int) string {
	rows := strings.Split(block, "\n")
	if i >= len(rows) {
		return ""
	}

	return rows[i]
}

func firstLine(block string) string {
	return lineAt(block, 0)
}
