package ui

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
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
		versions:    threeVersions(),
		variables:   []nomad.Variable{{Path: "nomad/jobs/web", Namespace: "production"}},
		nodePools:   []nomad.NodePool{{Name: "default"}},
		use:         map[string]nomad.ResourceUse{"node-1": {}},
		servers:     twoServers(),
		describe:    "{}",
		spec:        nomad.JobSource{Source: "job \"web\" {}"},
		logs:        &nomad.LogStream{Lines: make(chan string)},
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

	// The screens that hang off a job and a task.
	versions, cmd := jobs.update(key('v'))
	open["versions"] = drain(versions, cmd)

	events, _ := open["tasks"].update(key('e'))
	open["taskevents"] = events

	logs, cmd := open["tasks"].update(enter())
	open["logs"] = drain(logs, cmd)

	// The lists a region and a datacenter are picked from.
	regions, _ := jobs.update(regionsMsg([]string{"eu", "us"}))
	regions, _ = runLine(regions, "region")
	open["regions"] = regions

	datacenters, _ := jobs.update(datacentersMsg{names: []string{"dc1"}})
	datacenters, _ = runLine(datacenters, "dc")
	open["datacenters"] = datacenters

	return open
}

func TestHints_EveryKeyTheHeaderOffersDoesSomething(t *testing.T) {
	r := require.New(t)

	for name, m := range everyScreen(t) {
		// Whatever the screen was left holding says nothing about the key
		// that is about to be pressed.
		m = m.quiet()

		for _, h := range m.hints() {
			// A key in the header is a promise: pressing it opens something,
			// asks the cluster something or puts a question up. A key that
			// is drawn and does nothing is worse than no key.
			next, cmd := m.handleKey(keyOf(h.Key))

			// Anything at all: another screen, a question, a mark, a way
			// of reading the text, or a request to the cluster.
			did := cmd != nil || !reflect.DeepEqual(next, m)

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
	case "ctrl-a":
		return ctrlKey('a')
	case "space":
		return space()
	}

	return key(rune(name[0]))
}

// screenKeys are the keys a screen could answer on its own: every letter and
// every control letter, enter and space, less the keys that work on every
// screen and are listed in help instead.
func screenKeys() []tea.KeyPressMsg {
	everywhere := map[string]bool{"j": true, "k": true, "g": true, "q": true, "ctrl+b": true, "ctrl+f": true, "ctrl+c": true}

	keys := []tea.KeyPressMsg{{Code: tea.KeyEnter}, space()}

	for letter := 'a'; letter <= 'z'; letter++ {
		for _, press := range []tea.KeyPressMsg{key(letter), ctrlKey(letter)} {
			if !everywhere[press.String()] {
				keys = append(keys, press)
			}
		}
	}

	return keys
}

func TestHints_EveryKeyThatDoesSomethingIsInTheHeader(t *testing.T) {
	r := require.New(t)

	// A key that edits writes its file before anything else happens.
	t.Setenv("TMPDIR", t.TempDir())

	for name, m := range everyScreen(t) {
		m = m.quiet()

		offered := map[string]bool{}
		for _, h := range m.hints() {
			offered[keyOf(h.Key).String()] = true
		}

		for _, press := range screenKeys() {
			next, cmd := m.handleKey(press)
			did := cmd != nil || !reflect.DeepEqual(next, m)

			// The header is where a key is found: one that works without
			// being there is one nobody learns about.
			r.False(did && !offered[press.String()], "the %s screen answers %s without offering it", name, press.String())
		}
	}
}

func TestHints_EveryKeyIsNamedInOneOrTwoWords(t *testing.T) {
	r := require.New(t)

	// The header is read at a glance: a key says what it does in a word or
	// two, and a sentence there is cut off or pushes the other keys out.
	named := func(where, label string) {
		words := len(strings.Fields(label))
		r.True(words >= 1 && words <= 2, "%s is named %q, %d words", where, label, words)
	}

	for name, m := range everyScreen(t) {
		for _, b := range m.screen.bindings() {
			named(fmt.Sprintf("%s on the %s screen", b.press, name), b.label)
		}
	}

	for _, h := range slices.Concat(generalHints, navigationHints) {
		named(h.Key+" in help", h.Description)
	}
}
