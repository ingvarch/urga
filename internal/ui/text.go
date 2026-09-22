package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// textModel is a block of text with a window over it: what a description or
// a job file is read through.
type textModel struct {
	lines []string

	top int

	width  int
	height int
}

func newTextModel(content string) textModel {
	return textModel{lines: strings.Split(content, "\n")}
}

func (t *textModel) setSize(width, height int) {
	t.width, t.height = width, max(height, 1)
	t.follow()
}

func (t *textModel) move(delta int) {
	t.top = clamp(t.top+delta, 0, max(len(t.lines)-t.height, 0))
}

func (t *textModel) follow() {
	t.top = clamp(t.top, 0, max(len(t.lines)-t.height, 0))
}

func (t textModel) view() string {
	rows := make([]string, 0, t.height)

	for i := t.top; i < len(t.lines) && i < t.top+t.height; i++ {
		line := ansi.Truncate(t.lines[i], t.width, "…")
		rows = append(rows, styleText.Render(pad(line, t.width)))
	}

	return strings.Join(rows, "\n")
}
