package ui

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/settings"
)

func TestClusterColours_EveryOneTheSettingsTake(t *testing.T) {
	// A colour the file takes and urga cannot paint would leave prod green.
	for _, name := range settings.Colours {
		_, ok := clusterColours[name]
		require.True(t, ok, name)
	}

	require.Len(t, clusterColours, len(settings.Colours))
}

// paintedIn is a model of the jobs of a cluster the settings paint.
func paintedIn(colour string) Model {
	m := New(&fakeClient{}, Options{Cluster: "prod", Color: colour, Version: "v-test"})
	m, _ = m.update(sizeMsg())

	return m
}

func TestFrame_InTheColourOfTheCluster(t *testing.T) {
	r := require.New(t)

	red := opening(lipgloss.NewStyle().Foreground(clusterColours["red"]))
	out := paintedIn("red").render()

	// The box around the screen and the name of the cluster: seen out of
	// the corner of an eye while reading the rows.
	r.Contains(out, red+"╭")
	r.Contains(out, red+"╰")
	r.Contains(out, red+"│")
	r.Contains(out, red+"prod")
}

func TestFrame_WithoutAColour(t *testing.T) {
	r := require.New(t)

	out := paintedIn("").render()

	r.Contains(out, opening(styleBorder)+"╭")
	r.Contains(out, opening(styleValue)+"prod")
}

func TestFrame_ADialogKeepsItsOwn(t *testing.T) {
	r := require.New(t)

	m := paintedIn("red")
	m, _ = m.update(key(':'))

	// The line and the questions are urga's, not the cluster's.
	r.Equal(1, countOf(m.render(), opening(styleBorder)+"╭"))
}

// countOf is how many times a piece is in a string.
func countOf(s, piece string) int {
	count := 0
	for i := 0; i+len(piece) <= len(s); i++ {
		if s[i:i+len(piece)] == piece {
			count++
		}
	}

	return count
}
