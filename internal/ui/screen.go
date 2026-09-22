package ui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// screenKind is the resource the body shows.
type screenKind int

const (
	screenJobs screenKind = iota
	screenAllocations
	screenTasks
	screenDeployments
	screenNamespaces
	screenServices
	screenEvaluations
	screenNodes
	screenVariables
	screenNodePools
	screenDescribe
	screenLogs
	screenTaskGroups
)

// screenNames label a screen in its title.
var screenNames = map[screenKind]string{
	screenJobs:        "Jobs",
	screenAllocations: "Allocations",
	screenTasks:       "Tasks",
	screenDeployments: "Deployments",
	screenNamespaces:  "Namespaces",
	screenServices:    "Services",
	screenEvaluations: "Evaluations",
	screenNodes:       "Nodes",
	screenVariables:   "Variables",
	screenNodePools:   "Node Pools",
	screenTaskGroups:  "Task Groups",
}

// clusterWide screens hold what belongs to the cluster, not to a namespace.
var clusterWide = map[screenKind]bool{
	screenNamespaces: true,
	screenNodes:      true,
	screenNodePools:  true,
}

// screen is what is open: the resource and what it was opened for. The
// namespace travels with it, a job of another namespace keeps its own.
type screen struct {
	kind      screenKind
	namespace string

	jobID   string
	allocID string

	// label titles a screen that is about one thing, like a description.
	label string

	// taskGroup narrows the allocations to one group of the job.
	taskGroup string

	// task and source are whose output the log screen follows.
	task   string
	source string
}

// titles are the columns of a screen.
func (s screen) titles() []string {
	switch s.kind {
	case screenAllocations:
		return allocTitles
	case screenTasks:
		return taskTitles
	case screenTaskGroups:
		return taskGroupTitles
	case screenDeployments:
		return deploymentTitles
	case screenNamespaces:
		return namespaceTitles
	case screenServices:
		return serviceTitles
	case screenEvaluations:
		return evaluationTitles
	case screenNodes:
		return nodeTitles
	case screenVariables:
		return variableTitles
	case screenNodePools:
		return nodePoolTitles
	default:
		return jobTitles
	}
}

// hints are the keys the open resource answers. Keys that work everywhere
// are not here, they live in help.
func (s screen) hints() []hint {
	switch s.kind {
	case screenJobs:
		return jobHints
	case screenAllocations:
		return allocHints
	case screenTasks:
		return taskHints
	case screenTaskGroups:
		return taskGroupHints
	case screenLogs:
		return logHints
	case screenDeployments, screenServices:
		return describeHints
	case screenNamespaces:
		return namespaceHints
	default:
		return nil
	}
}

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

// title labels the box with what it holds and how much of it. The count is
// what is on the screen, which the filter narrows.
func (m Model) title() string {
	count := len(m.table.rows)

	switch m.screen.kind {
	case screenDescribe:
		return m.screen.label

	case screenLogs:
		return logsTitle(m.screen)

	case screenAllocations:
		if m.screen.taskGroup != "" {
			return fmt.Sprintf("Allocations (Group: %s) [%d]", m.screen.taskGroup, count)
		}

		return fmt.Sprintf("Allocations (Job: %s) [%d]", m.screen.jobID, count)

	case screenTaskGroups:
		return fmt.Sprintf("Task Groups (Job: %s) [%d]", m.screen.jobID, count)

	case screenTasks:
		return fmt.Sprintf("Tasks (Allocation: %s) [%d]", shortID(m.screen.allocID), count)
	}

	name := screenNames[m.screen.kind]

	if clusterWide[m.screen.kind] {
		return fmt.Sprintf("%s [%d]", name, count)
	}

	return fmt.Sprintf("%s (%s) [%d]", name, namespaceLabel(m.namespace), count)
}

// rows are what the open screen shows.
func (m Model) rows() []tableRow {
	switch m.screen.kind {
	case screenAllocations:
		return allocRows(m.visibleAllocs(), m.rowUsage)
	case screenTasks:
		return taskRows(m.tasks())
	case screenTaskGroups:
		return taskGroupRows(m.groups)
	case screenDeployments:
		return deploymentRows(m.deployments)
	case screenNamespaces:
		return namespaceRows(m.namespaces)
	case screenServices:
		return serviceRows(m.services)
	case screenEvaluations:
		return evaluationRows(m.evaluations)
	case screenNodes:
		return nodeRows(m.nodes, m.rowUsage)
	case screenVariables:
		return variableRows(m.variables)
	case screenNodePools:
		return nodePoolRows(m.nodePools)
	default:
		return jobRows(m.jobs)
	}
}

// fetch asks the cluster for what the open screen shows. Every answer comes
// back as a message.
func (m Model) fetch() tea.Cmd {
	client, screen, namespace := m.client, m.screen, m.namespace

	switch screen.kind {
	case screenAllocations:
		return fetchList(func(ctx context.Context) ([]nomad.Alloc, error) {
			return client.Allocations(ctx, screen.namespace, screen.jobID)
		}, func(items []nomad.Alloc) tea.Msg { return allocsMsg(items) })

	case screenTaskGroups:
		return fetchList(func(ctx context.Context) ([]nomad.TaskGroup, error) {
			return client.TaskGroups(ctx, screen.namespace, screen.jobID)
		}, func(items []nomad.TaskGroup) tea.Msg { return taskGroupsMsg(items) })

	case screenTasks:
		// The tasks are part of the allocation, the screen under them polls.
		return nil

	case screenDescribe:
		// A description is a snapshot of one moment, it is not polled.
		return nil

	case screenLogs:
		// The stream pushes on its own, there is nothing to ask again for.
		return nil

	case screenDeployments:
		return fetchList(func(ctx context.Context) ([]nomad.Deployment, error) {
			return client.Deployments(ctx, namespace)
		}, func(items []nomad.Deployment) tea.Msg { return deploymentsMsg(items) })

	case screenNamespaces:
		return fetchList(client.Namespaces, func(items []nomad.Namespace) tea.Msg { return namespacesMsg(items) })

	case screenServices:
		return fetchList(func(ctx context.Context) ([]nomad.Service, error) {
			return client.Services(ctx, namespace)
		}, func(items []nomad.Service) tea.Msg { return servicesMsg(items) })

	case screenEvaluations:
		return fetchList(func(ctx context.Context) ([]nomad.Evaluation, error) {
			return client.Evaluations(ctx, namespace)
		}, func(items []nomad.Evaluation) tea.Msg { return evaluationsMsg(items) })

	case screenNodes:
		return fetchList(client.Nodes, func(items []nomad.Node) tea.Msg { return nodesMsg(items) })

	case screenVariables:
		return fetchList(func(ctx context.Context) ([]nomad.Variable, error) {
			return client.Variables(ctx, namespace)
		}, func(items []nomad.Variable) tea.Msg { return variablesMsg(items) })

	case screenNodePools:
		return fetchList(client.NodePools, func(items []nomad.NodePool) tea.Msg { return nodePoolsMsg(items) })

	default:
		return fetchList(func(ctx context.Context) ([]nomad.Job, error) {
			return client.Jobs(ctx, namespace)
		}, func(items []nomad.Job) tea.Msg { return jobsMsg(items) })
	}
}

// fetchList asks the cluster in the background and hands the answer over as a
// message. Nothing here touches the model.
func fetchList[T any](load func(ctx context.Context) ([]T, error), wrap func([]T) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		items, err := load(ctx)
		if err != nil {
			return errMsg{err: err}
		}

		return wrap(items)
	}
}

// open drills into what the cursor is on, if there is anything below it.
func (m Model) open() (Model, tea.Cmd) {
	row, ok := m.selectedIndex()
	if !ok {
		return m, nil
	}

	switch m.screen.kind {
	case screenJobs:
		if row >= len(m.jobs) {
			return m, nil
		}

		job := m.jobs[row]

		return m.push(screen{kind: screenAllocations, namespace: job.Namespace, jobID: job.ID})

	case screenAllocations:
		allocs := m.visibleAllocs()
		if row >= len(allocs) {
			return m, nil
		}

		alloc := allocs[row]

		return m.push(screen{
			kind:      screenTasks,
			namespace: alloc.Namespace,
			jobID:     alloc.JobID,
			allocID:   alloc.ID,
		})

	case screenTasks:
		return m.openLogs(nomad.LogStdout)

	case screenTaskGroups:
		if row >= len(m.groups) {
			return m, nil
		}

		group := m.groups[row]

		return m.push(screen{
			kind:      screenAllocations,
			namespace: m.screen.namespace,
			jobID:     group.JobID,
			taskGroup: group.Name,
		})
	}

	return m, nil
}

// selectedIndex is the resource the cursor is on. The filter shifts the rows,
// so the row number is not the number of the resource.
func (m Model) selectedIndex() (int, bool) {
	if m.table.cursor < 0 || m.table.cursor >= len(m.index) {
		return 0, false
	}

	return m.index[m.table.cursor], true
}

// show switches to a resource, which is what the command prompt does.
func (m Model) show(kind screenKind) (Model, tea.Cmd) {
	if kind == m.screen.kind {
		return m.enter()
	}

	return m.push(screen{kind: kind, namespace: m.namespace})
}

// push opens a screen on top of the one that is there, which is where escape
// comes back to.
func (m Model) push(next screen) (Model, tea.Cmd) {
	m.history = append(m.history, m.screen)
	m.screen = next
	m.err = nil

	entered, cmd := m.enter()

	return entered, tea.Batch(cmd, entered.remember())
}

// back is where escape goes.
func (m Model) back() (Model, tea.Cmd) {
	if len(m.history) == 0 {
		return m, nil
	}

	// What the screen held on to is let go of before leaving it.
	m.closeLogs()

	m.screen = m.history[len(m.history)-1]
	m.history = m.history[:len(m.history)-1]
	m.err = nil

	return m.enter()
}

// enter puts the screen on the table and asks the cluster for its rows.
func (m Model) enter() (Model, tea.Cmd) {
	m.table = newTableModel(m.screen.titles())
	m.filter = ""
	m.sort = newSortState()
	m.layout()

	return m, m.fetch()
}

// visibleAllocs are the allocations the screen was opened for: all of a job,
// or only those of one task group.
func (m Model) visibleAllocs() []nomad.Alloc {
	if m.screen.taskGroup == "" {
		return m.allocs
	}

	kept := make([]nomad.Alloc, 0, len(m.allocs))
	for _, alloc := range m.allocs {
		if alloc.TaskGroup == m.screen.taskGroup {
			kept = append(kept, alloc)
		}
	}

	return kept
}

// tasks are the tasks of the allocation the tasks screen was opened for.
func (m Model) tasks() []nomad.Task {
	for _, alloc := range m.allocs {
		if alloc.ID == m.screen.allocID {
			return alloc.Tasks
		}
	}

	return nil
}

// shortID keeps a UUID readable in a title.
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}

	return id
}
