package ui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		input     string
		view      *view
		namespace string
		ok        bool
	}{
		{input: "jobs", view: jobsView, ok: true},
		{input: "job", view: jobsView, ok: true},
		{input: "jb", view: jobsView, ok: true},
		{input: "deployments", view: deploymentsView, ok: true},
		{input: "dp", view: deploymentsView, ok: true},
		{input: "namespaces", view: namespacesView, ok: true},
		{input: "ns", view: namespacesView, ok: true},
		{input: "svc", view: servicesView, ok: true},
		{input: "ev", view: evaluationsView, ok: true},
		{input: "no", view: nodesView, ok: true},
		{input: "vars", view: variablesView, ok: true},
		{input: "np", view: nodePoolsView, ok: true},
		{input: "alloc", view: allocationsView, ok: true},

		// A namespace as the second word opens the resource there.
		{input: "jobs production", view: jobsView, namespace: "production", ok: true},
		{input: "  JOBS   Production  ", view: jobsView, namespace: "Production", ok: true},

		// A prefix of an alias is enough while it names one resource.
		{input: "jo", view: jobsView, ok: true},
		{input: "depl", view: deploymentsView, ok: true},

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
				r.Same(test.view, cmd.view)
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
		"ctx", "dc", "region",
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

func TestParseCommand_SwitchesTheRegionOrTheDatacenter(t *testing.T) {
	tests := []struct {
		input     string
		switching scope
		name      string
	}{
		{input: "region eu", switching: scopeRegion, name: "eu"},
		{input: "region", switching: scopeRegion},
		{input: "reg eu", switching: scopeRegion, name: "eu"},
		{input: "dc dc2", switching: scopeDatacenter, name: "dc2"},
		{input: "dc all", switching: scopeDatacenter, name: "all"},
		{input: "dc", switching: scopeDatacenter},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			r := require.New(t)

			cmd, ok := parseCommand(test.input)
			r.True(ok)
			r.Equal(test.switching, cmd.switching)
			r.Equal(test.name, cmd.name)
		})
	}
}

func TestParseCommand_AResourceComesBeforeASwitch(t *testing.T) {
	r := require.New(t)

	// `d` has opened the deployments since the first day, a datacenter
	// must not take it over.
	cmd, ok := parseCommand("d")
	r.True(ok)
	r.Same(deploymentsView, cmd.view)
	r.Equal(scopeNone, cmd.switching)
}

func TestMatchingCommands_OfferTheSwitchesAfterTheResources(t *testing.T) {
	r := require.New(t)

	r.Equal([]string{"deployments", "dc"}, matchingCommands("d"))
	r.Equal([]string{"dc"}, matchingCommands("dc"))
	r.Equal([]string{"region"}, matchingCommands("r"))
	r.Equal([]string{"region"}, matchingCommands("region eu"))
}
