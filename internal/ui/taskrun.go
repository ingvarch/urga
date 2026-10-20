package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// askCommand asks which command to run in the task under the cursor.
func askCommand[P taskLister](p P, e env) (P, outcome) {
	task, ok := p.pickedTask(e)
	if !ok {
		return p, outcome{}
	}

	return p, then(lineMsg{
		prefix: fmt.Sprintf("run in %s: ", task.name),
		answer: func(m Model, typed string) (Model, tea.Cmd) { return m.runIn(task.taskRef, typed) },
	})
}

// runIn runs the command typed in the task. It asks no question first: the
// command was typed and entered, as a shell is opened without one.
func (m Model) runIn(task taskRef, typed string) (Model, tea.Cmd) {
	typed = strings.TrimSpace(typed)
	if typed == "" {
		return m.warn("No command to run."), nil
	}

	return m, runCommand(m.client, task, typed)
}

// runCommand runs the line in a shell of the task, so a pipe, quotes and
// variables work as typed, and opens what it printed.
func runCommand(client allocsClient, task taskRef, line string) tea.Cmd {
	return request(func(ctx context.Context) (describeMsg, error) {
		out, err := client.RunCommand(ctx, task.namespace, task.allocID, task.name, []string{"/bin/sh", "-c", line})
		if errors.Is(err, context.DeadlineExceeded) {
			return describeMsg{}, fmt.Errorf("%s did not end within %s", line, defaultTimeout)
		}

		if err != nil {
			return describeMsg{}, err
		}

		// The command goes last: a title too long for the border is cut from
		// its end.
		return describeMsg{
			label: fmt.Sprintf("Output (Task: %s, Allocation: %s) [exit %d%s]",
				task.name, shortID(task.allocID), out.ExitCode, spanOf(out.Size, out.From)),
			after:   ": " + line,
			content: printedText(out.Text),
		}, nil
	}, func(msg describeMsg) tea.Msg { return msg })
}

// printedText is what a command printed, as a log shows it: no empty last
// line, and no control characters.
func printedText(text string) string {
	lines := linesOf(text)

	for i, line := range lines {
		lines[i] = cleanLine(line)
	}

	return strings.Join(lines, "\n")
}
