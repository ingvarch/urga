package ui

import (
	"strings"

	"github.com/ingvarch/urga/internal/nomad"
)

// diffLines lay a job diff out as a unified diff shows a change to a file: a
// column for what happened to the line, then the line as deep in the job as
// it sits. Added lines are green, deleted ones red, and the blocks that lead
// to them step back.
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

// plainLines are lines of text in the colour of text.
func plainLines(text string) []paintedLine {
	lines := []paintedLine{}
	for _, line := range strings.Split(text, "\n") {
		lines = append(lines, paintedLine{text: line})
	}

	return lines
}
