package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// infoRows is how many lines the cluster info takes.
const infoRows = 7

// unknown is what a value reads as before the cluster has answered.
const unknown = "n/a"

// everyDatacenter is what no datacenter chosen reads as.
const everyDatacenter = "all"

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
	cluster      string
	address      string
	region       string
	datacenter   string
	version      string
	nomadVersion string
	usage        string
	memory       string
	namespaces   []namespaceKey
	hints        []hint

	// readOnly says the session changes nothing in the cluster.
	readOnly bool
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

	// One more gap so the keys never touch the art in the corner.
	taken := infoWide + keysWide + 3*columnGap

	// The keys come before the art: when they do not all fit beside it, it
	// gives its place up to them. The info column keeps the width it had,
	// or it would take that place instead.
	if art != "" && hintsWidth(h.hints) > rest-taken {
		art, rest = "", width
	}

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

		row = truncate(strings.TrimRight(row, " "), rest)

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

// infoRow is a row of the info column.
type infoRow struct {
	label string
	value string

	// mark follows the value, which is cut at the width of the column
	// as ever: the column grows by the mark instead of the value
	// giving way to it.
	mark string
}

// infoColumn is the cluster the session talks to.
func infoColumn(h header, width int) string {
	rows := []infoRow{
		where(h),
		{"Region:", orUnknown(h.region), ""},
		{"DC:", orEvery(h.datacenter), ""},
		{"Urga Rev:", h.version, ""},
		{"Nomad Rev:", orUnknown(h.nomadVersion), ""},
		{"CPU:", orUnknown(h.usage), ""},
		{"MEM:", orUnknown(h.memory), ""},
	}

	out := make([]string, 0, len(rows))
	for _, row := range rows {
		label := styleLabel.Render(pad(row.label, labelWidth))
		value := styleValue.Render(truncate(row.value, max(width-labelWidth, 1))) + styleWarn.Render(row.mark)

		out = append(out, label+value)
	}

	return strings.Join(out, "\n")
}

// where is the first row of the header: the cluster the session talks to.
// A cluster the settings name is called by its name first, which tells prod
// from dev at a glance.
func where(h header) infoRow {
	if h.cluster != "" {
		return infoRow{"Cluster:", h.cluster + "  " + h.address, readOnlyMark(h.readOnly)}
	}

	return infoRow{"Address:", h.address, readOnlyMark(h.readOnly)}
}

// readOnlyMark says, next to the address, that the session changes nothing
// in that cluster.
func readOnlyMark(readOnly bool) string {
	if !readOnly {
		return ""
	}

	return " read-only"
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

// hintCells are a list of keys laid out as a block of even columns: the key
// in one, what it does in the next.
func hintCells(hints []hint, describe lipgloss.Style) []string {
	keyWidth, descriptionWidth := 0, 0
	for _, h := range hints {
		keyWidth = max(keyWidth, ansi.StringWidth(h.Key))
		descriptionWidth = max(descriptionWidth, ansi.StringWidth(h.Description))
	}

	cells := make([]string, 0, len(hints))
	for _, h := range hints {
		cells = append(cells,
			styleKey.Render(pad(h.Key, keyWidth))+" "+describe.Render(pad(h.Description, descriptionWidth)))
	}

	return cells
}

// hintColumns lays the keys of the screen out for the header.
// hintsWidth is what the keys take laid out whole.
func hintsWidth(hints []hint) int {
	if len(hints) == 0 {
		return 0
	}

	return blockWidth(grid(hintCells(hints, styleValue), headerHeight))
}

func hintColumns(hints []hint, width int) string {
	if len(hints) == 0 || width <= 0 {
		return ""
	}

	rows := strings.Split(grid(hintCells(hints, styleValue), headerHeight), "\n")
	for i, row := range rows {
		rows[i] = truncate(row, width)
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

// orEvery is a datacenter, or every one of them when none was chosen.
func orEvery(datacenter string) string {
	if datacenter == "" {
		return everyDatacenter
	}

	return datacenter
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
