package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// logo is the name, in the corner where the eye is not reading anything else.
var logo = []string{
	`@@@  @@@ @@@@@@@   @@@@@@@   @@@@@@  `,
	`@@!  @@@ @@!  @@@ !@@       @@!  @@@ `,
	`@!@  !@! @!@!!@!  !@! @!@!@ @!@!@!@! `,
	`!!:  !!! !!: :!!  :!!   !!: !!:  !!! `,
	` :.:: :   :   : :  :: :: :   :   : : `,
}

// logoWidth is what the art takes on the screen.
var logoWidth = ansi.StringWidth(logo[0])

// minHeaderWidth is what the cluster info and the keys need. When room is
// short, the art is hidden before they are cut.
const minHeaderWidth = 60

// fitLogo is the art when there is room for it, nothing when there is not.
// Half a logo is worse than none.
func fitLogo(width int) string {
	if width-logoWidth < minHeaderWidth {
		return ""
	}

	rows := make([]string, 0, len(logo))
	for _, row := range logo {
		rows = append(rows, styleLogo.Render(row))
	}

	return strings.Join(rows, "\n")
}
