package ui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/ingvarch/pulse"
)

// chartPlotMin is the narrowest plot that is drawn. A column holds one
// reading, and one reading is a mark: a line takes two.
const chartPlotMin = 2

// chart is a reading over time, drawn with a scale on the left and the span
// of time along the bottom.
type chart struct {
	name string

	// reading is the share of the machine this takes, detail the numbers
	// behind it.
	reading string
	detail  string

	// values are the readings in the units of the scale, oldest first.
	values []float64

	// every is the time between two readings, which is what one column of
	// the plot stands for: the left edge of the chart reaches as far back
	// as the plot is wide.
	every time.Duration

	// color is what a reading is drawn in.
	color color.Color

	// max is what the top of the scale stands for, unit what it counts in.
	max  float64
	unit string

	// labelWidth is how wide the scale down the left reads. Charts next
	// to each other share the wider one, so neither shifts when units
	// change and both shrink instead.
	labelWidth int
}

// Units a chart reads in: shares of a hundred, megahertz and mebibytes,
// and gigahertz and gibibytes past a thousand of those.
const (
	percentUnit = "%"
	mhzUnit     = "MHz"
	mibUnit     = "MiB"
	ghzUnit     = "GHz"
	gibUnit     = "GiB"
)

// render draws the chart into a block of the given width, with a plot of the
// given height. It takes two lines more: what it reads now and the times at
// the ends of the plot.
func (c chart) render(width, height int) []string {
	// The scale takes its columns whatever it is given. Less than a line
	// of plot left is not a chart, and drawing it anyway spills out of the
	// box.
	plot := width - c.axisWidth()
	if plot < chartPlotMin || height < 1 {
		return nil
	}

	rows := []string{pad(c.headline(width), width)}
	rows = append(rows, strings.Split(c.plot(plot, height), "\n")...)

	return append(rows,
		styleMuted.Render(strings.Repeat(" ", c.axisWidth())+pad(c.times(plot), plot)))
}

// axisWidth is the scale down the left of a chart: the widest label it
// carries and the side of the plot.
func (c chart) axisWidth() int {
	return c.axisLabelWidth() + 1
}

// axisLabelWidth is how wide the scale of the chart reads: the top of it
// names the longest label, and a chart next to another takes the wider of
// the two.
func (c chart) axisLabelWidth() int {
	return max(c.labelWidth, len(amount(c.max, c.unit)))
}

// plot is the line of the readings over time, newest at the right, on the
// scale of their own units.
func (c chart) plot(width, height int) string {
	ch := pulse.New(width, height,
		pulse.WithRange(0, c.max),
		pulse.WithTicks(c.ticks(height)...),
		pulse.WithLabelFormatter(func(value float64) string { return amount(value, c.unit) }),
		pulse.WithLabelWidth(c.axisLabelWidth()),
		pulse.WithAxisStyle(styleMuted),
		// CPU reads green and memory cyan, the way the rest of the screen
		// names them.
		pulse.WithLineWidth(1),
		pulse.WithLineStyle(lipgloss.NewStyle().Foreground(c.color)),
		// What is under the line is the shade of a panel, with no marks
		// of its own.
		pulse.WithFill(true),
		pulse.WithTintedFill(true),
		pulse.WithSolidFill(true),
		pulse.WithTintColor(colorPanel),
	)

	for _, value := range c.values {
		ch.Push(value)
	}

	return ch.View()
}

// ticks are the levels of the scale a plot of this height marks. The rows
// of a plot are one more than the steps between them, so a level is marked
// only where it falls on a row: a label anywhere else would stand beside
// the wrong place.
func (c chart) ticks(height int) []float64 {
	steps := height - 1

	switch {
	case steps%4 == 0:
		return []float64{0, c.max * 0.25, c.max * 0.5, c.max * 0.75, c.max}
	case steps%2 == 0:
		return []float64{0, c.max * 0.5, c.max}
	}

	return []float64{0, c.max}
}

// alignLabels gives both charts the wider scale, so their axes stand
// aligned and neither shifts when units change: both shrink instead.
func alignLabels(cpu, memory chart) (chart, chart) {
	width := max(cpu.axisLabelWidth(), memory.axisLabelWidth())
	cpu.labelWidth, memory.labelWidth = width, width

	return cpu, memory
}

// amount names a reading in its unit: percents and small numbers read
// bare, thousands with two decimals. One decimal reads the little an idle
// machine uses as nothing.
func amount(value float64, unit string) string {
	switch unit {
	case percentUnit:
		return fmt.Sprintf("%.0f%%", value)
	case ghzUnit, gibUnit:
		return fmt.Sprintf("%.2f %s", value, unit)
	default:
		return fmt.Sprintf("%.0f %s", value, unit)
	}
}

// pair names a reading against its total, the unit once behind both.
func pair(value, total float64, unit string) string {
	switch unit {
	case ghzUnit, gibUnit:
		return fmt.Sprintf("%.2f / %.2f %s", value, total, unit)
	default:
		return fmt.Sprintf("%.0f / %.0f %s", value, total, unit)
	}
}

// headline is what the chart shows right now: what it is and how much of it
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

// times are padded to the plot so that the line under the chart is as wide
// as the chart itself.

// times are the ends of the time axis: how far back the chart reaches, and
// the reading that has just come in.
func (c chart) times(width int) string {
	back := age(time.Duration(width)*c.every) + " ago"

	gap := width - len(back) - len("now")
	if gap < 1 {
		return truncate("now", width)
	}

	return back + strings.Repeat(" ", gap) + "now"
}
