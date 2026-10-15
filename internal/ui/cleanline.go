package ui

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// tabWidth is how far apart tab stops are, in cells: a tab took 4 before.
const tabWidth = 4

// cleanLine is a line a task wrote, as the screen can hold it: tabs are
// spaces up to the next stop, control characters and every escape sequence
// but colours and styles are dropped.
func cleanLine(line string) string {
	var (
		out   strings.Builder
		state byte
		col   int
	)

	parser := ansi.NewParser()

	for len(line) > 0 {
		seq, width, n, next := ansi.DecodeSequence(line, state, parser)
		state, line = next, line[n:]

		switch {
		case seq[0] == ansi.ESC:
			if isStyle(ansi.Cmd(parser.Command()), seq) {
				out.WriteString(seq)
			}
		case seq == "\t":
			pad := tabWidth - col%tabWidth
			out.WriteString(strings.Repeat(" ", pad))
			col += pad
		case isControl(seq):
		default:
			out.WriteString(seq)

			col += width
		}
	}

	return out.String()
}

// isStyle tells a colour or style sequence (SGR) from the others.
func isStyle(cmd ansi.Cmd, seq string) bool {
	return ansi.HasCsiPrefix(seq) && cmd.Final() == 'm' && cmd.Prefix() == 0 && cmd.Intermediate() == 0
}

// isControl tells a C0 or C1 control character, or a byte that is no
// character, from text.
func isControl(seq string) bool {
	r, _ := utf8.DecodeRuneInString(seq)

	return r == utf8.RuneError || unicode.IsControl(r)
}
