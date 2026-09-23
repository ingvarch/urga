package ui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// chartAxisWidth is the scale down the left of a chart: the widest label it
// carries, a space and the side of the plot.
const chartAxisWidth = len("100%") + 2

// chartLevels are the levels of the scale that are drawn across the plot.
// The top and the bottom are the frame itself.
var chartLevels = []int{75, 50, 25}

// chartQuarters is how tall a plot has to be before it is worth marking the
// quarters as well as the half. Below it the lines crowd out the readings.
const chartQuarters = 8

// levels are the lines drawn across a plot of this height.
func levels(height int) []int {
	if height < chartQuarters {
		return []int{50}
	}

	return chartLevels
}

// What a cell of the plot holds: nothing, the top of a reading, the fill
// under that top, a line of the scale in the air, or a line of the scale
// over the fill that covers it.
const (
	cellAir = iota
	cellEdge
	cellFill
	cellLine
	cellOver
)

// chartDash is how often a line of the scale puts a mark down: every other
// column, which reads as a dotted line rather than a drawn one.
const chartDash = 2

// chartHair is what a line of the scale is drawn with. It sits along the
// floor of its cell and is as thin as the border of the screen; the blocks a
// reading is drawn with are far heavier.
const chartHair = '_'

// chartCell is one cell of the plot.
type chartCell struct {
	glyph rune
	kind  int
}

// chart is a reading over time, drawn with a scale on the left and the span
// of time along the bottom.
type chart struct {
	name string

	// reading is the share of the machine this takes, detail the numbers
	// behind it.
	reading string
	detail  string

	// values are the readings, each from 0 to 1, oldest first.
	values []float64

	// window is how far back the left edge of the chart reaches.
	window time.Duration

	// color is what a reading is drawn in.
	color color.Color
}

// render draws the chart into a block of the given width, with a plot of the
// given height. It takes four lines more: what it reads now, the top of the
// scale, the axis under the plot and the times at its ends.
//
// A row of the plot is a band of the scale, so a level falls between two
// rows, not on one. The lines are drawn there, on the edge of a row, and a
// level that falls inside a row is left off the scale: a label there would
// stand beside the wrong place.
func (c chart) render(width, height int) []string {
	plot := max(width-chartAxisWidth, 1)

	rows := []string{
		c.headline(width),
		styleMuted.Render(c.side("100%", "┌") + strings.Repeat("─", plot)),
	}

	for i, row := range c.cells(plot, height) {
		rows = append(rows, styleMuted.Render(c.side(levelAt(i, height), "│"))+c.line(row))
	}

	return append(rows,
		styleMuted.Render(c.side("0%", "└")+strings.Repeat("─", plot)),
		styleMuted.Render(strings.Repeat(" ", chartAxisWidth)+c.times(plot)))
}

// side is the scale down the left of the plot: what this row stands for and
// the edge of the plot next to it.
func (c chart) side(label, edge string) string {
	return padLeft(label, chartAxisWidth-2) + " " + edge
}

// cells are the rows of the plot: the readings, with the lines of the scale
// laid over them.
func (c chart) cells(width, height int) [][]chartCell {
	rows := make([][]chartCell, height)
	for row := range rows {
		rows[row] = make([]chartCell, width)
		for column := range rows[row] {
			rows[row][column] = chartCell{glyph: ' ', kind: cellAir}
		}
	}

	values := c.values
	if len(values) > width {
		values = values[len(values)-width:]
	}

	for i, value := range values {
		c.draw(rows, width-len(values)+i, value, height)
	}

	c.mark(rows, width, height)

	return rows
}

// draw puts one reading up its column, from the floor of the plot. The top
// of it is kept apart from the body: a line of the scale crosses the body,
// and gives way to the top.
func (c chart) draw(rows [][]chartCell, column int, value float64, height int) {
	filled := value * float64(height)
	top := true

	for row := range height {
		part := eighths(filled - float64(height-row-1))
		if part == 0 {
			continue
		}

		if top {
			rows[row][column] = chartCell{glyph: blocks[part], kind: cellEdge}
			top = false

			continue
		}

		rows[row][column] = chartCell{glyph: '█', kind: cellFill}
	}
}

// mark lays the lines of the scale over the readings. A line runs along the
// floor of its row, in the air or over the fill; where the top of a reading
// stands in that cell the line passes behind it, the way a chart is read.
func (c chart) mark(rows [][]chartCell, width, height int) {
	for _, level := range levels(height) {
		at := rowOf(level, height)
		if at < 0 {
			continue
		}

		for column := 0; column < width; column += chartDash {
			switch rows[at][column].kind {
			case cellAir:
				rows[at][column] = chartCell{glyph: chartHair, kind: cellLine}
			case cellFill:
				rows[at][column] = chartCell{glyph: chartHair, kind: cellOver}
			}
		}
	}
}

// rowOf is the row a level of the scale is drawn under, or -1 when the level
// falls inside a row rather than between two of them.
func rowOf(level, height int) int {
	if height < 2 || level*height%100 != 0 {
		return -1
	}

	return height - level*height/100 - 1
}

// levelAt is what the floor of a row stands for, for the rows that carry a
// line of the scale.
func levelAt(row, height int) string {
	for _, level := range levels(height) {
		if rowOf(level, height) == row {
			return fmt.Sprintf("%d%%", level)
		}
	}

	return ""
}

// line paints a row of the plot, in runs of what is drawn the same way: a
// style of its own per cell would be mostly escape codes.
func (c chart) line(row []chartCell) string {
	out := strings.Builder{}

	for at := 0; at < len(row); {
		end := at

		// An empty cell shows nothing but the shade behind it, so it joins
		// whatever run is open unless that run is painted over a reading.
		for end < len(row) && (row[end].kind == row[at].kind ||
			(row[end].glyph == ' ' && row[at].kind != cellOver)) {
			end++
		}

		run := strings.Builder{}
		for _, cell := range row[at:end] {
			run.WriteRune(cell.glyph)
		}

		out.WriteString(c.paint(row[at].kind).Render(run.String()))

		at = end
	}

	return out.String()
}

// paint is how each kind of cell is drawn. A reading is one color from its
// top to the floor of the plot; a line of the scale is a hairline, over the
// shade of the plot or over the reading that covers it.
func (c chart) paint(kind int) lipgloss.Style {
	switch kind {
	case cellLine:
		return chartPaint(chartLine)
	case cellOver:
		return lipgloss.NewStyle().Foreground(chartLine).Background(c.color)
	}

	return chartPaint(c.color)
}

// headline is what the chart says right now: what it is and how much of it
// on the left, the numbers behind that on the right.
func (c chart) headline(width int) string {
	left := styleLabel.Render(c.name) + "  " + styleValue.Render(c.reading)
	right := styleMuted.Render(c.detail)

	gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		return truncate(left, width)
	}

	return left + strings.Repeat(" ", gap) + right
}

// times are the ends of the time axis: how far back the chart reaches, and
// the reading that has just come in.
func (c chart) times(width int) string {
	back := age(c.window) + " ago"

	gap := width - len(back) - len("now")
	if gap < 1 {
		return truncate("now", width)
	}

	return back + strings.Repeat(" ", gap) + "now"
}

// chartPaint is how a chart is drawn: the color of the reading on the shade
// the chart sits on.
func chartPaint(of color.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(of).Background(colorPanel)
}

// padLeft lines a value up against the right of its column.
func padLeft(s string, width int) string {
	if gap := width - len(s); gap > 0 {
		return strings.Repeat(" ", gap) + s
	}

	return s
}

// blocks fill a cell from empty to full, an eighth at a time.
var blocks = []rune(" ▁▂▃▄▅▆▇█")

// eighths is how much of one cell a share of it fills, in eighths.
func eighths(share float64) int {
	return clamp(int(share*8+0.5), 0, 8)
}
