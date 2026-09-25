package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// variableItemTitles are the columns of the values of a variable.
var variableItemTitles = []string{"Key", "Value"}

// hiddenValue stands for a value that is not shown. It is the same for
// every value, so it gives away no length.
const hiddenValue = "••••••••"

// variableMsg is a variable read with its values.
type variableMsg nomad.VariableDetail

// variableState is the variable the screen is open on, and whether its
// values are shown.
type variableState struct {
	detail nomad.VariableDetail
	shown  bool
}

// variableRefusedMsg is an edit of a variable the cluster refused, and what
// the edit was made from.
type variableRefusedMsg struct {
	namespace, path string
	index           uint64
	source          string
	err             error
}

var variableBindings = []binding{
	{press: "v", label: "Toggle Values", do: toggleValues},
	{press: "c", label: "Copy", do: copyValue},
	{press: "e", label: "Edit", do: editVariable, writes: true, offered: variableUnlocked},
}

// openVariable opens the variable under the cursor with its values hidden:
// a screen someone else can see is no place for a password until asked.
func openVariable(m Model) (Model, tea.Cmd) {
	variable, ok := selectedOf(m, screenVariables, m.variables)
	if !ok {
		return m, nil
	}

	m.variable = variableState{}

	return m.push(screen{kind: screenVariable, namespace: variable.Namespace, label: variable.Path})
}

// fetchVariable reads the variable the screen is open on.
func fetchVariable(m Model) tea.Cmd {
	client, s := m.client, m.screen

	return request(func(ctx context.Context) (nomad.VariableDetail, error) {
		return client.Variable(ctx, s.namespace, s.label)
	}, func(v nomad.VariableDetail) tea.Msg { return variableMsg(v) })
}

// keepVariable stores a variable that is the one the screen is open on.
func (m Model) keepVariable(msg variableMsg) (Model, tea.Cmd) {
	ours := m.screen.kind == screenVariable && msg.Path == m.screen.label && msg.Namespace == m.screen.namespace

	return m.applyWhen(ours, func(m *Model) { m.variable.detail = nomad.VariableDetail(msg) })
}

func toggleValues(m Model) (Model, tea.Cmd) {
	m.variable.shown = !m.variable.shown
	m.layout()

	return m, nil
}

// copyValue copies the value under the cursor as the variable holds it: the
// row may show it hidden or cut to one line.
func copyValue(m Model) (Model, tea.Cmd) {
	row, ok := m.table.selected()
	if !ok || len(row.cells) == 0 {
		return m, nil
	}

	key := row.cells[0]

	value, ok := m.variable.detail.Items[key]
	if !ok {
		return m, nil
	}

	return copied(m, key, value)
}

// variableItemRows are the values of the variable, one line each.
func variableItemRows(v variableState) []tableRow {
	cells := make(map[string]string, len(v.detail.Items))
	for key, value := range v.detail.Items {
		cells[key] = valueCell(value, v.shown)
	}

	return fieldRows(cells)
}

// valueCell is a value as one line of the table: its first line, and how
// many it has when there are more.
func valueCell(value string, shown bool) string {
	first, _, _ := strings.Cut(value, "\n")
	if !shown {
		first = hiddenValue
	}

	if count := strings.Count(strings.TrimSuffix(value, "\n"), "\n") + 1; count > 1 {
		return fmt.Sprintf("%s (%d lines)", first, count)
	}

	return first
}

// variablePanel says who holds the variable as a lock, when someone does,
// and why it has no Edit then.
func (m Model) variablePanel(width int) []string {
	lock := m.variable.detail.Lock
	if lock == nil {
		return nil
	}

	head := []string{
		" " + fieldLine([]field{{"Lock", lock.ID}, {"TTL", lock.TTL}, {"Delay", lock.Delay}}, width-1),
		" " + styleMuted.Render(truncate("Only the holder of the lock can change it.", width-1)),
	}

	return fitPanel(head, panelBlock{}, m.rowsForPanel())
}

// actedVariable is the variable a key acts on: the one under the cursor of
// the list, or the one the screen is open on.
func (m Model) actedVariable() (nomad.Variable, bool) {
	if m.screen.kind == screenVariables {
		return selectedOf(m, screenVariables, m.variables)
	}

	v := m.variable.detail.Variable
	v.Namespace, v.Path = m.screen.namespace, m.screen.label

	return v, true
}

// variableUnlocked says the variable can be changed from here: Nomad
// refuses a change of a locked one to anyone but the holder.
func variableUnlocked(m Model) bool {
	v, ok := m.actedVariable()

	return ok && v.Lock == nil
}

func editVariable(m Model) (Model, tea.Cmd) {
	v, ok := m.actedVariable()
	if !ok {
		return m, nil
	}

	return m, openEditor(variableFile(m.client, v.Namespace, v.Path))
}

// variableFile is a variable as a file, saved over the version it was read
// at.
func variableFile(client Client, namespace, path string) load {
	return func(ctx context.Context) (file, error) {
		spec, err := client.VariableSpec(ctx, namespace, path)

		return file{extension: "toml", content: spec.Source, submit: saveVariable(client, namespace, path, spec.Index)}, err
	}
}

// saveVariable sends an edit of a variable read at index. What the cluster
// refuses goes back to the editor instead of being lost.
func saveVariable(client Client, namespace, path string, index uint64) func(source string) tea.Cmd {
	return func(source string) tea.Cmd {
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
			defer cancel()

			if err := client.SubmitVariable(ctx, namespace, path, source, index); err != nil {
				return variableRefusedMsg{namespace: namespace, path: path, index: index, source: source, err: err}
			}

			return doneMsg{said: fmt.Sprintf("Variable %s saved.", path)}
		}
	}
}

// reopenVariable opens a refused edit again, with why at the top. A variable
// changed or deleted since it was read is saved over next time, and the file
// says so; a locked one is asked at the old version again.
func (m Model) reopenVariable(msg variableRefusedMsg) tea.Cmd {
	reason, ok := m.refusedBecause(msg.err)
	if !ok {
		reason = msg.err.Error()
	}

	index, advice := msg.index, ""

	var conflict *nomad.VariableConflict
	if errors.As(msg.err, &conflict) && conflict.Lock == nil {
		index, advice = conflict.Index, "Saving again replaces that change."

		if conflict.Deleted {
			advice = "Saving again creates it."
		}
	}

	content := refusal(reason, advice) + withoutLeadingComments(msg.source)
	client := m.client

	return openEditor(func(context.Context) (file, error) {
		return file{extension: "toml", content: content, submit: saveVariable(client, msg.namespace, msg.path, index)}, nil
	})
}

// refusal is why a save was refused, as comment lines. A file that comes
// back as it went out changes nothing, so saving it again takes a change.
func refusal(reason, advice string) string {
	lines := strings.Split("Not saved: "+reason+".", "\n")
	if advice != "" {
		lines = append(lines, advice)
	}

	lines = append(lines, "To save, change the file: delete these lines at least.", "To drop your edit, quit without saving.")

	var b strings.Builder
	for _, line := range lines {
		b.WriteString("# " + line + "\n")
	}

	return b.String()
}

// withoutLeadingComments drops the comments at the top of a file: the
// refusal of the save before, or the name of the variable, which the
// refusal takes the place of.
func withoutLeadingComments(source string) string {
	for strings.HasPrefix(source, "#") {
		_, source, _ = strings.Cut(source, "\n")
	}

	return source
}
