package ui

import "strings"

// commandAliases are the words the prompt understands for a resource.
var commandAliases = map[string]screenKind{
	"jobs":        screenJobs,
	"job":         screenJobs,
	"jb":          screenJobs,
	"allocations": screenAllocations,
	"allocation":  screenAllocations,
	"allocs":      screenAllocations,
	"alloc":       screenAllocations,
	"deployments": screenDeployments,
	"deployment":  screenDeployments,
	"dp":          screenDeployments,
	"namespaces":  screenNamespaces,
	"namespace":   screenNamespaces,
	"ns":          screenNamespaces,
	"services":    screenServices,
	"service":     screenServices,
	"svc":         screenServices,
	"evaluations": screenEvaluations,
	"evaluation":  screenEvaluations,
	"evals":       screenEvaluations,
	"eval":        screenEvaluations,
	"ev":          screenEvaluations,
	"nodes":       screenNodes,
	"node":        screenNodes,
	"no":          screenNodes,
	"variables":   screenVariables,
	"variable":    screenVariables,
	"vars":        screenVariables,
	"var":         screenVariables,
	"nodepools":   screenNodePools,
	"nodepool":    screenNodePools,
	"np":          screenNodePools,
}

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

	if word == "" {
		return 0, false
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

// completeCommand is the rest of the alias the prompt shows dim behind what
// was typed. It suggests nothing while the word still fits several.
func completeCommand(typed string) string {
	if typed == "" || strings.Contains(typed, " ") {
		return ""
	}

	word := strings.ToLower(typed)

	candidates := []string{}
	for _, name := range commandNames {
		if strings.HasPrefix(name, word) && name != word {
			candidates = append(candidates, name)
		}
	}

	if len(candidates) != 1 {
		return ""
	}

	return candidates[0][len(word):]
}

// commandNames are the words the prompt suggests. Short aliases stay out of
// it, a suggestion is only useful when it reads as the thing it opens.
var commandNames = []string{
	"allocations",
	"deployments",
	"evaluations",
	"jobs",
	"namespaces",
	"nodepools",
	"nodes",
	"quit",
	"services",
	"variables",
}
