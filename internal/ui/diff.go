package ui

import (
	"strings"

	"github.com/ingvarch/urga/internal/nomad"
)

// diffLines lay a job diff out as a unified diff shows a change to a file: a
// column with + or -, then the line indented as deep as it sits in the job.
// Added lines are green, deleted ones red, and the blocks around them are
// muted.
func diffLines(diff []nomad.DiffLine) []paintedLine {
	lines := make([]paintedLine, 0, len(diff))

	for _, line := range diff {
		if line.Text == "" {
			lines = append(lines, paintedLine{})

			continue
		}

		mark, style := " ", &styleMuted

		switch line.Kind {
		case nomad.DiffAdded:
			mark, style = "+", &styleAdded
		case nomad.DiffDeleted:
			mark, style = "-", &styleDeleted
		}

		lines = append(lines, paintedLine{
			text:  mark + " " + strings.Repeat("  ", line.Indent) + line.Text,
			style: style,
		})
	}

	return lines
}

// plainLines are the lines of a text, in the default text colour.
func plainLines(text string) []paintedLine {
	lines := []paintedLine{}
	for _, line := range strings.Split(text, "\n") {
		lines = append(lines, paintedLine{text: line})
	}

	return lines
}
