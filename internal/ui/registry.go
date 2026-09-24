package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// binding is one key a screen answers: how the keyboard names it, what the
// header calls it, and what it does. Each screen has a table of its own, so
// the same key can mean one thing here and another there, and a key that is
// not in the table of the open screen does nothing on it.
type binding struct {
	press string
	label string
	do    func(m Model) (Model, tea.Cmd)
}

// hint is the binding as the header and help write it: ctrl+s is <ctrl-s>.
func (b binding) hint() hint {
	return hint{Key: "<" + strings.ReplaceAll(b.press, "+", "-") + ">", Description: b.label}
}

// The keys each screen answers. What works everywhere is not here, it lives
// in help.
var (
	jobBindings = []binding{
		{press: "enter", label: "Allocations", do: openJobAllocations},
		{press: "space", label: "Mark", do: mark},
		{press: "ctrl+a", label: "Mark all", do: markAll},
		{press: "t", label: "Task groups", do: openJobGroups},
		{press: "d", label: "Describe", do: describeJob},
		{press: "h", label: "Job spec", do: showJobSpec},
		{press: "ctrl+s", label: "Start or stop", do: startStopJob},
		// The list of jobs reverts to the version before the one that runs;
		// the list of versions reverts to the one under the cursor.
		{press: "u", label: "Revert", do: revertJob},
		{press: "v", label: "Versions", do: openVersions},
		{press: "e", label: "Edit", do: editJob},
	}

	allocBindings = []binding{
		{press: "enter", label: "Tasks", do: openAllocation},
		{press: "d", label: "Describe", do: describeAllocation},
		{press: "r", label: "Restart", do: restartAllocation},
		{press: "ctrl+k", label: "Stop", do: stopAllocation},
		{press: "space", label: "Mark", do: mark},
		{press: "ctrl+a", label: "Mark all", do: markAll},
	}

	serverBindings = []binding{{press: "enter", label: "Details", do: openServer}}

	fieldBindings = []binding{{press: "c", label: "Copy the value", do: copyField}}

	serviceBindings = []binding{{press: "d", label: "Describe", do: describeService}}

	// textBindings are the keys of a screen that reads as text rather than
	// as a list.
	textBindings = []binding{
		{press: "w", label: "Wrap lines", do: wrapLines},
		{press: "ctrl+s", label: "Save", do: saveScreen},
	}

	namespaceBindings = []binding{{press: "e", label: "Edit", do: editNamespace}}

	// regionBindings and datacenterBindings are the keys of a list a region
	// or a datacenter is picked from.
	regionBindings     = []binding{{press: "enter", label: "Switch", do: chooseRegion}}
	datacenterBindings = []binding{{press: "enter", label: "Switch", do: chooseDatacenter}}
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
	keys   []binding

	// keysFor answers for a screen that is opened for different things and
	// can do different things with them.
	keysFor func(s screen) []binding

	// topics are what the cluster is asked to say about: a change in one of
	// them is this screen no longer being what it shows. A screen with none
	// is asked on its own.
	topics []string

	// cluster says the screen holds what belongs to the cluster rather than
	// to a namespace, so its title carries no namespace.
	cluster bool

	// fields says the screen reads as a list of fields, where a row is a
	// name and the value behind it rather than a resource.
	fields bool

	// ids name the resources of the screen, one per row, so that a mark
	// belongs to the resource and not to the line it sits on. A screen
	// whose rows answer no action of their own leaves it nil.
	ids func(m Model) []string

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

// resources is the one place that knows what urga can show. It is filled in
// init: the keys of a screen open other screens, which read this table, and
// a variable cannot be built out of code that reads it.
var resources map[screenKind]resource

func init() {
	resources = map[screenKind]resource{
		screenJobs: {
			name:    "Jobs",
			stored:  "jobs",
			aliases: []string{"jobs", "job", "jb"},
			titles:  jobTitles,
			keys:    jobBindings,
			ids:     jobIDs,
			topics:  []string{nomad.TopicJob},
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
			keys:    allocBindings,
			ids:     allocIDs,
			topics:  []string{nomad.TopicAllocation},

			// The allocations of a client sit on the screen of that client,
			// which answers for the machine as well as for the work on it.
			keysFor: func(s screen) []binding {
				if s.isClient() {
					return clientBindings
				}

				return allocBindings
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

				// The command line opens the allocations of the namespace, with
				// no job to name.
				if m.screen.jobID == "" {
					return sprintf("Allocations (%s) [%d]", namespaceLabel(m.screen.namespace), count)
				}

				return sprintf("Allocations (Job: %s) [%d]", m.screen.jobID, count)
			},
			fetch: func(m Model) tea.Cmd {
				client, screen := m.client, m.screen

				if screen.nodeID != "" {
					// The machine answers for its allocations and for itself:
					// the chart above them is what the host is doing. That is
					// read on the timer of the chart, not with the list.
					return tea.Batch(
						fetchList(func(ctx context.Context) ([]nomad.Alloc, error) {
							return client.NodeAllocations(ctx, screen.nodeID)
						}, func(items []nomad.Alloc) tea.Msg { return allocsMsg(items) }),
						fetchHost(client, screen.nodeID),
					)
				}

				return fetchList(func(ctx context.Context) ([]nomad.Alloc, error) {
					return client.Allocations(ctx, screen.namespace, screen.jobID)
				}, func(items []nomad.Alloc) tea.Msg { return allocsMsg(items) })
			},
			rows: func(m Model) []tableRow { return allocRows(m.visibleAllocs(), m.usage.rows) },
		},

		screenTasks: {
			titles: taskTitles,
			keys:   taskBindings,
			topics: []string{nomad.TopicAllocation},
			fetch:  fetchAllocation,

			title: func(m Model, count int) string {
				return sprintf("Tasks (Allocation: %s) [%d]", shortID(m.screen.allocID), count)
			},
			rows: func(m Model) []tableRow { return taskRows(m.tasks()) },
		},

		screenTaskGroups: {
			titles: taskGroupTitles,
			keys:   taskGroupBindings,

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
			keys:    deploymentBindings,
			topics:  []string{nomad.TopicDeployment},
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
			keys:    namespaceBindings,
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
			keys:    serviceBindings,
			topics:  []string{nomad.TopicService},
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
			topics:  []string{nomad.TopicEvaluation},
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
			keys:    nodeBindings,
			ids:     nodeIDs,
			topics:  []string{nomad.TopicNode},
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
			rows: func(m Model) []tableRow { return nodeRows(m.nodes, m.usage.rows) },
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
			topics:  []string{nomad.TopicNodePool},
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
			keys:    serverBindings,
			cluster: true,
			fetch: func(m Model) tea.Cmd {
				return fetchList(m.client.Servers, func(items []nomad.Server) tea.Msg { return serversMsg(items) })
			},
			rows: func(m Model) []tableRow { return serverRows(m.servers) },
		},

		screenServer: {
			titles:  fieldTitles,
			keys:    fieldBindings,
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
			keys:    driverBindings,
			cluster: true,

			title: func(m Model, count int) string {
				return sprintf("Drivers (Client: %s) [%d]", m.screen.label, count)
			},
			fetch: fetchNodeDetail,
			rows:  func(m Model) []tableRow { return driverRows(m.nodeDetail.Drivers) },
		},

		screenNodeDriver: {
			titles:  fieldTitles,
			keys:    fieldBindings,
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
			keys:    fieldBindings,
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
			keys:    metaBindings,
			cluster: true,
			fields:  true,

			title: func(m Model, count int) string {
				return sprintf("Meta (Client: %s) [%d]", m.screen.label, count)
			},
			fetch: fetchNodeMeta,
			rows:  func(m Model) []tableRow { return metaRows(m.nodeMeta) },
		},

		screenTaskEvents: {
			titles: taskEventTitles,
			topics: []string{nomad.TopicAllocation},
			fetch:  fetchAllocation,

			title: func(m Model, count int) string {
				return sprintf("Events (Task: %s) [%d]", m.screen.task, count)
			},
			rows: func(m Model) []tableRow { return taskEventRows(m.taskEvents()) },
		},

		screenJobVersions: {
			titles: versionTitles,
			keys:   versionBindings,

			title: func(m Model, count int) string {
				return sprintf("Versions (Job: %s) [%d]", m.screen.jobID, count)
			},
			fetch: func(m Model) tea.Cmd {
				client, screen := m.client, m.screen

				return fetchList(func(ctx context.Context) ([]nomad.JobVersion, error) {
					return client.JobVersions(ctx, screen.namespace, screen.jobID)
				}, func(items []nomad.JobVersion) tea.Msg {
					return versionsMsg{jobID: screen.jobID, versions: items}
				})
			},
			rows: func(m Model) []tableRow { return versionRows(m.versions) },
		},

		// The lists a region and a datacenter are picked from. The words that
		// switch them open them, so they have no alias of their own.
		screenRegions: {
			name:    "Regions",
			titles:  choiceTitles,
			keys:    regionBindings,
			cluster: true,
			fetch:   func(m Model) tea.Cmd { return fetchRegions(m.client) },
			rows:    func(m Model) []tableRow { return choiceRows(m.regions, m.regionInUse()) },
		},

		screenDatacenters: {
			name:    "Datacenters",
			titles:  choiceTitles,
			keys:    datacenterBindings,
			cluster: true,
			fetch:   func(m Model) tea.Cmd { return fetchDatacenters(m.client) },
			rows:    func(m Model) []tableRow { return choiceRows(m.datacenterChoices(), orEvery(m.datacenter)) },
		},

		screenDescribe: {
			keys:  textBindings,
			title: func(m Model, _ int) string { return m.screen.label },
		},

		screenLogs: {
			keys:  logBindings,
			title: func(m Model, _ int) string { return logsTitle(m.screen) },
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
