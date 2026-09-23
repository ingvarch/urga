package ui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// The keys each screen answers. What works everywhere is not here, it lives
// in help.
var (
	jobHints = []hint{
		{Key: "<enter>", Description: "Allocations"},
		{Key: "<t>", Description: "Task groups"},
		{Key: "<d>", Description: "Describe"},
		{Key: "<h>", Description: "Job spec"},
		{Key: "<ctrl-s>", Description: "Start or stop"},
		{Key: "<u>", Description: "Revert"},
		{Key: "<e>", Description: "Edit"},
	}

	allocHints = []hint{
		{Key: "<enter>", Description: "Tasks"},
		{Key: "<d>", Description: "Describe"},
		{Key: "<r>", Description: "Restart"},
		{Key: "<ctrl-k>", Description: "Stop"},
	}

	describeHints = []hint{{Key: "<d>", Description: "Describe"}}

	namespaceHints = []hint{{Key: "<e>", Description: "Edit"}}
)

// resource is everything a screen knows about itself: what it is called, what
// its columns are, which keys it answers, how it is asked for and how its
// rows are built. One entry per screen, so that adding a resource is adding
// a line here instead of editing a switch in every file.
type resource struct {
	// name labels the screen in its title.
	name string

	// stored is how the screen is written down between runs. Screens that
	// are opened from another one have none: a session comes back to a list,
	// not to the allocations of a job it no longer remembers.
	stored string

	// aliases are the words the command line takes for it. The first one is
	// what the prompt suggests.
	aliases []string

	titles []string
	hints  []hint

	// cluster says the screen holds what belongs to the cluster rather than
	// to a namespace, so its title carries no namespace.
	cluster bool

	// readings says the rows of this screen carry what they are using.
	readings bool

	// title overrides the "<name> (<namespace>) [n]" form for a screen that
	// says what it was opened for.
	title func(m Model, count int) string

	// fetch asks the cluster for the rows; nil is a screen that is not
	// polled, because what it shows arrives another way.
	fetch func(m Model) tea.Cmd

	// rows are what the screen shows.
	rows func(m Model) []tableRow
}

// resources is the one place that knows what urga can show.
var resources = map[screenKind]resource{
	screenJobs: {
		name:    "Jobs",
		stored:  "jobs",
		aliases: []string{"jobs", "job", "jb"},
		titles:  jobTitles,
		hints:   jobHints,
		fetch: func(m Model) tea.Cmd {
			client, namespace := m.client, m.namespace

			return fetchList(func(ctx context.Context) ([]nomad.Job, error) {
				return client.Jobs(ctx, namespace)
			}, func(items []nomad.Job) tea.Msg { return jobsMsg(items) })
		},
		rows: func(m Model) []tableRow { return jobRows(m.jobs) },
	},

	screenAllocations: {
		name:    "Allocations",
		aliases: []string{"allocations", "allocation", "allocs", "alloc"},
		titles:  allocTitles,
		hints:   allocHints,

		readings: true,

		title: func(m Model, count int) string {
			if m.screen.taskGroup != "" {
				return sprintf("Allocations (Group: %s) [%d]", m.screen.taskGroup, count)
			}

			return sprintf("Allocations (Job: %s) [%d]", m.screen.jobID, count)
		},
		fetch: func(m Model) tea.Cmd {
			client, screen := m.client, m.screen

			return fetchList(func(ctx context.Context) ([]nomad.Alloc, error) {
				return client.Allocations(ctx, screen.namespace, screen.jobID)
			}, func(items []nomad.Alloc) tea.Msg { return allocsMsg(items) })
		},
		rows: func(m Model) []tableRow { return allocRows(m.visibleAllocs(), m.rowUsage) },
	},

	screenTasks: {
		name:   "Tasks",
		titles: taskTitles,
		hints:  taskHints,

		title: func(m Model, count int) string {
			return sprintf("Tasks (Allocation: %s) [%d]", shortID(m.screen.allocID), count)
		},
		rows: func(m Model) []tableRow { return taskRows(m.tasks()) },
	},

	screenTaskGroups: {
		name:   "Task Groups",
		titles: taskGroupTitles,
		hints:  taskGroupHints,

		title: func(m Model, count int) string {
			return sprintf("Task Groups (Job: %s) [%d]", m.screen.jobID, count)
		},
		fetch: func(m Model) tea.Cmd {
			client, screen := m.client, m.screen

			return fetchList(func(ctx context.Context) ([]nomad.TaskGroup, error) {
				return client.TaskGroups(ctx, screen.namespace, screen.jobID)
			}, func(items []nomad.TaskGroup) tea.Msg { return taskGroupsMsg(items) })
		},
		rows: func(m Model) []tableRow { return taskGroupRows(m.groups) },
	},

	screenDeployments: {
		name:    "Deployments",
		stored:  "deployments",
		aliases: []string{"deployments", "deployment", "dp"},
		titles:  deploymentTitles,
		hints:   deploymentHints,
		fetch: func(m Model) tea.Cmd {
			client, namespace := m.client, m.namespace

			return fetchList(func(ctx context.Context) ([]nomad.Deployment, error) {
				return client.Deployments(ctx, namespace)
			}, func(items []nomad.Deployment) tea.Msg { return deploymentsMsg(items) })
		},
		rows: func(m Model) []tableRow { return deploymentRows(m.deployments) },
	},

	screenNamespaces: {
		name:    "Namespaces",
		stored:  "namespaces",
		aliases: []string{"namespaces", "namespace", "ns"},
		titles:  namespaceTitles,
		hints:   namespaceHints,
		cluster: true,
		fetch: func(m Model) tea.Cmd {
			return fetchList(m.client.Namespaces, func(items []nomad.Namespace) tea.Msg { return namespacesMsg(items) })
		},
		rows: func(m Model) []tableRow { return namespaceRows(m.namespaces) },
	},

	screenServices: {
		name:    "Services",
		stored:  "services",
		aliases: []string{"services", "service", "svc"},
		titles:  serviceTitles,
		hints:   describeHints,
		fetch: func(m Model) tea.Cmd {
			client, namespace := m.client, m.namespace

			return fetchList(func(ctx context.Context) ([]nomad.Service, error) {
				return client.Services(ctx, namespace)
			}, func(items []nomad.Service) tea.Msg { return servicesMsg(items) })
		},
		rows: func(m Model) []tableRow { return serviceRows(m.services) },
	},

	screenEvaluations: {
		name:    "Evaluations",
		stored:  "evaluations",
		aliases: []string{"evaluations", "evaluation", "evals", "eval", "ev"},
		titles:  evaluationTitles,
		fetch: func(m Model) tea.Cmd {
			client, namespace := m.client, m.namespace

			return fetchList(func(ctx context.Context) ([]nomad.Evaluation, error) {
				return client.Evaluations(ctx, namespace)
			}, func(items []nomad.Evaluation) tea.Msg { return evaluationsMsg(items) })
		},
		rows: func(m Model) []tableRow { return evaluationRows(m.evaluations) },
	},

	screenNodes: {
		name:     "Nodes",
		stored:   "nodes",
		aliases:  []string{"nodes", "node", "no"},
		titles:   nodeTitles,
		hints:    nodeHints,
		cluster:  true,
		readings: true,
		fetch: func(m Model) tea.Cmd {
			return fetchList(m.client.Nodes, func(items []nomad.Node) tea.Msg { return nodesMsg(items) })
		},
		rows: func(m Model) []tableRow { return nodeRows(m.nodes, m.rowUsage) },
	},

	screenVariables: {
		name:    "Variables",
		stored:  "variables",
		aliases: []string{"variables", "variable", "vars", "var"},
		titles:  variableTitles,
		fetch: func(m Model) tea.Cmd {
			client, namespace := m.client, m.namespace

			return fetchList(func(ctx context.Context) ([]nomad.Variable, error) {
				return client.Variables(ctx, namespace)
			}, func(items []nomad.Variable) tea.Msg { return variablesMsg(items) })
		},
		rows: func(m Model) []tableRow { return variableRows(m.variables) },
	},

	screenNodePools: {
		name:    "Node Pools",
		stored:  "nodepools",
		aliases: []string{"nodepools", "nodepool", "np"},
		titles:  nodePoolTitles,
		cluster: true,
		fetch: func(m Model) tea.Cmd {
			return fetchList(m.client.NodePools, func(items []nomad.NodePool) tea.Msg { return nodePoolsMsg(items) })
		},
		rows: func(m Model) []tableRow { return nodePoolRows(m.nodePools) },
	},

	screenDescribe: {
		hints: describeHints,
		title: func(m Model, _ int) string { return m.screen.label },
	},

	screenLogs: {
		hints: logHints,
		title: func(m Model, _ int) string { return logsTitle(m.screen) },
	},
}

// of is the resource a screen shows.
func (s screen) of() resource { return resources[s.kind] }

// sprintf is fmt.Sprintf, named short because the titles read better that
// way.
func sprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }
