package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// everyScreen is one model per screen, each with rows on it: a key that acts
// on a row says nothing about itself when there is no row. The screens are
// kept by name rather than by kind: the allocations of a job and the screen
// of one client are the same kind and answer different keys.
func everyScreen(t *testing.T) map[string]Model {
	t.Helper()

	client := &fakeClient{
		jobs:        twoJobs(),
		allocs:      twoAllocs(),
		groups:      twoGroups(),
		deployments: []nomad.Deployment{{ID: "dep-1", JobID: "web", Namespace: "production", Status: "running"}},
		namespaces:  twoNamespaces(),
		services:    []nomad.Service{{Name: "api", Namespace: "production"}},
		evaluations: []nomad.Evaluation{{ID: "eval-1", JobID: "web", Namespace: "production", Status: "complete"}},
		nodes:       busyClient(),
		nodeAllocs:  clientAllocs(),
		nodeDetail:  clientDetail(),
		nodeMeta:    clientMeta(),
		variables:   []nomad.Variable{{Path: "nomad/jobs/web", Namespace: "production"}},
		nodePools:   []nomad.NodePool{{Name: "default"}},
		use:         map[string]nomad.ResourceUse{"node-1": {}},
		servers:     twoServers(),
		describe:    "{}",
		spec:        "job \"web\" {}",
	}

	rows := map[string]tea.Msg{
		"jobs":        jobsMsg(client.jobs),
		"deployments": deploymentsMsg(client.deployments),
		"namespaces":  namespacesMsg(client.namespaces),
		"services":    servicesMsg(client.services),
		"evaluations": evaluationsMsg(client.evaluations),
		"nodes":       nodesMsg(client.nodes),
		"variables":   variablesMsg(client.variables),
		"nodepools":   nodePoolsMsg(client.nodePools),
		"servers":     serversMsg(client.servers),
	}

	open := map[string]Model{}

	for name, filled := range rows {
		m := newTestModel(client)
		m, _ = m.update(key(':'))
		m = typeIn(m, name)
		m, _ = m.update(enter())
		m, _ = m.update(filled)

		open[name] = m
	}

	jobs := open["jobs"]

	allocs, _ := jobs.update(enter())
	allocs, _ = allocs.update(allocsMsg(twoAllocs()))
	open["allocations"] = allocs

	tasks, _ := allocs.update(enter())
	open["tasks"] = tasks

	groups, _ := jobs.update(key('t'))
	groups, _ = groups.update(taskGroupsMsg(twoGroups()))
	open["taskgroups"] = groups

	described, cmd := jobs.update(key('d'))
	open["describe"] = drain(described, cmd)

	server, _ := open["servers"].update(enter())
	open["server"] = server

	// The screens of one client, each opened by the key that offers it.
	machine, machineCmd := open["nodes"].update(enter())
	machine = drain(machine, machineCmd)
	open["client"] = machine

	for name, press := range map[string]tea.KeyPressMsg{
		"events":     key('e'),
		"drivers":    ctrlKey('d'),
		"volumes":    ctrlKey('h'),
		"attributes": key('a'),
		"meta":       key('m'),
	} {
		next, cmd := machine.update(press)
		open[name] = drain(next, cmd)
	}

	driver, cmd := open["drivers"].update(enter())
	open["driver"] = drain(driver, cmd)

	return open
}

func TestHints_EveryKeyTheHeaderOffersDoesSomething(t *testing.T) {
	r := require.New(t)

	for name, m := range everyScreen(t) {
		// Whatever the screen was left holding says nothing about the key
		// that is about to be pressed.
		m.err = nil

		for _, h := range m.screen.hints() {
			// A key in the header is a promise: pressing it opens something,
			// asks the cluster something or puts a question up. A key that
			// is drawn and does nothing is worse than no key.
			next, cmd := m.handleKey(keyOf(h.Key))

			did := cmd != nil ||
				next.screen != m.screen ||
				next.overlay != m.overlay ||
				next.err != nil ||
				next.said != m.said

			r.True(did, "the %s screen offers %s and nothing happens", name, h.Key)
		}
	}
}

// keyOf reads a key the way the header writes it: <enter>, <ctrl-s>, <d>. A
// key it cannot spell would be pressed as its first letter, which is why
// every control key has a case of its own here.
func keyOf(shown string) tea.KeyPressMsg {
	name := shown[1 : len(shown)-1]

	switch name {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "ctrl-s":
		return ctrlKey('s')
	case "ctrl-k":
		return ctrlKey('k')
	case "ctrl-d":
		return ctrlKey('d')
	case "ctrl-e":
		return ctrlKey('e')
	case "ctrl-h":
		return ctrlKey('h')
	}

	return key(rune(name[0]))
}
