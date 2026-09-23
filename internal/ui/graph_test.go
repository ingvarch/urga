package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func aChart(values ...float64) chart {
	return chart{
		name:    "CPU",
		reading: "29%",
		detail:  "1160 / 4000 MHz",
		values:  values,
		window:  2 * time.Minute,
		color:   colorAccent,
	}
}

// kinds are what each row of a column holds, top to bottom.
func kinds(cells [][]chartCell, column int) []int {
	out := make([]int, 0, len(cells))
	for _, row := range cells {
		out = append(out, row[column].kind)
	}

	return out
}

func TestChart_HasAxes(t *testing.T) {
	r := require.New(t)

	out := plain(strings.Join(aChart(0.3, 0.6).render(30, 8), "\n"))

	// What is drawn, the scale it is read against, and how far back it
	// reaches.
	r.Contains(out, "CPU")
	r.Contains(out, "100%")
	r.Contains(out, "75%")
	r.Contains(out, "50%")
	r.Contains(out, "25%")
	r.Contains(out, "0%")
	r.Contains(out, "2m ago")
	r.Contains(out, "now")

	// The frame: the top of the scale, the axis at the bottom, the side.
	r.Contains(out, "┌")
	r.Contains(out, "└")
	r.Contains(out, "│")
}

func TestChart_DrawsALevelWhereItIs(t *testing.T) {
	r := require.New(t)

	// Half of the scale is the line under the fourth row of eight, so a
	// reading of half reaches it and stops.
	cells := aChart(0.5).cells(1, 8)

	// Three lines cross the column: two in the air above the reading, one
	// over the fill under its top.
	r.Equal([]int{
		cellAir, cellLine, cellAir, cellLine,
		cellEdge, cellOver, cellFill, cellFill,
	}, kinds(cells, 0))
}

func TestChart_DoesNotOverstateAReading(t *testing.T) {
	r := require.New(t)

	// Under a quarter stays under the line that marks a quarter: the line
	// is in the air, the reading below it.
	cells := aChart(0.23).cells(1, 8)

	r.Equal(cellLine, cells[5][0].kind)
	r.Equal(cellEdge, cells[6][0].kind)
	r.NotEqual('█', cells[6][0].glyph)
}

func TestChart_ALineShowsThroughWhatCoversIt(t *testing.T) {
	r := require.New(t)

	cells := aChart(0.95, 0.95).cells(2, 8)

	// Where a reading covers a line, the line is laid over it rather than
	// cut into it: the cell keeps the color of the reading behind a
	// hairline.
	r.Equal(cellOver, cells[3][0].kind)
	r.Equal(chartHair, cells[3][0].glyph)

	over := aChart().paint(cellOver)
	r.Equal(colorAccent, over.GetBackground())
	r.Equal(chartLine, over.GetForeground())
}

func TestChart_ALineIsAHairline(t *testing.T) {
	r := require.New(t)

	cells := aChart(0.1).cells(1, 8)

	// The line is drawn with the thinnest mark there is, not with a block:
	// it reads like the border of the screen, not like a reading.
	r.Equal(chartHair, cells[3][0].glyph)
	r.NotContains(string(blocks), string(chartHair))
}

func TestChart_ALineGivesWayToTheTopOfAReading(t *testing.T) {
	r := require.New(t)

	// A reading whose top lands on the line keeps the cell: the line runs
	// behind what is drawn, it does not cut through it.
	cells := aChart(0.27).cells(1, 8)

	r.Equal(cellEdge, cells[5][0].kind)
}

func TestChart_AReadingIsOneColor(t *testing.T) {
	r := require.New(t)

	c := aChart()

	// The top of a reading and what is under it are the same color: a tall
	// reading must not read as a different one.
	r.Equal(colorAccent, c.paint(cellEdge).GetForeground())
	r.Equal(colorAccent, c.paint(cellFill).GetForeground())
}

func TestChart_TheLinesAreDotted(t *testing.T) {
	r := require.New(t)

	cells := aChart(0.05, 0.05, 0.95, 0.95).cells(4, 8)

	// A line puts a mark down every other column, in the air and over a
	// reading alike.
	r.Equal(cellLine, cells[3][0].kind)
	r.Equal(cellAir, cells[3][1].kind)

	r.Equal(cellOver, cells[3][2].kind)
	r.Equal(cellFill, cells[3][3].kind)
}

func TestChart_TheNewestReadingIsAtTheRight(t *testing.T) {
	r := require.New(t)

	cells := aChart(1).cells(3, 4)

	// A chart that has just started grows from the right, the way it moves
	// as the readings come in.
	r.Equal(cellAir, cells[0][0].kind)
	r.Equal(cellAir, cells[0][1].kind)
	r.Equal(cellEdge, cells[0][2].kind)
}

func TestChart_KeepsWhatFits(t *testing.T) {
	r := require.New(t)

	cells := aChart(0, 1, 1).cells(2, 4)

	// Older than the chart is wide falls off the left.
	r.Equal(cellEdge, cells[0][0].kind)
	r.Equal(cellEdge, cells[0][1].kind)
}

func TestChart_MarksOnlyWhatFallsOnALine(t *testing.T) {
	r := require.New(t)

	out := plain(strings.Join(aChart(0.23).render(30, 5), "\n"))

	// Five rows cannot carry a quarter or a half: those fall inside a row,
	// and a label there would stand beside the wrong place.
	r.Contains(out, "100%")
	r.Contains(out, "0%")
	r.NotContains(out, "25%")
	r.NotContains(out, "50%")
}

func TestChart_AShortChartKeepsOnlyHalf(t *testing.T) {
	r := require.New(t)

	out := plain(strings.Join(aChart(0.23).render(30, 4), "\n"))

	// Four rows with three lines across them are more scale than chart.
	r.Contains(out, "50%")
	r.NotContains(out, "75%")
	r.NotContains(out, "25%")
}

func TestChart_ReadsFromBothEnds(t *testing.T) {
	r := require.New(t)

	head := plain(aChart(0.3).render(40, 4)[0])

	// What it is and how much of it, on the left; the numbers behind the
	// share, on the right.
	r.True(strings.HasPrefix(head, "CPU  29%"), head)
	r.True(strings.HasSuffix(head, "1160 / 4000 MHz"), head)
	r.Equal(40, ansi.StringWidth(head))
}

func TestChart_IsAsTallAsItIsAsked(t *testing.T) {
	r := require.New(t)

	// The reading, the top of the scale, the rows, the axis and the times.
	r.Len(aChart().render(30, 8), 8+4)
	r.Len(aChart().render(30, 4), 4+4)
}

func TestChart_EveryLineIsTheSameWidth(t *testing.T) {
	r := require.New(t)

	for _, line := range aChart(1, 0.5).render(30, 4) {
		r.LessOrEqual(ansi.StringWidth(plain(line)), 30)
	}
}

func TestChart_SitsOnAShadeOfItsOwn(t *testing.T) {
	r := require.New(t)

	// The area the chart covers reads as one block against the screen, so
	// an empty column is still part of the chart.
	r.Equal(colorPanel, chartPaint(colorAccent).GetBackground())
	r.NotEqual(colorPanel, colorAccent)
}
