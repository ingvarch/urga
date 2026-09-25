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

// variablesPage is the variables of the namespace the session looks at.
type variablesPage struct {
	ofTheSession

	variables []nomad.Variable
}

var variableTitles = []string{"Path", "Namespace", "Lock", "Age", "Modified"}

func (variablesPage) title(e env, count int) string {
	return sprintf("Variables (%s) [%d]", namespaceLabel(e.namespace), count)
}

func (variablesPage) titles() []string { return variableTitles }
func (variablesPage) topics() []string { return nil }

func (variablesPage) fetch(e env) tea.Cmd {
	client, namespace := e.client, e.namespace

	return fetchList(func(ctx context.Context) ([]nomad.Variable, error) {
		return client.Variables(ctx, namespace)
	}, func(items []nomad.Variable) tea.Msg { return variablesMsg(items) })
}

func (p variablesPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	variables, ok := msg.(variablesMsg)
	if !ok {
		return p, outcome{}, false
	}

	p.variables = variables

	return p, outcome{}, true
}

func (p variablesPage) rows(env) []tableRow { return variableRows(p.variables) }

func variableRows(variables []nomad.Variable) []tableRow {
	rows := make([]tableRow, 0, len(variables))

	for _, v := range variables {
		rows = append(rows, tableRow{
			cells: []string{v.Path, v.Namespace, lockOf(v), ageOf(v.Created), ageOf(v.Modified)},
			ages:  moments{3: v.Created, 4: v.Modified},
		})
	}

	return rows
}

// lockOf names who holds a variable as a lock, by the ID of the lock: Nomad
// knows the holder by nothing else.
func lockOf(v nomad.Variable) string {
	if v.Lock == nil {
		return ""
	}

	return shortID(v.Lock.ID)
}

// picked is the variable under the cursor.
func (p variablesPage) picked(e env) (nomad.Variable, bool) { return pickedFrom(e, p.variables) }

var variablesKeys = []pageKey[variablesPage]{
	{press: "enter", label: "Values", do: openVariable},
	editVariableKey(variablesPage.picked),
}

func (p variablesPage) keys(e env) []keyHint { return hintsOf(p, e, variablesKeys) }

func (p variablesPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, variablesKeys, k)
}

// openVariable opens the variable under the cursor with its values hidden:
// a screen someone else can see is no place for a password until asked.
func openVariable(p variablesPage, e env) (variablesPage, outcome) {
	v, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg(screen{kind: screenVariable, page: variablePage{namespace: v.Namespace, path: v.Path}}))
}

// variablePage is the variable the page is open on, and whether its values
// are shown.
type variablePage struct {
	namespace, path string

	detail nomad.VariableDetail
	shown  bool
}

func (p variablePage) title(_ env, count int) string {
	return sprintf("Variable %s (%s) [%d]", p.path, namespaceLabel(p.namespace), count)
}

func (variablePage) titles() []string { return variableItemTitles }
func (variablePage) topics() []string { return nil }

// fetch reads the variable the page is open on.
func (p variablePage) fetch(e env) tea.Cmd {
	client, namespace, path := e.client, p.namespace, p.path

	return request(func(ctx context.Context) (nomad.VariableDetail, error) {
		return client.Variable(ctx, namespace, path)
	}, func(v nomad.VariableDetail) tea.Msg { return variableMsg(v) })
}

// take keeps a variable that is the one the page is open on.
func (p variablePage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	v, ok := msg.(variableMsg)
	if !ok || v.Path != p.path || v.Namespace != p.namespace {
		return p, outcome{}, false
	}

	p.detail = nomad.VariableDetail(v)

	return p, outcome{}, true
}

func (p variablePage) rows(env) []tableRow { return variableItemRows(p.detail.Items, p.shown) }

// variable is the one the page is open on, with the lock it last said it
// has.
func (p variablePage) variable(env) (nomad.Variable, bool) {
	v := p.detail.Variable
	v.Namespace, v.Path = p.namespace, p.path

	return v, true
}

var variableKeys = []pageKey[variablePage]{
	{press: "v", label: "Toggle Values", do: toggleValues},
	{press: "c", label: "Copy", do: copyValue},
	editVariableKey(variablePage.variable),
}

func (p variablePage) keys(e env) []keyHint { return hintsOf(p, e, variableKeys) }

func (p variablePage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, variableKeys, k)
}

func toggleValues(p variablePage, _ env) (variablePage, outcome) {
	p.shown = !p.shown

	return p, outcome{}
}

// copyValue copies the value under the cursor as the variable holds it: the
// row may show it hidden or cut to one line.
func copyValue(p variablePage, e env) (variablePage, outcome) {
	row, ok := pickedFrom(e, p.rows(e))
	if !ok || len(row.cells) == 0 {
		return p, outcome{}
	}

	key := row.cells[0]

	value, ok := p.detail.Items[key]
	if !ok {
		return p, outcome{}
	}

	return p, copying(key, value)
}

// variableItemRows are the values of the variable, one line each.
func variableItemRows(items map[string]string, shown bool) []tableRow {
	cells := make(map[string]string, len(items))
	for key, value := range items {
		cells[key] = valueCell(value, shown)
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

// panel says who holds the variable as a lock, when someone does, and why
// it has no Edit then.
func (p variablePage) panel(_ env, width, room int) []string {
	lock := p.detail.Lock
	if lock == nil {
		return nil
	}

	head := []string{
		" " + fieldLine([]field{{"Lock", lock.ID}, {"TTL", lock.TTL}, {"Delay", lock.Delay}}, width-1),
		" " + styleMuted.Render(truncate("Only the holder of the lock can change it.", width-1)),
	}

	return fitPanel(head, panelBlock{}, room)
}

// editVariableKey edits the variable a key acts on, which acted names: the
// one under the cursor of the list, or the one the page is open on.
func editVariableKey[P any](acted func(p P, e env) (nomad.Variable, bool)) pageKey[P] {
	return pageKey[P]{
		press: "e", label: "Edit", writes: true,
		do: func(p P, e env) (P, outcome) {
			v, _ := acted(p, e)

			return p, outcome{cmd: openEditor(variableFile(e.client, v.Namespace, v.Path))}
		},

		// The variable can be changed from here: Nomad refuses a change of
		// a locked one to anyone but the holder.
		offered: func(p P, e env) bool {
			v, ok := acted(p, e)

			return ok && v.Lock == nil
		},
	}
}

// variableFile is a variable as a file, saved over the version it was read
// at.
func variableFile(client variablesClient, namespace, path string) load {
	return func(ctx context.Context) (file, error) {
		spec, err := client.VariableSpec(ctx, namespace, path)

		return file{extension: "toml", content: spec.Source, submit: saveVariable(client, namespace, path, spec.Index)}, err
	}
}

// saveVariable sends an edit of a variable read at index. What the cluster
// refuses goes back to the editor instead of being lost. A variable changed
// or deleted since it was read is saved over the next time, and the file
// says so; a locked one is asked at the old version again.
func saveVariable(client variablesClient, namespace, path string, index uint64) func(source string) tea.Cmd {
	return func(source string) tea.Cmd {
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
			defer cancel()

			err := client.SubmitVariable(ctx, namespace, path, source, index)
			if err == nil {
				return doneMsg{said: fmt.Sprintf("Variable %s saved.", path)}
			}

			next, advice := index, ""

			var conflict *nomad.VariableConflict
			if errors.As(err, &conflict) && conflict.Lock == nil {
				next, advice = conflict.Index, "Saving again replaces that change."

				if conflict.Deleted {
					advice = "Saving again creates it."
				}
			}

			again := file{extension: "toml", content: source, submit: saveVariable(client, namespace, path, next)}

			return refusedEditMsg{err: err, advice: advice, again: again}
		}
	}
}
