package ui

import (
	"context"
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

var variableBindings = []binding{
	{press: "v", label: "Toggle Values", do: toggleValues},
	{press: "c", label: "Copy", do: copyValue},
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

// variablePanel says who holds the variable as a lock, when someone does.
func (m Model) variablePanel(width int) []string {
	lock := m.variable.detail.Lock
	if lock == nil {
		return nil
	}

	head := []string{" " + fieldLine([]field{{"Lock", lock.ID}, {"TTL", lock.TTL}, {"Delay", lock.Delay}}, width-1)}

	return fitPanel(head, panelBlock{}, m.rowsForPanel())
}
