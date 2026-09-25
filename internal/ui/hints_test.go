package ui

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
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
// kept by name, which is what a failure reports.
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

		// The deployment of the list, with a canary of web that waits to be
		// promoted under the cursor.
		deployment: nomad.DeploymentDetail{
			Deployment: nomad.Deployment{ID: "dep-1", JobID: "web", Namespace: "production", Status: "running"},
			Groups:     []nomad.DeploymentGroup{{Name: "frontend", DesiredTotal: 2, Placed: 1, DesiredCanaries: 1, PlacedCanaries: 1}},
		},
		deploymentAllocs: []nomad.Alloc{
			{ID: "canary-1", Namespace: "production", JobID: "web", TaskGroup: "frontend", Status: "running", Canary: true},
		},

		// The directory of the task server, with a file to open.
		files: map[string][]nomad.File{
			"/server": {{Name: "local", Dir: true}, {Name: "app.env", Size: 42, Mode: "-rw-r--r--"}},
		},
		file: &nomad.LogStream{Lines: make(chan string)},

		// A registration left behind first: there is one to delete.
		instances: []nomad.ServiceInstance{
			{ID: "reg-1", Service: "api", Namespace: "production", JobID: "web", AllocID: "alloc-lost", Address: "10.0.0.7", Port: 80, AllocStatus: "lost"},
			{ID: "reg-2", Service: "api", Namespace: "production", JobID: "web", AllocID: "alloc-1", Address: "10.0.0.8", Port: 80, AllocStatus: "running"},
		},

		variable: nomad.VariableDetail{
			Variable: nomad.Variable{Path: "nomad/jobs/web", Namespace: "production"},
			Items:    map[string]string{"DB_HOST": "10.0.0.5"},
		},
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

	// The instances of a service.
	instances, cmd := open["services"].update(enter())
	open["instances"] = drain(instances, cmd)

	// The values of a variable.
	variable, cmd := open["variables"].update(enter())
	open["variable"] = drain(variable, cmd)

	// The screen of a deployment, opened from the list of them.
	deployment, cmd := open["deployments"].update(enter())
	open["deployment"] = drain(deployment, cmd)

	// The files of a task, and one of them open.
	files, cmd := open["tasks"].update(key('b'))
	open["files"] = drain(files, cmd)

	files, _ = open["files"].update(key('G'))
	file, cmd := files.update(enter())
	open["file"] = drain(file, cmd)

	// The logs of a job: which task, then that task in every allocation.
	pick, cmd := jobs.update(key('l'))
	open["logtasks"] = drain(pick, cmd)

	jobLogs, cmd := open["logtasks"].update(enter())
	open["joblogs"] = drain(jobLogs, cmd)

	// The plan of an edited job: the file changed, or there is nothing to
	// plan.
	editing := jobs
	editing.opts.Editor = &fakeEditor{replace: "job \"web\" {\n  type = \"batch\"\n}"}
	edited, cmd := editing.update(key('e'))
	open["plan"] = follow(edited, cmd, 6)

	// The list the clusters of the settings are picked from.
	clusters := New(client, Options{
		Cluster:  "dev",
		Clusters: []string{"dev", "prod"},
		Connect:  func(name string) (Connection, error) { return Connection{Name: name, Client: client}, nil },
		Version:  "v-test",
	})
	clusters, _ = clusters.update(sizeMsg())
	clusters, _ = runLine(clusters, "ctx")
	open["clusters"] = clusters

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
			did := cmd != nil || changed(m, next)

			r.True(did, "the %s screen offers %s and nothing happens", name, h.Key)
		}
	}
}

// keyOf reads a key the way the header writes it: <enter>, <ctrl-s>, <d>. A
// key it cannot spell would be pressed as its first letter, which is why
// every control key is read as one, whichever letter it takes.
func keyOf(shown string) tea.KeyPressMsg {
	name := shown[1 : len(shown)-1]

	if letter, ok := strings.CutPrefix(name, "ctrl-"); ok && len(letter) == 1 {
		return ctrlKey(rune(letter[0]))
	}

	switch name {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
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
			// The buttons at the foot of a plan are chosen and pressed the
			// way the buttons of a question are: those keys are the
			// dialog's, and help names them.
			if _, _, buttons := m.buttonKey(press); buttons {
				continue
			}

			next, cmd := m.handleKey(press)
			did := cmd != nil || changed(m, next)

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
		for _, k := range m.pageKeys() {
			named(fmt.Sprintf("%s on the %s screen", k.press, name), k.label)
		}
	}

	for _, h := range slices.Concat(generalHints, navigationHints) {
		named(h.Key+" in help", h.Description)
	}
}

func TestEveryScreen_CoversEveryPage(t *testing.T) {
	r := require.New(t)

	covered := map[string]bool{}
	for _, m := range everyScreen(t) {
		covered[reflect.TypeOf(m.screen.page).Name()] = true
	}

	// A screen left out of the tests of the keys is a screen whose keys
	// read-only is never checked against. Every page counts: they are read
	// out of the source, not out of a list someone has to keep.
	pages := pageTypes(t)
	r.NotEmpty(pages)

	for _, name := range pages {
		r.True(covered[name], "no %s in everyScreen", name)
	}
}

// pageTypes are the pages of the package, by name: every type that labels
// a box with title(env, int).
func pageTypes(t *testing.T) []string {
	t.Helper()

	files, err := filepath.Glob("*.go")
	require.NoError(t, err)

	fset := token.NewFileSet()
	names := []string{}

	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, name, nil, 0)
		require.NoError(t, err)

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "title" || fn.Type.Params.NumFields() != 2 {
				continue
			}

			// A page is a value: a pointer receiver would not be one.
			receiver, ok := fn.Recv.List[0].Type.(*ast.Ident)
			require.True(t, ok, "the title of a page in %s has a pointer receiver", name)

			names = append(names, receiver.Name)
		}
	}

	return names
}

// changed says a key did something to the model. What a model is given to
// connect and switch with is a function, and two functions are never equal:
// they are left out of the comparison.
func changed(before, after Model) bool {
	before.opts.Connect, after.opts.Connect = nil, nil
	before.opts.InRegion, after.opts.InRegion = nil, nil

	return !reflect.DeepEqual(before, after)
}
