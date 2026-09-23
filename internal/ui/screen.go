package ui

import (
	"context"

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
	screenServers
	screenServer
	screenNodeEvents
	screenNodeDrivers
	screenNodeDriver
	screenNodeVolumes
	screenNodeAttributes
	screenNodeMeta
	screenJobVersions
	screenTaskEvents
	screenDescribe
	screenLogs
	screenTaskGroups
	screenRegions
	screenDatacenters
)

// screen is what is open: the resource and what it was opened for. The
// namespace travels with it, a job of another namespace keeps its own.
type screen struct {
	kind      screenKind
	namespace string

	jobID   string
	allocID string

	// nodeID is the machine an allocation list was opened for: a client
	// shows its own work rather than the work of a job.
	nodeID string

	// label titles a screen that is about one thing, like a description.
	label string

	// taskGroup narrows the allocations to one group of the job.
	taskGroup string

	// task and source are whose output the log screen follows.
	task   string
	source string
}

// isClient says the screen was opened for one machine of the cluster, which
// answers keys of its own.
func (s screen) isClient() bool {
	return s.kind == screenAllocations && s.nodeID != ""
}

// titles are the columns of a screen.
func (s screen) titles() []string {
	return s.of().titles
}

// hints are the keys the open resource answers. Keys that work everywhere
// are not here, they live in help.
func (s screen) hints() []hint {
	res := s.of()
	if res.hintsFor != nil {
		return res.hintsFor(s)
	}

	return res.hints
}

// title labels the box with what it holds and how much of it. The count is
// what is on the screen, which the filter narrows.
func (m Model) title() string {
	count := len(m.table.rows)
	res := m.screen.of()

	if res.title != nil {
		return res.title(m, count)
	}

	if res.cluster {
		return sprintf("%s [%d]", res.name, count)
	}

	return sprintf("%s (%s) [%d]", res.name, namespaceLabel(m.namespace), count)
}

// rows are what the open screen shows.
func (m Model) rows() []tableRow {
	res := m.screen.of()
	if res.rows == nil {
		return nil
	}

	return res.rows(m)
}

// fetch asks the cluster for what the open screen shows. A screen whose
// content arrives another way, like a description or a log stream, asks for
// nothing.
func (m Model) fetch() tea.Cmd {
	res := m.screen.of()
	if res.fetch == nil {
		return nil
	}

	return askedFor(m.asked, res.fetch(m))
}

// answerMsg is an answer with the ask it belongs to. The session moves on
// while a request is out; what was asked before that is no longer what the
// screen shows.
type answerMsg struct {
	asked int
	msg   tea.Msg
}

// askedFor labels whatever a command answers with, the commands of a batch
// included.
func askedFor(asked int, cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}

	return func() tea.Msg {
		msg := cmd()

		if batch, ok := msg.(tea.BatchMsg); ok {
			labelled := make(tea.BatchMsg, 0, len(batch))
			for _, c := range batch {
				labelled = append(labelled, askedFor(asked, c))
			}

			return labelled
		}

		if msg == nil {
			return nil
		}

		return answerMsg{asked: asked, msg: msg}
	}
}

// request asks the cluster in the background and hands the answer over as a
// message. Nothing here touches the model: every answer arrives through
// Update like any other message.
func request[T any](load func(ctx context.Context) (T, error), wrap func(T) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		answer, err := load(ctx)
		if err != nil {
			return errMsg{err: err}
		}

		return wrap(answer)
	}
}

// fetchList is a request for a list of resources.
func fetchList[T any](load func(ctx context.Context) ([]T, error), wrap func([]T) tea.Msg) tea.Cmd {
	return request(load, wrap)
}

// open drills into what the cursor is on, if there is anything below it.
func (m Model) open() (Model, tea.Cmd) {
	switch m.screen.kind {
	case screenJobs:
		job, ok := selectedOf(m, screenJobs, m.jobs)
		if !ok {
			return m, nil
		}

		return m.push(screen{kind: screenAllocations, namespace: job.Namespace, jobID: job.ID})

	case screenAllocations:
		alloc, ok := selectedOf(m, screenAllocations, m.visibleAllocs())
		if !ok {
			return m, nil
		}

		return m.push(screen{
			kind:      screenTasks,
			namespace: alloc.Namespace,
			jobID:     alloc.JobID,
			allocID:   alloc.ID,
		})

	case screenNodes:
		node, ok := selectedOf(m, screenNodes, m.nodes)
		if !ok {
			return m, nil
		}

		// The readings belong to the machine they were taken on, the chart
		// starts over on every client.
		m.host = hostModel{node: node}

		return m.push(screen{
			kind:      screenAllocations,
			namespace: nomad.AllNamespaces,
			nodeID:    node.ID,
			label:     node.Name,
		})

	case screenServers:
		server, ok := selectedOf(m, screenServers, m.servers)
		if !ok {
			return m, nil
		}

		m.server, m.raft, m.raftErr = server, nil, nil

		return m.push(screen{kind: screenServer, label: server.Name})

	case screenNodeDrivers:
		return m.openDriver()

	case screenRegions:
		return m.chooseRegion()

	case screenDatacenters:
		return m.chooseDatacenter()

	case screenJobVersions:
		return m.openVersionDiff()

	case screenTasks:
		return m.openLogs(nomad.LogStdout)

	case screenTaskGroups:
		group, ok := selectedOf(m, screenTaskGroups, m.groups)
		if !ok {
			return m, nil
		}

		return m.push(screen{
			kind:      screenAllocations,
			namespace: m.screen.namespace,
			jobID:     group.JobID,
			taskGroup: group.Name,
		})
	}

	return m, nil
}

// selectedOf is the resource the cursor is on, when the screen showing it is
// the one that is open. Every key that acts on a row asks through here: the
// bounds are checked in one place instead of at a dozen call sites.
func selectedOf[T any](m Model, kind screenKind, items []T) (T, bool) {
	var none T

	if m.screen.kind != kind {
		return none, false
	}

	row, ok := m.selectedIndex()
	if !ok || row >= len(items) {
		return none, false
	}

	return items[row], true
}

// selectedIndex is the resource the cursor is on. The filter shifts the rows,
// so the row number is not the number of the resource.
func (m Model) selectedIndex() (int, bool) {
	if m.table.cursor < 0 || m.table.cursor >= len(m.index) {
		return 0, false
	}

	return m.index[m.table.cursor], true
}

// show switches to a resource, which is what the command prompt does. Either
// way the session writes down where it is looking, so the next run comes back
// to the same place.
func (m Model) show(kind screenKind) (Model, tea.Cmd) {
	if kind != m.screen.kind {
		return m.push(screen{kind: kind, namespace: m.namespace})
	}

	return m.arrive()
}

// push opens a list on top of the one that is there, which is where escape
// comes back to.
func (m Model) push(next screen) (Model, tea.Cmd) {
	return m.stack(next).arrive()
}

// stack puts a screen on top of the one that is there and leaves behind what
// belonged to it: its filter and the order of its rows say nothing about the
// screen that is opening.
func (m Model) stack(next screen) Model {
	m.history = append(m.history, m.screen)
	m.screen = next
	m = m.forget()
	m.filter = ""
	m.sort = newSortState()
	m.marks = nil

	return m
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
	m = m.forget()
	m.marks = nil

	return m.arrive()
}

// arrive puts the screen up and writes the session down. Every way of
// arriving at a screen goes through here, so that where the session is
// looking is never one screen behind what is on display.
func (m Model) arrive() (Model, tea.Cmd) {
	entered, cmd := m.enter()

	return entered, tea.Batch(cmd, entered.remember())
}

// enter puts the screen on the table and asks the cluster for its rows.
func (m Model) enter() (Model, tea.Cmd) {
	// Whatever is still out was asked for what was on the screen before.
	m.asked++

	// So were the readings, and their chain ends with that ask.
	m.usage = m.usage.forgetRows()

	m.table = newTableModel(m.screen.titles())
	m.filter = ""
	m.sort = newSortState()
	m.layout()

	// A screen that can be opened by name shows the namespace of the
	// session; one that was opened from another screen keeps the namespace
	// it was opened for. The rows and the stream have to agree on which.
	if m.screen.of().stored != "" {
		m.screen.namespace = m.namespace
	}

	// What the screen that was left was watching is let go of: the new one
	// watches what it shows, if the cluster will say.
	m = m.endWatch()

	return m, tea.Batch(m.fetch(), m.watch())
}

// stackText opens a screen that reads as text rather than as a list.
func (m Model) stackText(next screen, content string) Model {
	m = m.stack(next)
	m.text = newTextModel(content)
	m.layout()

	return m
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
