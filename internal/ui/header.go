package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// infoRows is how many lines the cluster info takes.
const infoRows = 6

// unknown is what a value reads as before the cluster has answered.
const unknown = "n/a"

// headerHeight is fixed so that the list below never moves, whatever the
// header has to show. It holds both the info and the art in the corner.
var headerHeight = max(len(logo), infoRows)

// hint is one key of the open screen.
type hint struct {
	Key         string
	Description string
}

// namespaceKey is one namespace the number keys switch to.
type namespaceKey struct {
	Key    string
	Name   string
	Active bool
}

// header is what the top of the screen shows: where we are, which namespace
// a number key switches to, and what this screen can do. Keys that work
// everywhere are not here, they are in help.
type header struct {
	address      string
	version      string
	nomadVersion string
	namespace    string
	usage        string
	memory       string
	namespaces   []namespaceKey
	hints        []hint
}

func renderHeader(h header, width int) string {
	art := fitLogo(width)

	// What is left for the cluster info and the keys once the art has its
	// corner.
	rest := width
	if art != "" {
		rest = width - logoWidth
	}

	info := infoColumn(h, infoWidth(rest))
	keys := namespaceColumn(h.namespaces)

	// Every column is a block of the same width from top to bottom, so the
	// one next to it starts where it says it does.
	infoWide, keysWide := blockWidth(info), blockWidth(keys)

	taken := infoWide + keysWide + 2*columnGap
	hints := hintColumns(h.hints, rest-taken)

	rows := make([]string, 0, headerHeight)
	for i := 0; i < headerHeight; i++ {
		row := pad(lineAt(info, i), infoWide)

		if keys != "" {
			row += strings.Repeat(" ", columnGap) + pad(lineAt(keys, i), keysWide)
		}

		if hint := lineAt(hints, i); hint != "" {
			row += strings.Repeat(" ", columnGap) + hint
		}

		row = ansi.Truncate(strings.TrimRight(row, " "), rest, "…")

		// The art is shorter than the header, the lines past it keep the
		// block square.
		if art != "" {
			row = pad(row, rest) + pad(lineAt(art, i), logoWidth)
		}

		rows = append(rows, row)
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
		{"Nomad Rev:", orUnknown(h.nomadVersion)},
		{"Namespace:", namespaceLabel(h.namespace)},
		{"CPU:", orUnknown(h.usage)},
		{"MEM:", orUnknown(h.memory)},
	}

	out := make([]string, 0, len(rows))
	for _, row := range rows {
		label := styleLabel.Render(pad(row.label, labelWidth))
		value := styleValue.Render(truncate(row.value, max(width-labelWidth, 1)))

		out = append(out, label+value)
	}

	return strings.Join(out, "\n")
}

// namespaceColumn is the list of namespaces a number key switches to. The one
// in use is lit up.
func namespaceColumn(keys []namespaceKey) string {
	if len(keys) == 0 {
		return ""
	}

	keyWidth, nameWidth := 0, 0
	for _, key := range keys {
		keyWidth = max(keyWidth, ansi.StringWidth(key.Key))
		nameWidth = max(nameWidth, ansi.StringWidth(key.Name))
	}

	cells := make([]string, 0, len(keys))
	for _, key := range keys {
		name := styleMuted.Render(pad(key.Name, nameWidth))
		if key.Active {
			name = styleKey.Render(pad(key.Name, nameWidth))
		}

		cells = append(cells, styleLabel.Render(pad(key.Key, keyWidth))+" "+name)
	}

	return grid(cells, headerHeight)
}

// grid lays cells into columns of the given height, the way a key list reads:
// down the column first, then on to the next one.
func grid(cells []string, height int) string {
	rows := make([]string, height)

	for i, cell := range cells {
		row := i % height

		if rows[row] != "" {
			rows[row] += strings.Repeat(" ", columnGap)
		}

		rows[row] += cell
	}

	return strings.Join(rows, "\n")
}

// hintColumns lays the keys of the screen out the same way.
func hintColumns(hints []hint, width int) string {
	if len(hints) == 0 || width <= 0 {
		return ""
	}

	keyWidth, descriptionWidth := 0, 0
	for _, h := range hints {
		keyWidth = max(keyWidth, ansi.StringWidth(h.Key))
		descriptionWidth = max(descriptionWidth, ansi.StringWidth(h.Description))
	}

	cells := make([]string, 0, len(hints))
	for _, h := range hints {
		cells = append(cells,
			styleKey.Render(pad(h.Key, keyWidth))+" "+styleValue.Render(pad(h.Description, descriptionWidth)))
	}

	rows := strings.Split(grid(cells, headerHeight), "\n")
	for i, row := range rows {
		rows[i] = ansi.Truncate(row, width, "…")
	}

	return strings.Join(rows, "\n")
}

// orUnknown is a value the cluster has not answered with yet.
func orUnknown(value string) string {
	if value == "" {
		return unknown
	}

	return value
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

// blockWidth is the width of the widest line of a block.
func blockWidth(block string) int {
	if block == "" {
		return 0
	}

	width := 0
	for _, line := range strings.Split(block, "\n") {
		width = max(width, ansi.StringWidth(line))
	}

	return width
}
