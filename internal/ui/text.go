package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// textModel is a block of text with a window over it: what a description or
// a job file is read through.
type textModel struct {
	lines []string

	// filter keeps only the lines that say it.
	filter string

	top int

	width  int
	height int
}

// visible are the lines the filter leaves.
func (t textModel) visible() []string {
	if t.filter == "" {
		return t.lines
	}

	match := matcher(t.filter)

	kept := make([]string, 0, len(t.lines))
	for _, line := range t.lines {
		if match(line) {
			kept = append(kept, line)
		}
	}

	return kept
}

func newTextModel(content string) textModel {
	return textModel{lines: strings.Split(content, "\n")}
}

func (t *textModel) setSize(width, height int) {
	t.width, t.height = width, max(height, 1)
	t.follow()
}

func (t *textModel) move(delta int) {
	t.top = clamp(t.top+delta, 0, max(len(t.visible())-t.height, 0))
}

func (t *textModel) follow() {
	t.top = clamp(t.top, 0, max(len(t.visible())-t.height, 0))
}

func (t textModel) view() string {
	lines := t.visible()

	rows := make([]string, 0, t.height)

	for i := t.top; i < len(lines) && i < t.top+t.height; i++ {
		line := ansi.Truncate(lines[i], t.width, "…")
		rows = append(rows, styleText.Render(pad(line, t.width)))
	}

	return strings.Join(rows, "\n")
}
