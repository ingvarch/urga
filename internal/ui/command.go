package ui

import (
	"slices"
	"sort"
	"strings"
)

// commandAliases are the words the prompt understands for a resource. They
// come from the one table that knows what urga opens by name.
var commandAliases = aliasIndex()

func aliasIndex() map[string]*view {
	aliases := map[string]*view{}

	for _, v := range views {
		for _, alias := range v.aliases {
			aliases[alias] = v
		}
	}

	return aliases
}

// commandNames are the words the prompt offers: the first alias of every
// resource, which is the one that reads as the thing it opens. Short forms
// stay out of it, and so does leaving urga: that is a command, not a
// resource to open.
var commandNames = nameIndex()

func nameIndex() []string {
	names := []string{}

	for _, v := range views {
		if len(v.aliases) > 0 {
			names = append(names, v.aliases[0])
		}
	}

	sort.Strings(names)

	return names
}

// bailAliases leave urga.
var bailAliases = map[string]bool{
	"q":    true,
	"q!":   true,
	"qa":   true,
	"quit": true,
	"exit": true,
}

// scope is where the session looks, which a command line word can switch
// instead of opening a resource.
type scope int

const (
	scopeNone scope = iota
	scopeRegion
	scopeDatacenter
	scopeCluster
)

// scopeNames are the words that switch a scope, in the order the prompt
// offers them: after the resources, so that `d` stays the deployments.
var scopeNames = []string{"ctx", "dc", "region"}

var scopeAliases = map[string]scope{
	"ctx":     scopeCluster,
	"cluster": scopeCluster,
	"dc":      scopeDatacenter,
	"region":  scopeRegion,
}

// command is what the prompt was asked to do.
type command struct {
	bail      bool
	view      *view
	namespace string

	// switching is the scope the line switches, name what it switches to.
	switching scope
	name      string
}

// parseCommand reads "<resource> [namespace]", for example "jobs production",
// or "<scope> [name]", for example "region eu". A word can be a prefix, as
// long as it fits a single one of them; a resource wins over a scope.
func parseCommand(input string) (command, bool) {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return command{}, false
	}

	word := strings.ToLower(fields[0])

	var second string
	if len(fields) > 1 {
		second = fields[1]
	}

	if bailAliases[word] {
		return command{bail: true}, true
	}

	if switching, ok := scopeAliases[word]; ok {
		return command{switching: switching, name: second}, true
	}

	if v, ok := resolveAlias(word); ok {
		return command{view: v, namespace: second}, true
	}

	if switching, ok := resolveScope(word); ok {
		return command{switching: switching, name: second}, true
	}

	return command{}, false
}

// resolveScope takes a prefix that fits one scope word.
func resolveScope(word string) (scope, bool) {
	found := []scope{}

	for _, name := range scopeNames {
		if strings.HasPrefix(name, word) {
			found = append(found, scopeAliases[name])
		}
	}

	if len(found) != 1 {
		return scopeNone, false
	}

	return found[0], true
}

// resolveAlias takes a whole alias or a prefix that fits one resource.
func resolveAlias(word string) (*view, bool) {
	if v, ok := commandAliases[word]; ok {
		return v, true
	}

	var found *view

	for alias, v := range commandAliases {
		if !strings.HasPrefix(alias, word) {
			continue
		}

		if found != nil && v != found {
			return nil, false
		}

		found = v
	}

	return found, found != nil
}

// matchingCommands are the resources a line could still be about: all of
// them while nothing is typed, and those the first word begins after that.
// What the line says after the first word is where to look, not what to
// open.
func matchingCommands(typed string) []string {
	word := strings.ToLower(firstWord(typed))

	matches := []string{}

	// A word that is an alias of its own names its resource, whatever else
	// begins with it: `no` is a client, not the pool it sits in.
	if named, ok := commandAliases[word]; ok && nameOf(named) != "" {
		matches = append(matches, nameOf(named))
	}

	for _, name := range append(slices.Clone(commandNames), scopeNames...) {
		if strings.HasPrefix(name, word) && !slices.Contains(matches, name) {
			matches = append(matches, name)
		}
	}

	return matches
}

// nameOf is what a resource is called in the command line.
func nameOf(v *view) string {
	if len(v.aliases) > 0 {
		return v.aliases[0]
	}

	return ""
}
