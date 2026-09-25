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
var (
	jobBindings = []binding{
		{press: "enter", label: "Allocations", do: openJobAllocations},
		{press: "space", label: "Mark", do: mark},
		{press: "ctrl+a", label: "Mark All", do: markAll},
		{press: "t", label: "Task Groups", do: openJobGroups},
		{press: "d", label: "Describe", do: describeJob},
		{press: "h", label: "Job Spec", do: showJobSpec},
		{press: "ctrl+s", label: "Start/Stop", do: startStopJob, writes: true},
		// The list of jobs reverts to the version before the one that runs;
		// the list of versions reverts to the one under the cursor.
		{press: "u", label: "Revert", do: revertJob, writes: true},
		{press: "v", label: "Versions", do: openVersions},
		{press: "l", label: "Logs", do: jobLogs},
		{press: "e", label: "Edit", do: editJob, writes: true},
		{press: "p", label: "Placement", do: jobPlacement, offered: jobWaits},
	}

	allocBindings = []binding{
		{press: "enter", label: "Tasks", do: openAllocation},
		{press: "d", label: "Describe", do: describeAllocation},
		{press: "r", label: "Restart", do: restartAllocation, writes: true},
		{press: "ctrl+k", label: "Stop", do: stopAllocation, writes: true},
		{press: "l", label: "Logs", do: listLogs},
		{press: "space", label: "Mark", do: mark},
		{press: "ctrl+a", label: "Mark All", do: markAll},
	}

	fieldBindings = []binding{{press: "c", label: "Copy", do: copyField}}

	serviceBindings = []binding{
		{press: "enter", label: "Instances", do: openServiceInstances},
		{press: "d", label: "Describe", do: describeService},
	}

	// textBindings are the keys of a screen that reads as text rather than
	// as a list.
	textBindings = []binding{
		{press: "w", label: "Toggle Wrap", do: wrapLines},
		{press: "ctrl+s", label: "Save", do: saveScreen},
	}
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
	reading  func(ctx context.Context, client Client, ref rowRef) (nomad.ResourceUse, error)

	// title overrides the "<name> (<namespace>) [n]" form for a screen that
	// says what it was opened for.
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
			aliases:  []string{"allocations", "allocation", "allocs", "alloc"},
			titles:   allocTitles,
			keys:     allocBindings,
			ids:      allocIDs,
			topics:   []string{nomad.TopicAllocation},
			readings: allocReadings,
			reading:  allocReading,

			title: func(m Model, count int) string {
				if m.screen.taskGroup != "" {
					return sprintf("Allocations (Group: %s) [%d]", m.screen.taskGroup, count)
				}

				// The command line opens the allocations of the namespace, with
				// no job to name.
				if m.screen.jobID == "" {
					return sprintf("Allocations (%s) [%d]", namespaceLabel(m.screen.namespace), count)
				}

				return sprintf("Allocations (Job: %s) [%d]", m.screen.jobID, count)
			},
			fetch: fetchAllocs,
			rows:  allocListRows,
		},

		// The allocations of a client sit on the screen of that client,
		// which answers for the machine as well as for the work on it.
		screenNode: {
			titles:   allocTitles,
			keys:     clientBindings,
			ids:      allocIDs,
			topics:   []string{nomad.TopicAllocation},
			readings: allocReadings,
			reading:  allocReading,

			title: func(m Model, count int) string {
				return sprintf("Client %s [%d]", m.screen.label, count)
			},
			fetch: func(m Model) tea.Cmd {
				// The machine answers for its allocations and for itself:
				// the chart above them is what the host is doing. That is
				// read on the timer of the chart, not with the list.
				return tea.Batch(fetchAllocs(m), fetchHost(m.client, m.screen.nodeID))
			},
			rows: allocListRows,
		},

		// The allocations of a deployment sit under its groups, and say
		// what the deployment made of each of them.
		screenDeployment: {
			titles:   deploymentAllocTitles,
			keys:     deploymentScreenBindings,
			ids:      allocIDs,
			topics:   []string{nomad.TopicAllocation, nomad.TopicDeployment},
			readings: allocReadings,
			reading:  allocReading,

			title: func(m Model, count int) string {
				return sprintf("Deployment %s (Job: %s) [%d]", shortID(m.screen.deploymentID), m.screen.jobID, count)
			},
			fetch: func(m Model) tea.Cmd {
				return tea.Batch(fetchAllocs(m), fetchDeployment(m.client, m.screen))
			},
			rows: func(m Model) []tableRow { return deploymentAllocRows(m.visibleAllocs(), m.usage.rows) },
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

		screenServiceInstances: {
			titles: instanceTitles,
			keys:   instanceBindings,
			topics: []string{nomad.TopicService},
			fetch:  fetchInstances,

			title: func(m Model, count int) string {
				return sprintf("Service %s (%s) [%d]", m.screen.label, namespaceLabel(m.screen.namespace), count)
			},
			rows: func(m Model) []tableRow { return instanceRows(m.instances, m.instanceChecks.byAlloc) },
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
			keys:    evaluationBindings,
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
				refs := make([]rowRef, 0, len(m.list.index))
				for _, at := range m.list.index {
					if at < len(m.nodes) {
						refs = append(refs, rowRef{id: m.nodes[at].ID})
					}
				}

				return refs
			},
			reading: func(ctx context.Context, client Client, ref rowRef) (nomad.ResourceUse, error) {
				return client.NodeUsage(ctx, ref.id)
			},
			fetch: func(m Model) tea.Cmd {
				return fetchList(m.client.Nodes, func(items []nomad.Node) tea.Msg { return nodesMsg(items) })
			},
			rows: func(m Model) []tableRow { return nodeRows(m.nodes, m.usage.rows) },
		},

		screenVariables: {
			stored:  "variables",
			aliases: []string{"variables", "variable", "vars", "var"},
			open:    func() page { return variablesPage{} },
		},

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

		screenDescribe: {
			keys:  textBindings,
			title: func(m Model, _ int) string { return m.screen.label },
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
