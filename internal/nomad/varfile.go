package nomad

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/hashicorp/nomad/api"
)

// VariableConflict is a save refused because the variable changed since it
// was read: what happened to it, and the index a save must give to
// replace it.
type VariableConflict struct {
	Path    string
	Deleted bool
	Lock    *VariableLock
	Index   uint64
}

func (e *VariableConflict) Error() string {
	switch {
	case e.Deleted:
		return fmt.Sprintf("variable %s was deleted since it was read", e.Path)
	case e.Lock != nil:
		return fmt.Sprintf("variable %s is locked: only the holder of lock %s can change it", e.Path, e.Lock.ID)
	}

	return fmt.Sprintf("variable %s changed since it was read", e.Path)
}

// VariableSource is a variable as a file, and the index it was read at: the
// save succeeds only if the variable is still at that index.
type VariableSource struct {
	Source string
	Index  uint64
}

// VariableSpec is a variable as a file, which is how it is edited.
func (c *Client) VariableSpec(ctx context.Context, namespace, path string) (VariableSource, error) {
	v, _, err := c.api.Variables().Read(path, c.query(ctx, namespace))
	if err != nil {
		return VariableSource{}, err
	}

	return VariableSource{Source: variableFile(v.Namespace, v.Path, v.Items), Index: v.ModifyIndex}, nil
}

// SubmitVariable sends a variable file back to the cluster with
// check-and-set, so that a change made since it was read is not overwritten.
func (c *Client) SubmitVariable(ctx context.Context, namespace, path, source string, index uint64) error {
	items, err := parseVariableFile(source)
	if err != nil {
		return err
	}

	v := &api.Variable{Namespace: namespace, Path: path, Items: items, ModifyIndex: index}

	_, _, err = c.api.Variables().CheckedUpdate(v, c.write(ctx, namespace))

	if conflict := (api.ErrCASConflict{}); errors.As(err, &conflict) {
		return c.variableConflict(ctx, namespace, path)
	}

	return err
}

// variableConflict reads the variable again to find out what happened to it:
// the cluster returns the same error for a variable changed, deleted or
// locked since, and returns no variable for the last two.
func (c *Client) variableConflict(ctx context.Context, namespace, path string) error {
	now, _, err := c.api.Variables().Peek(path, c.query(ctx, namespace))
	if err != nil {
		return fmt.Errorf("variable %s changed since it was read, and reading it again failed: %w", path, err)
	}

	if now == nil {
		return &VariableConflict{Path: path, Deleted: true}
	}

	return &VariableConflict{Path: path, Lock: lockOf(now.Lock), Index: now.ModifyIndex}
}

// variableFile writes the items of a variable as TOML, one key per line, by
// key. A value of several lines is written as those lines.
func variableFile(namespace, path string, items map[string]string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Variable %s in namespace %s.\n", path, namespace)

	for _, key := range slices.Sorted(maps.Keys(items)) {
		fmt.Fprintf(&b, "%s = %s\n", tomlKey(key), tomlString(items[key]))
	}

	return b.String()
}

// bareKey is a key TOML reads without quotes.
var bareKey = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// tomlKey quotes a key that is not bare: a dot in it would make a table.
func tomlKey(key string) string {
	if bareKey.MatchString(key) {
		return key
	}

	return `"` + escape(key, false) + `"`
}

// tomlString is a value as a TOML string: a multi-line one when it has
// lines, so that a certificate reads as one, and a literal one when it has
// quotes, so that JSON reads as JSON.
func tomlString(value string) string {
	if strings.Contains(value, "\n") {
		// TOML drops the line break that follows the opening quotes.
		return `"""` + "\n" + escape(value, true) + `"""`
	}

	if strings.Contains(value, `"`) && literalFits(value) {
		return "'" + value + "'"
	}

	return `"` + escape(value, false) + `"`
}

// literalFits says TOML can read the value between single quotes, where
// nothing is escaped: it has no single quote and no control character but a
// tab.
func literalFits(value string) bool {
	for _, r := range value {
		if r == '\'' || (r < 0x20 && r != '\t') || r == 0x7f {
			return false
		}
	}

	return true
}

// escape writes a value so that TOML reads it back as it is. A multi-line
// string keeps its line breaks and its quotes, but for a quote that another
// one follows: three in a row would end the string.
func escape(value string, multiline bool) string {
	var b strings.Builder

	runes := []rune(value)

	for i, r := range runes {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == '"' && (!multiline || i+1 < len(runes) && runes[i+1] == '"'):
			b.WriteString(`\"`)
		case r == '\n' && multiline, r == '\t', r >= 0x20 && r != 0x7f:
			b.WriteRune(r)
		default:
			fmt.Fprintf(&b, `\u%04X`, r)
		}
	}

	return b.String()
}

// parseVariableFile reads the items back. A variable holds text only, so a
// number or a table is refused rather than turned into text.
func parseVariableFile(source string) (map[string]string, error) {
	var raw map[string]any
	if _, err := toml.Decode(source, &raw); err != nil {
		return nil, fmt.Errorf("the variable is not valid TOML: %w", err)
	}

	items := make(map[string]string, len(raw))

	for _, key := range slices.Sorted(maps.Keys(raw)) {
		text, ok := raw[key].(string)
		if !ok {
			return nil, fmt.Errorf("%s: a value must be text in quotes", key)
		}

		items[key] = text
	}

	return items, nil
}
