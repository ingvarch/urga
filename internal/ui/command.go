package ui

import (
	"sort"
	"strings"
)

// commandAliases are the words the prompt understands for a resource. They
// come from the one table that knows what urga can show.
var commandAliases = func() map[string]screenKind {
	aliases := map[string]screenKind{}

	for kind, res := range resources {
		for _, alias := range res.aliases {
			aliases[alias] = kind
		}
	}

	return aliases
}()

// commandNames are the words the prompt suggests: the first alias of every
// resource, which is the one that reads as the thing it opens. Short forms
// stay out of it.
var commandNames = func() []string {
	names := []string{"quit"}

	for _, res := range resources {
		if len(res.aliases) > 0 {
			names = append(names, res.aliases[0])
		}
	}

	sort.Strings(names)

	return names
}()

// bailAliases leave urga.
var bailAliases = map[string]bool{
	"q":    true,
	"q!":   true,
	"qa":   true,
	"quit": true,
	"exit": true,
}

// command is what the prompt was asked to do.
type command struct {
	bail      bool
	kind      screenKind
	namespace string
}

// parseCommand reads "<resource> [namespace]", for example "jobs production".
// The resource can be an alias or a prefix of one, as long as the prefix
// names a single resource.
func parseCommand(input string) (command, bool) {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return command{}, false
	}

	word := strings.ToLower(fields[0])

	if bailAliases[word] {
		return command{bail: true}, true
	}

	kind, ok := resolveAlias(word)
	if !ok {
		return command{}, false
	}

	cmd := command{kind: kind}
	if len(fields) > 1 {
		cmd.namespace = fields[1]
	}

	return cmd, true
}

// resolveAlias takes a whole alias or a prefix that fits one resource.
func resolveAlias(word string) (screenKind, bool) {
	if kind, ok := commandAliases[word]; ok {
		return kind, true
	}

	var found screenKind
	matched := false

	for alias, kind := range commandAliases {
		if !strings.HasPrefix(alias, word) {
			continue
		}

		if matched && kind != found {
			return 0, false
		}

		found, matched = kind, true
	}

	return found, matched
}

// matchingCommands are the resources a line could still be about: all of
// them while nothing is typed, and those the first word begins after that.
// A line with a second word in it is about where to look rather than what to
// open, and has none.
func matchingCommands(typed string) []string {
	if strings.Contains(strings.TrimSpace(typed), " ") {
		return nil
	}

	word := strings.ToLower(firstWord(typed))

	matches := []string{}

	for _, name := range commandNames {
		if strings.HasPrefix(name, word) {
			matches = append(matches, name)
		}
	}

	return matches
}
