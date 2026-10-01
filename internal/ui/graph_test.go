package ui

import (
	"fmt"
	"image/color"
	"regexp"
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
		every:   5 * time.Second,
		color:   colorAccent,
		max:     100,
		unit:    percentUnit,
	}
}

// chartTurnRunes are the marks only the line ever draws: the scale and grid
// know turns of their own nowhere.
const chartTurnRunes = "╭╮╰╯"

// sgr is a change of how what follows it is painted.
var sgr = regexp.MustCompile("\x1b\\[([0-9;]*)m")

// What a color paints: the marks, or the cell behind them.
const (
	sgrForeground = 38
	sgrBackground = 48
)

// paintOf is how a change of paint names a color.
func paintOf(layer int, of color.Color) string {
	red, green, blue, _ := of.RGBA()

	return fmt.Sprintf("%d;2;%d;%d;%d", layer, red>>8, green>>8, blue>>8)
}

// drawn is a row with only what is painted in a color left on it. The scale
// and the ruling are drawn with the marks of the line, so the color is what
// tells the line apart.
func drawn(row string, in color.Color) string {
	paint := paintOf(sgrForeground, in)

	out, painted, at := strings.Builder{}, false, 0

	keep := func(text string) {
		if !painted {
			text = strings.Repeat(" ", ansi.StringWidth(text))
		}

		out.WriteString(text)
	}

	for _, found := range sgr.FindAllStringSubmatchIndex(row, -1) {
		keep(row[at:found[0]])

		painted = strings.Contains(row[found[2]:found[3]], paint)
		at = found[1]
	}

	keep(row[at:])

	return out.String()
}

// lineOn is what the line of a chart draws on a row, without the room
// around it.
func lineOn(row string) string {
	return strings.TrimSpace(drawn(row, colorAccent))
}

func TestChart_HasAxes(t *testing.T) {
	r := require.New(t)

	out := plain(strings.Join(aChart(30, 60).render(30, 13), "\n"))

	// What is drawn, the scale it is read against, and how far back it
	// reaches.
	r.Contains(out, "CPU")
	r.Contains(out, "75%")
	r.Contains(out, "50%")
	r.Contains(out, "25%")
	r.Contains(out, "0%")
	r.Contains(out, "2m ago")
	r.Contains(out, "now")

	// The side the scale stands on.
	r.Contains(out, "│")
}

func TestChart_DrawsTheLine(t *testing.T) {
	r := require.New(t)

	out := plain(strings.Join(aChart(10, 50, 90).render(30, 13), "\n"))

	// A rising line turns as it climbs.
	r.True(strings.ContainsAny(out, chartTurnRunes), out)
}

func TestChart_HighReadingsStayOnThePlot(t *testing.T) {
	r := require.New(t)

	rows := aChart(100, 100).render(30, 13)

	// A machine at full load reads on the top row, not past the plot
	// where nothing would draw.
	r.NotEmpty(lineOn(rows[1]), plain(rows[1]))

	// So does a reading past the top of the scale.
	rows = aChart(150, 150).render(30, 13)
	r.NotEmpty(lineOn(rows[1]), plain(rows[1]))
}

func TestChart_NothingUsedSitsOnTheFloor(t *testing.T) {
	r := require.New(t)

	rows := aChart(0).render(30, 13)

	// Nothing running is not the same as nothing read yet: it reads on
	// the floor of the plot, where it belongs.
	r.NotEmpty(lineOn(rows[13]), plain(rows[13]))
	r.Empty(lineOn(rows[12]), plain(rows[12]))
}

func TestChart_AnEmptyChartDrawsOnlyAxes(t *testing.T) {
	r := require.New(t)

	out := plain(strings.Join(aChart().render(30, 13), "\n"))

	// No readings yet is not a reading of nothing: the plot stays empty,
	// and the scale still stands.
	r.False(strings.ContainsAny(out, chartTurnRunes), out)
	r.Contains(out, "0%")
}

func TestChart_TheNewestReadingIsAtTheRight(t *testing.T) {
	r := require.New(t)

	rows := aChart(100).render(30, 13)

	// A chart that has just started grows from the right, the way it moves
	// as the readings come in: one reading is one mark in the last column.
	line := drawn(rows[1], colorAccent)
	r.True(strings.HasSuffix(line, "─"), line)
	r.Len([]rune(strings.TrimSpace(line)), 1, line)
}

func TestChart_KeepsWhatFits(t *testing.T) {
	r := require.New(t)

	// Older than the chart is wide falls off the left: what was first a
	// full load reads no higher than the floor once it no longer fits.
	rows := aChart(100, 0, 0, 0).render(7, 7)

	r.Empty(lineOn(rows[1]), plain(rows[1]))
	r.NotEmpty(lineOn(rows[7]), plain(rows[7]))
}

func TestChart_ThePlotIsRuledBehindTheLine(t *testing.T) {
	r := require.New(t)

	// A reading near the floor leaves the rows above it to the grid, which
	// rules every one of them and is no part of the line.
	rows := aChart(1).render(30, 13)

	for _, row := range rows[1:13] {
		r.Contains(plain(row), "─")
		r.Empty(lineOn(row), plain(row))
	}
}

func TestChart_GridGivesWayToTheLine(t *testing.T) {
	r := require.New(t)

	// A line climbing through the 50% row keeps its cell: the grid shows
	// around it, not through it.
	var half string
	for _, row := range aChart(10, 90).render(30, 13) {
		if strings.Contains(plain(row), "50%") {
			half = row
		}
	}

	r.Contains(plain(half), "─")
	r.Equal("│", lineOn(half))
}

func TestChart_AShortChartKeepsOnlyHalf(t *testing.T) {
	r := require.New(t)

	out := plain(strings.Join(aChart(23).render(30, 7), "\n"))

	// Seven rows carry the half but neither quarter: a label there would
	// stand beside the wrong place.
	r.Contains(out, "50%")
	r.NotContains(out, "75%")
	r.NotContains(out, "25%")
}

func TestChart_ShadesTheAreaUnderTheLine(t *testing.T) {
	r := require.New(t)

	shade := paintOf(sgrBackground, colorPanel)
	rows := aChart(50, 50, 50).render(30, 13)

	// The area beneath the line is the shade of a panel, with no marks of
	// its own, and what is above the line is not shaded.
	r.Contains(rows[13], shade)
	r.NotContains(rows[1], shade)
	r.NotContains(plain(strings.Join(rows, "\n")), "░")

	// No readings shade nothing.
	r.NotContains(strings.Join(aChart().render(30, 13), "\n"), shade)
}

func TestChart_TicksAreEquallySpaced(t *testing.T) {
	r := require.New(t)

	rows := strings.Split(plain(strings.Join(aChart().render(30, 13), "\n")), "\n")
	var tickRows []int
	for i := 1; i <= 13; i++ {
		for _, label := range []string{"100%", "75%", "50%", "25%", "0%"} {
			if strings.HasPrefix(strings.TrimSpace(rows[i]), label) {
				tickRows = append(tickRows, i)
				break
			}
		}
	}

	// 13 rows hold 5 ticks spaced evenly 3 rows apart.
	r.Equal([]int{1, 4, 7, 10, 13}, tickRows)
}

func TestChart_ShortTicksAreEquallySpaced(t *testing.T) {
	r := require.New(t)

	rows := strings.Split(plain(strings.Join(aChart().render(30, 7), "\n")), "\n")
	var tickRows []int
	for i := 1; i <= 7; i++ {
		for _, label := range []string{"100%", "50%", "0%"} {
			if strings.HasPrefix(strings.TrimSpace(rows[i]), label) {
				tickRows = append(tickRows, i)
				break
			}
		}
	}

	// 7 rows hold 3 ticks spaced evenly 3 rows apart.
	r.Equal([]int{1, 4, 7}, tickRows)
}

// tickRows are the rows of a chart that carry a label of the scale, counted
// from the reading at the top.
func tickRows(rows []string) []int {
	var out []int

	for i, row := range rows[1 : len(rows)-1] {
		if strings.Contains(plain(row), "%") {
			out = append(out, i+1)
		}
	}

	return out
}

func TestChart_MarksQuartersWhereTheyFallOnARow(t *testing.T) {
	r := require.New(t)

	// Nine rows are eight steps: every quarter is two of them, and falls
	// on a row of its own.
	r.Equal([]int{1, 3, 5, 7, 9}, tickRows(aChart().render(30, 9)))

	// Fifteen rows are fourteen steps: a quarter falls between two rows,
	// and a label there would stand beside the wrong place. The half does
	// fall on one.
	r.Equal([]int{1, 8, 15}, tickRows(aChart().render(30, 15)))

	// Eight rows are seven steps, and the half falls between two rows as
	// well: only the ends of the scale are marked.
	r.Equal([]int{1, 8}, tickRows(aChart().render(30, 8)))
}

func TestChart_ReadsFromBothEnds(t *testing.T) {
	r := require.New(t)

	head := plain(aChart(30).render(40, 7)[0])

	// What it is and how much of it, on the left; the numbers behind the
	// share, on the right.
	r.True(strings.HasPrefix(head, "CPU  29%"), head)
	r.True(strings.HasSuffix(head, "1160 / 4000 MHz"), head)
	r.Equal(40, ansi.StringWidth(head))
}

func TestChart_IsAsTallAsItIsAsked(t *testing.T) {
	r := require.New(t)

	// The reading, the rows of the plot, and the times.
	r.Len(aChart().render(30, 13), 13+2)
	r.Len(aChart().render(30, 7), 7+2)
}

func TestChart_EveryLineIsExactlyTheWidthItWasAsked(t *testing.T) {
	r := require.New(t)

	for _, width := range []int{8, 10, 17, 30, 64} {
		for _, line := range aChart(100, 50, 25).render(width, 7) {
			r.Equal(width, ansi.StringWidth(plain(line)), "width %d: %q", width, plain(line))
		}
	}
}

func TestChart_TooNarrowForItsScaleDrawsNothing(t *testing.T) {
	r := require.New(t)

	// The scale takes its columns first. Drawing it into less spills out of
	// the box the chart is given, and a plot one column wide holds a mark
	// and no line.
	c := aChart(1)
	r.Equal(len("100%")+1, c.axisWidth())

	r.Empty(c.render(c.axisWidth(), 7))
	r.Empty(c.render(c.axisWidth()+1, 7))
	r.NotEmpty(c.render(c.axisWidth()+2, 7))

	// A wider scale needs a wider chart.
	c.max, c.unit = 4, ghzUnit
	r.Equal(len("4.00 GHz")+1, c.axisWidth())
	r.Empty(c.render(c.axisWidth()+1, 7))
	r.NotEmpty(c.render(c.axisWidth()+2, 7))
}

func TestChart_ReachesAsFarBackAsItsPlotIsWide(t *testing.T) {
	r := require.New(t)

	c := aChart(30)
	c.every = time.Minute

	// Thirty columns less the five of the scale are twenty-five readings.
	rows := c.render(30, 7)
	r.Contains(plain(rows[len(rows)-1]), "25m ago")

	// A wider scale leaves fewer of them.
	c.max, c.unit = 4, ghzUnit

	rows = c.render(30, 7)
	r.Contains(plain(rows[len(rows)-1]), "21m ago")
}

func TestChart_TheLineTakesTheColorOfTheChart(t *testing.T) {
	r := require.New(t)

	// CPU reads green and memory cyan: the color names what the line is.
	cpu := aChart(100, 100).render(30, 13)[1]
	r.NotEmpty(strings.TrimSpace(drawn(cpu, colorAccent)))
	r.Empty(strings.TrimSpace(drawn(cpu, colorTitle)))

	memory := aChart(100, 100)
	memory.color = colorTitle

	mem := memory.render(30, 13)[1]
	r.NotEmpty(strings.TrimSpace(drawn(mem, colorTitle)))
	r.Empty(strings.TrimSpace(drawn(mem, colorAccent)))
}

func TestAmount_NamesAReadingInItsUnit(t *testing.T) {
	r := require.New(t)

	// Percents and small numbers read bare. The big units keep two
	// decimals: with one, a machine that does little reads as doing nothing.
	r.Equal("50%", amount(50, percentUnit))
	r.Equal("500 MHz", amount(500, mhzUnit))
	r.Equal("857 MiB", amount(857, mibUnit))
	r.Equal("3.00 GHz", amount(3, ghzUnit))
	r.Equal("0.84 GiB", amount(0.837, gibUnit))
	r.Equal("0.04 GHz", amount(0.04, ghzUnit))

	r.Equal("0.04 / 4.00 GHz", pair(0.04, 4, ghzUnit))
	r.Equal("400 / 500 MHz", pair(400, 500, mhzUnit))
}

func TestChart_AlignedAxesStandOnTheSameColumn(t *testing.T) {
	r := require.New(t)

	// Megahertz take more room than mebibytes here: sharing the wider
	// scale keeps both axes on the same column, so neither chart shifts
	// when units change and both shrink instead.
	cpu := aChart()
	cpu.values = []float64{1165, 2000}
	cpu.max, cpu.unit = 4000, mhzUnit

	mem := aChart()
	mem.values = []float64{256, 300}
	mem.max, mem.unit = 512, mibUnit

	cpu, mem = alignLabels(cpu, mem)

	cpuRows := cpu.render(40, 13)
	memRows := mem.render(40, 13)

	r.Equal(axisColumn(plain(cpuRows[1])), axisColumn(plain(memRows[1])))
	r.Equal(len("4000 MHz"), axisColumn(plain(cpuRows[1])))
}

// axisColumn is where the side of the scale stands on a plot row.
func axisColumn(row string) int {
	return strings.IndexRune(row, '│')
}

func TestChart_DrawsNumbersAgainstTheirOwnScale(t *testing.T) {
	r := require.New(t)

	cpu := aChart()
	cpu.values = []float64{1165, 2000}
	cpu.max, cpu.unit = 4000, mhzUnit

	out := plain(strings.Join(cpu.render(40, 13), "\n"))

	// The axis counts megahertz to the total, and the line climbs it.
	r.Contains(out, "3000 MHz")
	r.True(strings.ContainsAny(out, chartTurnRunes), out)
}
