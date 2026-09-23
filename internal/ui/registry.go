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

	serverHints = []hint{{Key: "<enter>", Description: "Details"}}

	fieldHints = []hint{{Key: "<c>", Description: "Copy the value"}}

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

	// hintsFor answers for a screen that is opened for different things and
	// can do different things with them.
	hintsFor func(s screen) []hint

	// cluster says the screen holds what belongs to the cluster rather than
	// to a namespace, so its title carries no namespace.
	cluster bool

	// fields says the screen reads as a list of fields, where a row is a
	// name and the value behind it rather than a resource.
	fields bool

	// readings are the resources whose usage the rows show, and reading is
	// how one of them is read. A screen without readings leaves both nil.
	readings func(m Model) []rowRef
	reading  func(client Client, ctx context.Context, ref rowRef) (nomad.ResourceUse, error)

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
		aliases: []string{"allocations", "allocation", "allocs", "alloc"},
		titles:  allocTitles,
		hints:   allocHints,

		// The allocations of a client sit on the screen of that client,
		// which answers for the machine as well as for the work on it.
		hintsFor: func(s screen) []hint {
			if s.isClient() {
				return clientHints
			}

			return allocHints
		},

		readings: func(m Model) []rowRef {
			allocs := m.visibleAllocs()

			refs := make([]rowRef, 0, len(m.index))
			for _, at := range m.index {
				// Only what runs has anything to report.
				if at < len(allocs) && allocs[at].Status == statusRunning {
					refs = append(refs, rowRef{namespace: allocs[at].Namespace, id: allocs[at].ID})
				}
			}

			return refs
		},
		reading: func(client Client, ctx context.Context, ref rowRef) (nomad.ResourceUse, error) {
			return client.AllocationUsage(ctx, ref.namespace, ref.id)
		},

		title: func(m Model, count int) string {
			if m.screen.taskGroup != "" {
				return sprintf("Allocations (Group: %s) [%d]", m.screen.taskGroup, count)
			}

			if m.screen.nodeID != "" {
				return sprintf("Client %s [%d]", m.screen.label, count)
			}

			return sprintf("Allocations (Job: %s) [%d]", m.screen.jobID, count)
		},
		fetch: func(m Model) tea.Cmd {
			client, screen := m.client, m.screen

			if screen.nodeID != "" {
				// The machine answers for its allocations and for itself:
				// the chart above them is what the host is doing.
				return tea.Batch(
					fetchList(func(ctx context.Context) ([]nomad.Alloc, error) {
						return client.NodeAllocations(ctx, screen.nodeID)
					}, func(items []nomad.Alloc) tea.Msg { return allocsMsg(items) }),
					fetchHost(client, screen.nodeID),
					fetchHostUse(client, screen.nodeID),
				)
			}

			return fetchList(func(ctx context.Context) ([]nomad.Alloc, error) {
				return client.Allocations(ctx, screen.namespace, screen.jobID)
			}, func(items []nomad.Alloc) tea.Msg { return allocsMsg(items) })
		},
		rows: func(m Model) []tableRow { return allocRows(m.visibleAllocs(), m.rowUsage) },
	},

	screenTasks: {
		titles: taskTitles,
		hints:  taskHints,

		title: func(m Model, count int) string {
			return sprintf("Tasks (Allocation: %s) [%d]", shortID(m.screen.allocID), count)
		},
		rows: func(m Model) []tableRow { return taskRows(m.tasks()) },
	},

	screenTaskGroups: {
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
		// Nomad calls them clients in its own interface; the command line
		// takes either word.
		name:    "Clients",
		stored:  "nodes",
		aliases: []string{"clients", "client", "nodes", "node", "no"},
		titles:  nodeTitles,
		hints:   nodeHints,
		cluster: true,

		readings: func(m Model) []rowRef {
			refs := make([]rowRef, 0, len(m.index))
			for _, at := range m.index {
				if at < len(m.nodes) {
					refs = append(refs, rowRef{id: m.nodes[at].ID})
				}
			}

			return refs
		},
		reading: func(client Client, ctx context.Context, ref rowRef) (nomad.ResourceUse, error) {
			return client.NodeUsage(ctx, ref.id)
		},
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

	screenServers: {
		name:    "Servers",
		stored:  "servers",
		aliases: []string{"servers", "server", "srv"},
		titles:  serverTitles,
		hints:   serverHints,
		cluster: true,
		fetch: func(m Model) tea.Cmd {
			return fetchList(m.client.Servers, func(items []nomad.Server) tea.Msg { return serversMsg(items) })
		},
		rows: func(m Model) []tableRow { return serverRows(m.servers) },
	},

	screenServer: {
		titles:  fieldTitles,
		hints:   fieldHints,
		cluster: true,
		fields:  true,

		title: func(m Model, _ int) string {
			return sprintf("Server %s", m.screen.label)
		},
		fetch: func(m Model) tea.Cmd {
			client, name := m.client, m.screen.label

			// The agent answers for itself, the raft says whether the rest
			// of the cluster still counts it.
			return tea.Batch(
				request(func(ctx context.Context) (nomad.Server, error) {
					return client.Server(ctx, name)
				}, func(server nomad.Server) tea.Msg { return serverMsg(server) }),
				fetchRaft(client),
			)
		},
		rows: func(m Model) []tableRow { return serverDetailRows(m.server, m.raft, m.raftErr) },
	},

	screenNodeEvents: {
		titles:  nodeEventTitles,
		cluster: true,

		title: func(m Model, count int) string {
			return sprintf("Events (Client: %s) [%d]", m.screen.label, count)
		},
		fetch: fetchNodeDetail,
		rows:  func(m Model) []tableRow { return nodeEventRows(m.nodeDetail.Events) },
	},

	screenNodeDrivers: {
		titles:  driverTitles,
		hints:   driverHints,
		cluster: true,

		title: func(m Model, count int) string {
			return sprintf("Drivers (Client: %s) [%d]", m.screen.label, count)
		},
		fetch: fetchNodeDetail,
		rows:  func(m Model) []tableRow { return driverRows(m.nodeDetail.Drivers) },
	},

	screenNodeDriver: {
		titles:  fieldTitles,
		hints:   fieldHints,
		cluster: true,
		fields:  true,

		title: func(m Model, count int) string {
			return sprintf("Driver %s [%d]", m.screen.label, count)
		},
		fetch: fetchNodeDetail,
		rows:  func(m Model) []tableRow { return fieldRows(m.driverAttributes()) },
	},

	screenNodeVolumes: {
		titles:  volumeTitles,
		cluster: true,

		title: func(m Model, count int) string {
			return sprintf("Host volumes (Client: %s) [%d]", m.screen.label, count)
		},
		fetch: fetchNodeDetail,
		rows:  func(m Model) []tableRow { return volumeRows(m.nodeDetail.Volumes) },
	},

	screenNodeAttributes: {
		titles:  fieldTitles,
		hints:   fieldHints,
		cluster: true,
		fields:  true,

		title: func(m Model, count int) string {
			return sprintf("Attributes (Client: %s) [%d]", m.screen.label, count)
		},
		fetch: fetchNodeDetail,
		rows:  func(m Model) []tableRow { return fieldRows(m.nodeDetail.Attributes) },
	},

	screenNodeMeta: {
		titles:  metaTitles,
		hints:   metaHints,
		cluster: true,
		fields:  true,

		title: func(m Model, count int) string {
			return sprintf("Meta (Client: %s) [%d]", m.screen.label, count)
		},
		fetch: fetchNodeMeta,
		rows:  func(m Model) []tableRow { return metaRows(m.nodeMeta) },
	},

	screenDescribe: {
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
