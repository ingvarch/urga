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
func cleanLine(line string) string { return cleanWith(ansi.NewParser(), line) }

// cleanWith is cleanLine with a parser of the caller's: a parser holds a
// buffer of 64 KiB, and a screen of lines can share one.
func cleanWith(parser *ansi.Parser, line string) string {
	var (
		out   strings.Builder
		state byte
		col   int
	)

	parser.Reset()

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

// drawable is the screen as the terminal gets it: line breaks, colours and
// styles stay, every other escape sequence and control character is dropped.
// Names, paths, specs and errors come from the cluster and from tasks, and a
// terminal acts on the sequences they can hold.
func drawable(screen string) string {
	parser := ansi.NewParser()

	lines := strings.Split(screen, "\n")
	for i, line := range lines {
		lines[i] = cleanWith(parser, line)
	}

	return strings.Join(lines, "\n")
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
