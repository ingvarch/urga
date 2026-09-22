package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func TestLogo(t *testing.T) {
	r := require.New(t)

	art := lines(fitLogo(160))

	// The art fits in the header, so it sits next to the cluster info
	// without pushing anything down.
	r.Len(art, len(logo))
	r.LessOrEqual(len(art), headerHeight)

	for _, row := range art {
		r.Equal(logoWidth, ansi.StringWidth(row))
	}

	r.Contains(strings.Join(art, "\n"), "@@@  @@@")
}

func TestLogo_GoesAwayWhenItDoesNotFit(t *testing.T) {
	r := require.New(t)

	// A window that has no room for it loses the art altogether. Drawing the
	// part that fits leaves broken pieces on the screen.
	r.Empty(fitLogo(80))
	r.Empty(fitLogo(minHeaderWidth + logoWidth - 1))

	r.NotEmpty(fitLogo(minHeaderWidth + logoWidth))
}
