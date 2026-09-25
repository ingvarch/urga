package nomad

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/nomad/api"
)

// DiffKind says what a line of a diff is: added, deleted, or there to show
// where in the job the change is.
type DiffKind int

// What a line of a diff is.
const (
	DiffContext DiffKind = iota
	DiffAdded
	DiffDeleted
)

// DiffLine is one line of a job diff laid out as HCL: how deep in the job it
// sits and what it says.
type DiffLine struct {
	Kind   DiffKind
	Indent int
	Text   string
}

// The types the cluster gives a part of a diff.
const (
	diffAdded   = "Added"
	diffDeleted = "Deleted"
	diffEdited  = "Edited"
	diffNone    = "None"
)

// changed says whether a part of a diff is a change. The cluster diffs in
// context, so what did not change comes along as None.
func changed(kind string) bool {
	return kind != diffNone
}

// hclDiff lays a job diff out the way the job file reads, as a unified diff
// shows a change to it: the blocks that lead to a change, and every changed
// value as the line it was and the line it is. What did not change is left
// out.
func hclDiff(diff *api.JobDiff) []DiffLine {
	if diff == nil || !changed(diff.Type) {
		return nil
	}

	lines := fieldLines(diff.Type, diff.Fields, 0)

	parts := objectBlocks(diff.Objects, 0)

	for _, group := range diff.TaskGroups {
		if group == nil || !changed(group.Type) {
			continue
		}

		tasks := [][]DiffLine{}

		for _, task := range group.Tasks {
			if task == nil || !changed(task.Type) {
				continue
			}

			tasks = append(tasks, block(fmt.Sprintf("task %q", task.Name), task.Type, task.Fields, task.Objects, nil, 1))
		}

		parts = append(parts, block(fmt.Sprintf("group %q", group.Name), group.Type, group.Fields, group.Objects, tasks, 0))
	}

	// The job has no block of its own in a diff: its parts stand one under
	// the other, a line apart.
	for _, part := range parts {
		if len(lines) > 0 {
			lines = append(lines, DiffLine{})
		}

		lines = append(lines, part...)
	}

	return lines
}

// block is a block of the job: its opening line, its values, the blocks
// inside it, and its closing brace. An edited block marks only what changed
// in it; an added or a deleted one is marked whole.
func block(open, kind string, fields []*api.FieldDiff, objects []*api.ObjectDiff, children [][]DiffLine, indent int) []DiffLine {
	edge := blockKind(kind)

	lines := []DiffLine{{Kind: edge, Indent: indent, Text: open + " {"}}
	lines = append(lines, fieldLines(kind, fields, indent+1)...)

	for _, inner := range append(objectBlocks(objects, indent+1), children...) {
		// A line apart from what came before it inside the block.
		if len(lines) > 1 {
			lines = append(lines, DiffLine{})
		}

		lines = append(lines, inner...)
	}

	return append(lines, DiffLine{Kind: edge, Indent: indent, Text: "}"})
}

// objectBlocks are the blocks of the objects that changed.
func objectBlocks(objects []*api.ObjectDiff, indent int) [][]DiffLine {
	blocks := [][]DiffLine{}

	for _, object := range objects {
		if object == nil || !changed(object.Type) {
			continue
		}

		blocks = append(blocks, block(hclName(object.Name), object.Type, object.Fields, object.Objects, nil, indent))
	}

	return blocks
}

// fieldLines are the values of a block that changed: plain values first, then
// the ones a job file writes inside a block of their own, like env and meta,
// which the cluster names Env[KEY].
func fieldLines(kind string, fields []*api.FieldDiff, indent int) []DiffLine {
	lines := []DiffLine{}

	keyed := map[string][]*api.FieldDiff{}
	order := []string{}

	for _, field := range fields {
		if field == nil || !changed(field.Type) {
			continue
		}

		name, key, ok := keyedField(field.Name)
		if !ok {
			lines = append(lines, valueLines(field, hclName(field.Name), hclValue, indent)...)

			continue
		}

		if _, seen := keyed[name]; !seen {
			order = append(order, name)
		}

		keyed[name] = append(keyed[name], &api.FieldDiff{Type: field.Type, Name: key, Old: field.Old, New: field.New})
	}

	for _, name := range order {
		edge := blockKind(kind)

		lines = append(lines, DiffLine{Kind: edge, Indent: indent, Text: name + " {"})

		// A key is written as it is: env and meta keys are the names a task
		// reads, and they tell capitals apart. Their values are strings,
		// whatever they hold.
		for _, field := range keyed[name] {
			lines = append(lines, valueLines(field, field.Name, strconv.Quote, indent+1)...)
		}

		lines = append(lines, DiffLine{Kind: edge, Indent: indent, Text: "}"})
	}

	return lines
}

// valueLines are one value as the line it was and the line it is.
func valueLines(field *api.FieldDiff, name string, write func(string) string, indent int) []DiffLine {
	line := func(kind DiffKind, value string) DiffLine {
		return DiffLine{Kind: kind, Indent: indent, Text: name + " = " + write(value)}
	}

	lines := []DiffLine{}

	switch field.Type {
	case diffAdded:
		if field.New != "" {
			lines = append(lines, line(DiffAdded, field.New))
		}

	case diffDeleted:
		if field.Old != "" {
			lines = append(lines, line(DiffDeleted, field.Old))
		}

	default:
		if field.Old != "" {
			lines = append(lines, line(DiffDeleted, field.Old))
		}

		if field.New != "" {
			lines = append(lines, line(DiffAdded, field.New))
		}
	}

	return lines
}

// blockKind is how the edges of a block are marked: an edited block leads to
// its changes and is not a change itself.
func blockKind(kind string) DiffKind {
	switch kind {
	case diffAdded:
		return DiffAdded
	case diffDeleted:
		return DiffDeleted
	}

	return DiffContext
}

var keyedName = regexp.MustCompile(`^(\w+)\[(.+)\]$`)

// keyedField reads Env[KEY] as the block env and the key KEY.
func keyedField(name string) (block, key string, ok bool) {
	match := keyedName.FindStringSubmatch(name)
	if match == nil {
		return "", "", false
	}

	// args[1] is an element of a list, not a key.
	if _, err := strconv.Atoi(match[2]); err == nil {
		return "", "", false
	}

	return strings.ToLower(match[1]), match[2], true
}

var (
	acronymBefore = regexp.MustCompile(`([A-Z]+)([A-Z][a-z])`)
	wordBefore    = regexp.MustCompile(`([a-z\d])([A-Z])`)
)

// renamed are the fields a job file names other than by their words.
var renamed = map[string]string{
	"MemoryMB":    "memory",
	"MemoryMaxMB": "memory_max",
}

// hclName is a field as a job file names it: PortLabel is port_label, and an
// acronym is one word, HTTPPort http_port.
func hclName(name string) string {
	if hcl, ok := renamed[name]; ok {
		return hcl
	}

	name = acronymBefore.ReplaceAllString(name, "${1}_${2}")
	name = wordBefore.ReplaceAllString(name, "${1}_${2}")

	return strings.ToLower(name)
}

var literal = regexp.MustCompile(`^(\d+|\d+\.\d+|true|false)$`)

// hclValue is a value as a job file writes it: a number or a boolean as it
// is, anything else as a string.
func hclValue(value string) string {
	if literal.MatchString(value) {
		return value
	}

	return strconv.Quote(value)
}
