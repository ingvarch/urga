package ui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		input     string
		kind      screenKind
		namespace string
		ok        bool
	}{
		{input: "jobs", kind: screenJobs, ok: true},
		{input: "job", kind: screenJobs, ok: true},
		{input: "jb", kind: screenJobs, ok: true},
		{input: "deployments", kind: screenDeployments, ok: true},
		{input: "dp", kind: screenDeployments, ok: true},
		{input: "namespaces", kind: screenNamespaces, ok: true},
		{input: "ns", kind: screenNamespaces, ok: true},
		{input: "svc", kind: screenServices, ok: true},
		{input: "ev", kind: screenEvaluations, ok: true},
		{input: "no", kind: screenNodes, ok: true},
		{input: "vars", kind: screenVariables, ok: true},
		{input: "np", kind: screenNodePools, ok: true},
		{input: "alloc", kind: screenAllocations, ok: true},

		// A namespace as the second word opens the resource there.
		{input: "jobs production", kind: screenJobs, namespace: "production", ok: true},
		{input: "  JOBS   Production  ", kind: screenJobs, namespace: "Production", ok: true},

		// A prefix of an alias is enough while it names one resource.
		{input: "jo", kind: screenJobs, ok: true},
		{input: "depl", kind: screenDeployments, ok: true},

		// Nothing, or a word that fits nothing, is no command.
		{input: "", ok: false},
		{input: "zzz", ok: false},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			r := require.New(t)

			cmd, ok := parseCommand(test.input)
			r.Equal(test.ok, ok)

			if test.ok {
				r.Equal(test.kind, cmd.kind)
				r.Equal(test.namespace, cmd.namespace)
			}
		})
	}
}

func TestParseCommand_Quit(t *testing.T) {
	r := require.New(t)

	for _, input := range []string{"q", "q!", "qa", "quit", "exit"} {
		cmd, ok := parseCommand(input)
		r.True(ok, input)
		r.True(cmd.bail, input)
	}
}

func TestMatchingCommands(t *testing.T) {
	r := require.New(t)

	// What the line could still be about, in the order it walks through.
	r.Equal([]string{"jobs"}, matchingCommands("jo"))
	r.Equal([]string{"servers", "services"}, matchingCommands("se"))
	r.Equal([]string{
		"allocations", "clients", "deployments", "evaluations", "jobs",
		"namespaces", "nodepools", "servers", "services", "variables",
	}, matchingCommands(""))

	// A word that is an alias of its own comes first, whatever other names
	// begin with it: `no` is what Nomad calls a client, and it must not be
	// taken for the pool the client is in.
	r.Equal("clients", matchingCommands("no")[0])
	r.Equal("clients", matchingCommands("node")[0])
	r.Contains(matchingCommands("no"), "nodepools")

	// Where to look is not part of the name: the resource is still the
	// first word, whatever follows it.
	r.Equal([]string{"jobs"}, matchingCommands("jo production"))

	// Nothing fits a word that names nothing.
	r.Empty(matchingCommands("zz"))
}
