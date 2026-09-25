package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// binding is one key a screen answers: how the keyboard names it, what the
// header calls it, and what it does. Each screen has a table of its own, so
// the same key can mean one thing here and another there, and a key that is
// not in the table of the open screen does nothing on it.
type binding struct {
	press string
	label string
	do    func(m Model) (Model, tea.Cmd)

	// writes says the key changes the cluster, which read-only takes away.
	writes bool

	// offered says the key does something in the state the screen is in.
	// Nil is always.
	offered func(m Model) bool
}

// hint is the binding as the header and help write it: ctrl+s is <ctrl-s>.
func (b binding) hint() hint {
	return hint{Key: "<" + strings.ReplaceAll(b.press, "+", "-") + ">", Description: b.label}
}

// The keys each screen answers. What works everywhere is not here, it lives
// in help.
var ()

// resource is everything a screen knows about itself: what it is called, what
// its columns are, which keys it answers, how it is asked for and how its
// rows are built. One entry per screen, so that adding a resource is adding
// a line here instead of editing a switch in every file.
type resource struct {
	// stored is how the screen is written down between runs. Screens that
	// are opened from another one have none: a session comes back to a list,
	// not to the allocations of a job it no longer remembers.
	stored string

	// aliases are the words the command line takes for it. The first one is
	// what the prompt suggests.
	aliases []string

	titles []string
	keys   []binding

	// topics are what the cluster is asked to say about: a change in one of
	// them is this screen no longer being what it shows. A screen with none
	// is asked on its own.
	topics []string

	// title labels the box.
	title func(m Model, count int) string

	// fetch asks the cluster for the rows; nil is a screen that is not
	// polled, because what it shows arrives another way.
	fetch func(m Model) tea.Cmd

	// rows are what the screen shows.
	rows func(m Model) []tableRow

	// open makes the page of a screen that is a type of its own; such a
	// screen needs nothing else here but the names it is opened by.
	open func() page
}

// resources is the one place that knows what urga can show. It is filled in
// init: the keys of a screen open other screens, which read this table, and
// a variable cannot be built out of code that reads it.
var resources map[screenKind]resource

func init() {
	resources = map[screenKind]resource{
		screenJobs: {
			stored:  "jobs",
			aliases: []string{"jobs", "job", "jb"},
			open:    func() page { return jobsPage{} },
		},

		screenAllocations: {
			aliases: []string{"allocations", "allocation", "allocs", "alloc"},
			open:    func() page { return allocationsPage{} },
		},

		screenLogTasks: {
			titles: logTaskTitles,
			keys:   logTaskBindings,
			title:  func(m Model, _ int) string { return m.logPick.title },
			rows:   func(m Model) []tableRow { return logTaskRows(m.logPick.choices) },
		},

		screenJobLogs: {
			keys:  jobLogBindings,
			title: func(m Model, _ int) string { return jobLogsTitle(m.screen, m.jobLogs) },
		},

		screenFiles: {
			titles: fileTitles,
			keys:   fileBindings,
			fetch:  fetchFiles,

			title: func(m Model, count int) string {
				return sprintf("Files (Allocation: %s, %s) [%d]", shortID(m.screen.allocID), m.screen.path, count)
			},
			rows: func(m Model) []tableRow { return fileRows(m.entries()) },
		},

		screenDeployments: {
			stored:  "deployments",
			aliases: []string{"deployments", "deployment", "dp"},
			open:    func() page { return deploymentsPage{} },
		},

		screenServices: {
			stored:  "services",
			aliases: []string{"services", "service", "svc"},
			open:    func() page { return servicesPage{} },
		},

		screenEvaluations: {
			stored:  "evaluations",
			aliases: []string{"evaluations", "evaluation", "evals", "eval", "ev"},
			open:    func() page { return evaluationsPage{} },
		},

		screenNodes: {
			// Nomad calls them clients in its own interface; the command line
			// takes either word.
			stored:  "nodes",
			aliases: []string{"clients", "client", "nodes", "node", "no"},
			open:    func() page { return nodesPage{} },
		},

		screenVariables: {
			stored:  "variables",
			aliases: []string{"variables", "variable", "vars", "var"},
			open:    func() page { return variablesPage{} },
		},

		// The lists a region and a datacenter are picked from. The words that
		// switch them open them, so they have no alias of their own.
		screenRegions:     {open: func() page { return regionsPage{} }},
		screenDatacenters: {open: func() page { return datacentersPage{} }},
		screenClusters:    {open: func() page { return clustersPage{} }},

		screenNamespaces: {
			stored:  "namespaces",
			aliases: []string{"namespaces", "namespace", "ns"},
			open:    func() page { return namespacesPage{} },
		},

		screenServers: {
			stored:  "servers",
			aliases: []string{"servers", "server", "srv"},
			open:    func() page { return serversPage{} },
		},

		screenNodePools: {
			stored:  "nodepools",
			aliases: []string{"nodepools", "nodepool", "np"},
			open:    func() page { return nodePoolsPage{} },
		},

		screenPlan: {
			keys:  planBindings,
			title: func(m Model, _ int) string { return planTitle(m) },
		},

		screenLogs: {
			keys:  logBindings,
			title: func(m Model, _ int) string { return logsTitle(m.screen, m.logs.finished) },
		},

		screenFile: {
			keys:  fileTextBindings,
			title: func(m Model, _ int) string { return fileTitle(m.screen, m.logs) },
		},
	}

	// What is read out of the table is read once it is there.
	commandAliases = aliasIndex()
	commandNames = nameIndex()
	screenOfName = storedIndex()
}

// of is the resource a screen shows.
func (s screen) of() resource { return resources[s.kind] }

// sprintf is fmt.Sprintf, named short because the titles read better that
// way.
func sprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }
