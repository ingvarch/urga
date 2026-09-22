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
	r.Contains(rows[1], "Urga Rev:")
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
	r.Len(rows, headerHeight)
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

func TestHeader_LogoOnTheRight(t *testing.T) {
	r := require.New(t)

	out := renderHeader(header{address: "https://nmd.1ly.dev", version: "v0.1.0-dev"}, 140)
	rows := lines(out)

	// The art sits at the right edge, the cluster info keeps the left.
	r.Contains(rows[0], "@@@  @@@")
	r.True(strings.HasPrefix(rows[0], "Address:"))

	for i, row := range rows {
		r.Equal(140, ansi.StringWidth(row), "line %d", i)
	}
}

func TestHeader_WithoutRoomForTheLogo(t *testing.T) {
	r := require.New(t)

	out := renderHeader(header{address: "https://nmd.1ly.dev"}, 80)

	// No art at all, and no piece of it either.
	r.NotContains(out, "@")
	r.Len(lines(out), headerHeight)
}

func TestHeader_LeavesTheNamespaceToTheKeys(t *testing.T) {
	r := require.New(t)

	out := renderHeader(header{
		address:    "https://nmd.1ly.dev",
		namespace:  "production",
		namespaces: []namespaceKey{{Key: "<1>", Name: "production", Active: true}},
	}, 140)

	// The namespace in use is the lit up key, saying it twice in one header
	// is a line wasted.
	r.NotContains(plain(out), "Namespace:")
	r.Contains(plain(out), "<1> production")
}

func TestHeader_IsAsTallAsWhatItHolds(t *testing.T) {
	r := require.New(t)

	// The art and the cluster info both fit, whichever of them is taller.
	r.GreaterOrEqual(headerHeight, len(logo))
	r.GreaterOrEqual(headerHeight, infoRows)
}

func TestHeader_ShowsWhatTheClusterIsUsing(t *testing.T) {
	r := require.New(t)

	out := renderHeader(header{usage: "15%", memory: "31%"}, 140)
	rows := lines(out)

	r.Contains(rows[3], "CPU:")
	r.Contains(rows[3], "15%")
	r.Contains(rows[4], "MEM:")
	r.Contains(rows[4], "31%")
}

func TestHeader_BeforeTheClusterAnswers(t *testing.T) {
	r := require.New(t)

	// Nothing is known yet, and the header says that rather than zero.
	out := renderHeader(header{}, 140)

	r.Contains(lines(out)[3], "n/a")
	r.Contains(lines(out)[2], "n/a")
}
