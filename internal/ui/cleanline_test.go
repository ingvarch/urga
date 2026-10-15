package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestCleanLine(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"trailing carriage return", "done\r", "done"},
		{"carriage return inside", "a\rb", "ab"},
		{"leading tab", "\tat Foo.bar()", "    at Foo.bar()"},
		{"tab to the next stop", "ab\tc", "ab  c"},
		{"tab on a stop", "abcd\te", "abcd    e"},
		{"colour kept", "\x1b[31mred\x1b[0m", "\x1b[31mred\x1b[0m"},
		{"tab after a colour code", "\x1b[31mab\x1b[0m\tc", "\x1b[31mab\x1b[0m  c"},
		{"tab after a wide character", "世\tx", "世  x"},
		{"osc title", "\x1b]0;title\x07text", "text"},
		{"osc closed by st", "\x1b]0;title\x1b\\text", "text"},
		// An ESC inside a string sequence ends it, as in a terminal: the
		// passthrough is dropped and what follows reads on its own.
		{"dcs", "\x1bPtmux;\x1b\x1b[1m\x1b\\after", "\x1b[1mafter"},
		{"apc", "\x1b_data\x1b\\after", "after"},
		{"pm", "\x1b^data\x1b\\after", "after"},
		{"sos", "\x1bXdata\x1b\\after", "after"},
		{"cursor move", "\x1b[2Jclear", "clear"},
		{"private sequence that ends in m", "\x1b[>4;1mkeys", "keys"},
		{"cursor up", "a\x1b[1Ab", "ab"},
		{"lone escape", "a\x1b", "a"},
		{"bell", "a\x07b", "ab"},
		{"c1 control", "a\u0085b", "ab"},
		{"plain", "just text", "just text"},
		{"wide text", "日本語 text", "日本語 text"},
		{"empty", "", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, cleanLine(c.in))
		})
	}
}

func TestText_AddCleansWhatTheTaskWrote(t *testing.T) {
	r := require.New(t)

	text := textModel{wrap: true}
	text.setSize(12, 10)
	text.add([]string{"\tat Foo.bar(Foo.java:10)\r"}, arrivedAt, nil, nil)

	r.Equal([]string{"    at Foo.bar(Foo.java:10)"}, text.lines)

	rows := drawnInFull(text)
	r.Greater(len(rows), 1)

	var joined strings.Builder

	for _, row := range rows {
		r.LessOrEqual(ansi.StringWidth(row.text), 12)
		joined.WriteString(row.text)
	}

	r.Equal("    at Foo.bar(Foo.java:10)", joined.String())
}

func TestTaskEvents_AMultiLineMessageIsOneRow(t *testing.T) {
	r := require.New(t)

	allocs := allocsWithEvents()
	allocs[0].Tasks[0].Events = []nomad.TaskEvent{
		{Type: "Failed", Message: "1 error occurred:\n\t* failed to download artifact", Failed: true},
		{Type: "Started", Message: "Task started by client"},
	}

	m := onTasks(t, allocs)
	m, _ = m.update(key('e'))

	rows := lines(m.render())
	at := -1

	for i, row := range rows {
		if strings.Contains(row, "failed to download artifact") {
			at = i
		}
	}

	r.NotEqual(-1, at)
	r.Contains(rows[at], "1 error occurred:")
	r.Contains(rows[at], "Failed")
	r.Contains(rows[at+1], "Started")
}
