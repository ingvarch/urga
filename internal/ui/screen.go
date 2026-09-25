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
	screenPlan
	screenFiles
	screenFile
	screenClusters
	screenLogTasks
	screenJobLogs
	screenServiceInstances
	screenVariable

	// screenNode is one client: the allocations on it, under what the
	// machine itself is doing. screenDeployment is one deployment: the
	// allocations it placed, under its groups.
	screenNode
	screenDeployment
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

	// deploymentID is the deployment an allocation list was opened for: it
	// shows the allocations that deployment placed, under its groups.
	deploymentID string

	// label titles a screen that is about one thing, like a description.
	label string

	// taskGroup narrows the allocations to one group of the job.
	taskGroup string

	// task and source are whose output the log screen follows.
	task   string
	source string

	// path is the directory of an allocation the files screen lists.
	path string

	// left is where the screen was when another one was opened on top of
	// it, which is where escape comes back to.
	left place

	// page is the screen, for a screen that is a type of its own.
	page page
}

// place is where a list was left: the row under the cursor, how far down the
// window was, and the filter and the order it was read in. A row number means
// nothing in a list that is read another way.
type place struct {
	cursor int
	top    int
	filter string
	sort   sortState
}

// listsAllocs says the screen is a list of allocations, which answers the
// keys of an allocation whatever it was opened for.
func (s screen) listsAllocs() bool {
	return s.kind == screenAllocations || s.kind == screenNode || s.kind == screenDeployment
}

// titles are the columns of a screen.
func (s screen) titles() []string {
	if s.page != nil {
		return s.page.titles()
	}

	return s.of().titles
}

// topics are what the cluster is asked to say about while the screen is up.
func (s screen) topics() []string {
	if s.page != nil {
		return s.page.topics()
	}

	return s.of().topics
}

// bindings are the keys of the screen, in the order the header shows them,
// whether or not each one does something right now.
func (s screen) bindings() []binding { return s.of().keys }

// bindings are the keys of the open screen, whether or not each does
// something right now. A page's keys press the page.
func (m Model) bindings() []binding {
	p := m.screen.page
	if p == nil {
		return m.screen.bindings()
	}

	hints := p.keys(m.env())

	out := make([]binding, 0, len(hints))
	for _, h := range hints {
		offered := h.offered

		out = append(out, binding{
			press:   h.press,
			label:   h.label,
			writes:  h.writes,
			offered: func(Model) bool { return offered },
			do:      func(m Model) (Model, tea.Cmd) { return m.pressPage(h.press) },
		})
	}

	return out
}

// pressPage hands a key to the open page and takes what it asks for.
func (m Model) pressPage(press string) (Model, tea.Cmd) {
	next, out, ok := m.screen.page.press(press, m.env())
	if !ok {
		return m, nil
	}

	m.screen.page = next

	return m.apply(out)
}

// apply takes what a page asked for: its messages at once, in order, and its
// work meanwhile.
func (m Model) apply(out outcome) (Model, tea.Cmd) {
	cmds := []tea.Cmd{out.cmd}

	for _, msg := range out.now {
		var cmd tea.Cmd

		m, cmd = m.update(msg)
		cmds = append(cmds, cmd)
	}

	m.layout()

	return m, tea.Batch(cmds...)
}

// keys are what the open screen offers now: a key that would do nothing in
// the state the screen is in is not offered, nor, read-only, a key that
// changes the cluster.
func (m Model) keys() []binding {
	all := m.bindings()

	keys := make([]binding, 0, len(all))
	for _, b := range all {
		if m.withheld(b) {
			continue
		}

		if b.offered == nil || b.offered(m) {
			keys = append(keys, b)
		}
	}

	return keys
}

// withheld says read-only takes the key away.
func (m Model) withheld(b binding) bool {
	return m.opts.ReadOnly && b.writes
}

// withheldKey is the key read-only took away from the open screen, when the
// press is one.
func (m Model) withheldKey(press string) (binding, bool) {
	for _, b := range m.bindings() {
		if b.press == press && m.withheld(b) {
			return b, true
		}
	}

	return binding{}, false
}

// hints are the keys the open resource answers. Keys that work everywhere
// are not here, they live in help.
func (m Model) hints() []hint {
	keys := m.keys()

	hints := make([]hint, 0, len(keys))
	for _, b := range keys {
		hints = append(hints, b.hint())
	}

	return hints
}

// binding is what a key does on the screen, when the screen offers it.
func (m Model) binding(press string) (binding, bool) {
	for _, b := range m.keys() {
		if b.press == press {
			return b, true
		}
	}

	return binding{}, false
}

// title labels the box with what it holds and how much of it. The count is
// what is on the screen, which the filter narrows.
func (m Model) title() string {
	count := len(m.list.table.rows)
	if p := m.screen.page; p != nil {
		return p.title(m.env(), count)
	}

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
	if p := m.screen.page; p != nil {
		return p.rows(m.env())
	}

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
	if p := m.screen.page; p != nil {
		return askedFor(m.asked, p.fetch(m.env()))
	}

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
	return labelled(cmd, func(msg tea.Msg) tea.Msg { return answerMsg{asked: asked, msg: msg} })
}

// labelled wraps whatever a command answers, the commands of a batch
// included.
func labelled(cmd tea.Cmd, label func(tea.Msg) tea.Msg) tea.Cmd {
	if cmd == nil {
		return nil
	}

	return func() tea.Msg {
		msg := cmd()

		if batch, ok := msg.(tea.BatchMsg); ok {
			out := make(tea.BatchMsg, 0, len(batch))
			for _, c := range batch {
				out = append(out, labelled(c, label))
			}

			return out
		}

		if msg == nil {
			return nil
		}

		return label(msg)
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
	return m.list.selected()
}

// show switches to a resource, which is what the command prompt does. Either
// way the session writes down where it is looking, so the next run comes back
// to the same place.
func (m Model) show(kind screenKind) (Model, tea.Cmd) {
	if kind != m.screen.kind {
		return m.push(m.screenOf(kind))
	}

	return m.arrive()
}

// screenOf is a screen opened by name, in the namespace of the session, with
// nothing read into it yet.
func (m Model) screenOf(kind screenKind) screen {
	s := screen{kind: kind, namespace: m.namespace}
	if open := resources[kind].open; open != nil {
		s.page = open()
	}

	return s
}

// push opens a list on top of the one that is there, which is where escape
// comes back to.
func (m Model) push(next screen) (Model, tea.Cmd) {
	return m.stack(next).arrive()
}

// stack puts a screen on top of the one that is there, which keeps where it
// was left for escape to come back to. What belonged to it, its filter and
// the order of its rows, says nothing about the screen that is opening.
func (m Model) stack(next screen) Model {
	m.screen.left = place{cursor: m.list.table.cursor, top: m.list.table.top, filter: m.list.filter, sort: m.list.sort}
	m.history = append(m.history, m.screen)
	m.screen = next
	m = m.forget()
	m.list.filter = ""
	m.list.sort = newSortState()
	m.list.marks = nil

	return m
}

// back is where escape goes.
func (m Model) back() (Model, tea.Cmd) {
	if len(m.history) == 0 {
		return m, nil
	}

	// What the screen held on to is let go of before leaving it.
	m = m.stopLogs()

	m.screen = m.history[len(m.history)-1]
	m.history = m.history[:len(m.history)-1]
	m = m.forget()
	m.list.marks = nil

	arrived, cmd := m.arrive()

	var read tea.Cmd

	switch arrived.screen.kind {
	case screenLogs:
		arrived, read = arrived.readAgain()
	case screenFile:
		arrived, read = arrived.readFileAgain()
	case screenJobLogs:
		arrived, read = reloadJobLogs(arrived)
	}

	cmd = tea.Batch(cmd, read)

	return arrived.returnTo(arrived.screen.left), cmd
}

// returnTo puts a list back the way it was left. The rows it held are still
// there, so the cursor lands on the same one; the answer that follows keeps
// it there the way any refresh does.
func (m Model) returnTo(left place) Model {
	m.list.filter, m.list.sort = left.filter, left.sort
	m.layout()

	m.list.table.cursor, m.list.table.top = left.cursor, left.top
	m.list.table.follow()

	return m
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
	m.answered = false

	// So were the readings, and their chain ends with that ask.
	m.usage = m.usage.forgetRows()
	m.host.due = false
	m.checks.due = false
	m.instanceChecks.due = false

	m.list.table = newTableModel(m.screen.titles())
	m.list.filter = ""
	m.list.sort = newSortState()
	m.layout()

	// A screen that can be opened by name shows the namespace of the
	// session; one that was opened from another screen keeps the namespace
	// it was opened for. The rows and the stream have to agree on which.
	if m.screen.of().stored != "" {
		m.screen.namespace = m.namespace
	}

	// What the screen that was left was watching is let go of: the new one
	// watches what it shows, if the cluster will say.
	m.watch = m.watch.end()

	return m, tea.Batch(m.fetch(), m.watchScreen())
}

// stackText opens a screen that reads as text rather than as a list.
func (m Model) stackText(next screen, text textModel) Model {
	m = m.stack(next)
	m.text = text
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
	if alloc, ok := m.shownAlloc(); ok {
		return alloc.Tasks
	}

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
