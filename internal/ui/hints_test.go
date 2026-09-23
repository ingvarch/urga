package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// everyScreen is one model per screen, each with rows on it: a key that acts
// on a row says nothing about itself when there is no row.
func everyScreen(t *testing.T) map[screenKind]Model {
	t.Helper()

	client := &fakeClient{
		jobs:        twoJobs(),
		allocs:      twoAllocs(),
		groups:      twoGroups(),
		deployments: []nomad.Deployment{{ID: "dep-1", JobID: "web", Namespace: "production", Status: "running"}},
		namespaces:  twoNamespaces(),
		services:    []nomad.Service{{Name: "api", Namespace: "production"}},
		evaluations: []nomad.Evaluation{{ID: "eval-1", JobID: "web", Namespace: "production", Status: "complete"}},
		nodes:       readyNode(),
		variables:   []nomad.Variable{{Path: "nomad/jobs/web", Namespace: "production"}},
		nodePools:   []nomad.NodePool{{Name: "default"}},
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

	open := map[screenKind]Model{}

	for name, filled := range rows {
		m := newTestModel(client)
		m, _ = m.update(key(':'))
		m = typeIn(m, name)
		m, _ = m.update(enter())
		m, _ = m.update(filled)

		open[m.screen.kind] = m
	}

	jobs := open[screenJobs]

	allocs, _ := jobs.update(enter())
	allocs, _ = allocs.update(allocsMsg(twoAllocs()))
	open[screenAllocations] = allocs

	tasks, _ := allocs.update(enter())
	open[screenTasks] = tasks

	groups, _ := jobs.update(key('t'))
	groups, _ = groups.update(taskGroupsMsg(twoGroups()))
	open[screenTaskGroups] = groups

	described, cmd := jobs.update(key('d'))
	open[screenDescribe] = drain(described, cmd)

	return open
}

func TestHints_EveryKeyTheHeaderOffersDoesSomething(t *testing.T) {
	r := require.New(t)

	for kind, m := range everyScreen(t) {
		for _, h := range m.screen.hints() {
			// A key in the header is a promise: pressing it opens something,
			// asks the cluster something or puts a question up. A key that
			// is drawn and does nothing is worse than no key.
			next, cmd := m.handleKey(keyOf(h.Key))

			did := cmd != nil ||
				next.screen != m.screen ||
				next.overlay != m.overlay ||
				next.err != nil

			r.True(did, "screen %d offers %s and nothing happens", kind, h.Key)
		}
	}
}

// keyOf reads a key the way the header writes it: <enter>, <ctrl-s>, <d>.
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
	}

	return key(rune(name[0]))
}
