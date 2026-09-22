package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func TestHeader(t *testing.T) {
	r := require.New(t)

	out := renderHeader(header{
		address:      "https://nmd.1ly.dev",
		version:      "v0.1.0-dev",
		nomadVersion: "1.11.1",
	}, 80)

	rows := lines(out)

	r.Contains(rows[0], "Address:")
	r.Contains(rows[0], "https://nmd.1ly.dev")
	r.Contains(rows[1], "urga Rev:")
	r.Contains(rows[1], "v0.1.0-dev")
	r.Contains(rows[2], "Nomad Rev:")
	r.Contains(rows[2], "1.11.1")

	for i, row := range rows {
		r.LessOrEqual(ansi.StringWidth(row), 80, "line %d", i)
	}
}

func TestHeader_LongAddressIsEaten(t *testing.T) {
	r := require.New(t)

	long := "https://" + strings.Repeat("nomad-cluster.", 10) + "example.com"

	out := renderHeader(header{address: long}, 60)
	rows := lines(out)

	// A value that does not fit is cut. Wrapping it pushes the rest of the
	// header down and the screen jumps.
	r.Len(rows, 3)
	r.LessOrEqual(ansi.StringWidth(rows[0]), 60)
	r.Contains(rows[0], "…")
}

func TestHeader_ShowsTheKeysOfTheScreen(t *testing.T) {
	r := require.New(t)

	out := renderHeader(header{
		address: "https://nmd.1ly.dev",
		hints: []hint{
			{Key: "<enter>", Description: "Allocations"},
			{Key: "<r>", Description: "Restart"},
		},
	}, 80)

	rows := lines(out)

	// The keys of the open resource sit next to the cluster info.
	r.Contains(rows[0], "<enter>")
	r.Contains(rows[0], "Allocations")
	r.Contains(rows[1], "<r>")
	r.Contains(rows[1], "Restart")
}

func TestHeader_Height(t *testing.T) {
	r := require.New(t)

	// The header is the same height whatever it holds, so the list below it
	// does not move.
	r.Len(lines(renderHeader(header{}, 80)), headerHeight)

	many := []hint{{Key: "<a>"}, {Key: "<b>"}, {Key: "<c>"}, {Key: "<d>"}, {Key: "<e>"}}
	r.Len(lines(renderHeader(header{hints: many}, 80)), headerHeight)
}
