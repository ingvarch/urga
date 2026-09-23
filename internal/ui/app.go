// Package ui is the terminal interface: one root model that owns the screen
// and the keyboard.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/config"
	"github.com/ingvarch/urga/internal/nomad"
)

// Client is what the screens need from the cluster. Every call names the
// namespace it asks in.
type Client interface {
	Address() string
	Version(ctx context.Context) (string, error)
	Jobs(ctx context.Context, namespace string) ([]nomad.Job, error)
	DescribeJob(ctx context.Context, namespace, jobID string) (string, error)
	DescribeAllocation(ctx context.Context, namespace, allocID string) (string, error)
	DescribeDeployment(ctx context.Context, namespace, deploymentID string) (string, error)
	DescribeService(ctx context.Context, namespace, name string) (string, error)
	JobSpec(ctx context.Context, namespace, jobID string) (string, error)
	Logs(ctx context.Context, namespace, allocID, task, source string) (*nomad.LogStream, error)
	Usage(ctx context.Context) (nomad.Usage, error)
	AllocationUsage(ctx context.Context, namespace, allocID string) (nomad.ResourceUse, error)
	NodeUsage(ctx context.Context, nodeID string) (nomad.ResourceUse, error)

	TaskGroups(ctx context.Context, namespace, jobID string) ([]nomad.TaskGroup, error)

	SubmitJob(ctx context.Context, namespace, source string) error
	NamespaceSpec(ctx context.Context, name string) (string, error)
	SubmitNamespace(ctx context.Context, source string) error

	StartJob(ctx context.Context, namespace, jobID string) error
	StopJob(ctx context.Context, namespace, jobID string) error
	RevertJob(ctx context.Context, namespace, jobID string) error
	ScaleJob(ctx context.Context, namespace, jobID, group string, count int) error
	RestartAllocation(ctx context.Context, namespace, allocID string) error
	StopAllocation(ctx context.Context, namespace, allocID string) error
	DrainNode(ctx context.Context, nodeID string, drain bool) error
	SetNodeEligible(ctx context.Context, nodeID string, eligible bool) error
	PromoteDeployment(ctx context.Context, namespace, deploymentID string) error
	FailDeployment(ctx context.Context, namespace, deploymentID string) error
	Allocations(ctx context.Context, namespace, jobID string) ([]nomad.Alloc, error)
	NodeAllocations(ctx context.Context, nodeID string) ([]nomad.Alloc, error)
	NodeDetail(ctx context.Context, nodeID string) (nomad.NodeDetail, error)
	NodeMeta(ctx context.Context, nodeID string) ([]nomad.MetaEntry, error)
	NodeMetaSpec(ctx context.Context, nodeID string) (string, error)
	SubmitNodeMeta(ctx context.Context, nodeID, source string) error
	Deployments(ctx context.Context, namespace string) ([]nomad.Deployment, error)
	Namespaces(ctx context.Context) ([]nomad.Namespace, error)
	Services(ctx context.Context, namespace string) ([]nomad.Service, error)
	Evaluations(ctx context.Context, namespace string) ([]nomad.Evaluation, error)
	Nodes(ctx context.Context) ([]nomad.Node, error)
	Variables(ctx context.Context, namespace string) ([]nomad.Variable, error)
	NodePools(ctx context.Context) ([]nomad.NodePool, error)
	Servers(ctx context.Context) ([]nomad.Server, error)
	Server(ctx context.Context, name string) (nomad.Server, error)
	RaftPeers(ctx context.Context) ([]nomad.RaftPeer, error)
}

// Options are what the session starts with.
type Options struct {
	// Namespace the session looks at. Empty is every namespace.
	Namespace string

	// Version of urga, for the header.
	Version string

	// PollEvery is the wait between an answer and the next ask.
	PollEvery time.Duration

	// Config is what the last session left behind. It may be nil.
	Config *config.Config

	// Editor hands a resource to the editor of the user. It may be nil,
	// and editing then says so.
	Editor Editor

	// Shell runs a shell inside a task. It may be nil.
	Shell Shell
}

const (
	// usageEvery is how often the header reads the load of the cluster. It
	// walks every node and allocation, so it is not asked for often.
	usageEvery = 15 * time.Second

	defaultPollEvery = 2 * time.Second
	defaultTimeout   = 10 * time.Second

	statusHeight = 1

	// The screen keeps its distance from the edges of the terminal: a line
	// of air on top, the box one column in, the text of the header and the
	// status line one further.
	screenPadTop = 1
	screenPadX   = 1
	headerPadX   = 2
)

// Messages. Every answer from the cluster arrives as one of these, the model
// changes nowhere else.
type (
	jobsMsg        []nomad.Job
	allocsMsg      []nomad.Alloc
	taskGroupsMsg  []nomad.TaskGroup
	deploymentsMsg []nomad.Deployment
	namespacesMsg  []nomad.Namespace
	servicesMsg    []nomad.Service
	evaluationsMsg []nomad.Evaluation
	nodesMsg       []nomad.Node
	variablesMsg   []nomad.Variable
	nodePoolsMsg   []nomad.NodePool
	serversMsg     []nomad.Server
	serverMsg      nomad.Server
	nodeDetailMsg  nomad.NodeDetail
	nodeMetaMsg    []nomad.MetaEntry

	// raftMsg is what the raft of the cluster says about its servers. An
	// ACL may hold it back, and then the reason is shown where the answer
	// would have been.
	raftMsg struct {
		peers []nomad.RaftPeer
		err   error
	}

	usageMsg     nomad.Usage
	versionMsg   string
	errMsg       struct{ err error }
	pollMsg      struct{}
	pollUsageMsg struct{}
)

// Model is the whole interface. It owns the screen, the keyboard and what the
// cluster last said.
type Model struct {
	client Client
	opts   Options

	namespace string

	width  int
	height int

	// screen is what the body shows, history is where escape goes back to.
	screen  screen
	history []screen

	// overlay is who holds the keyboard: the screen, the command line or
	// the help window.
	overlay overlay
	prompt  promptModel
	confirm confirmModel
	filter  string

	// said is what came of the last action.
	said string

	// editing is the file that is open in the editor.
	editing editFileMsg

	// index maps a row of the table back to the resource it came from, which
	// the filter and the sort order shift.
	index []int

	// sort is the column the list is ordered by.
	sort sortState

	// troubled leaves only what the cluster is not happy with.
	troubled bool

	// shown and held are how many rows are on the screen out of how many
	// the cluster answered with.
	shown int
	held  int

	// namespaceOrder is which namespace each number key stands for.
	namespaceOrder []string

	jobs        []nomad.Job
	allocs      []nomad.Alloc
	groups      []nomad.TaskGroup
	deployments []nomad.Deployment
	namespaces  []nomad.Namespace
	services    []nomad.Service
	evaluations []nomad.Evaluation
	nodes       []nomad.Node
	variables   []nomad.Variable
	nodePools   []nomad.NodePool
	servers     []nomad.Server

	// host is the machine a client screen is open on, hostTrail the
	// readings taken of it since it was opened.
	host      nomad.Node
	hostTrail []nomad.ResourceUse

	// nodeDetail is what the machine of a client screen says about itself,
	// nodeMeta the metadata it carries.
	nodeDetail nomad.NodeDetail
	nodeMeta   []nomad.MetaEntry

	// server is the one a server screen is open on, raft what the raft of
	// the cluster makes of the servers.
	server  nomad.Server
	raft    []nomad.RaftPeer
	raftErr error

	table tableModel
	text  textModel

	// stream is the task output the log screen follows.
	stream    *nomad.LogStream
	following bool

	nomadVersion string
	usage        nomad.Usage

	// rowUsage is what each row on the screen takes, by its id, and why the
	// rest of them said nothing.
	rowUsage     map[string]nomad.ResourceUse
	missingUsage int
	usageReason  error
	err          error
}

// New builds the model. Nothing is asked of the cluster until Init runs.
func New(client Client, opts Options) Model {
	if opts.PollEvery == 0 {
		opts.PollEvery = defaultPollEvery
	}

	m := Model{
		client:    client,
		opts:      opts,
		namespace: opts.Namespace,
		screen:    screen{kind: screenJobs, namespace: opts.Namespace},
		table:     newTableModel(jobTitles),
		sort:      newSortState(),
	}

	return m.restore()
}

// Init asks the cluster for what the first screen shows, and for what the
// session needs whatever is open.
func (m Model) Init() tea.Cmd {
	client := m.client

	return tea.Batch(
		m.fetch(),
		fetchVersion(client),
		fetchClusterUsage(client),
		fetchList(client.Namespaces, func(items []nomad.Namespace) tea.Msg { return namespacesMsg(items) }),
	)
}

// Update is the entry for every message. It keeps the concrete model, the
// interface method wraps it.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)

	return next, cmd
}

func (m Model) update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()

		return m, nil

	case tea.KeyPressMsg:
		m.said = ""

		return m.handleKey(msg)

	case jobsMsg:
		return m.applyList(screenJobs, func(m *Model) { m.jobs = msg })

	case allocsMsg:
		next, cmd := m.applyList(screenAllocations, func(m *Model) { m.allocs = msg })

		return next, tea.Batch(cmd, next.usageOnce())

	case taskGroupsMsg:
		return m.applyList(screenTaskGroups, func(m *Model) { m.groups = msg })

	case deploymentsMsg:
		return m.applyList(screenDeployments, func(m *Model) { m.deployments = msg })

	case namespacesMsg:
		// The namespaces are kept whatever is on the screen: the command
		// line and the number keys need the list to switch between them.
		m.namespaces = msg
		m.rememberNamespaces(msg)

		next, cmd := m.applyList(screenNamespaces, func(*Model) {})

		return next, tea.Batch(cmd, next.remember())

	case servicesMsg:
		return m.applyList(screenServices, func(m *Model) { m.services = msg })

	case evaluationsMsg:
		return m.applyList(screenEvaluations, func(m *Model) { m.evaluations = msg })

	case nodesMsg:
		next, cmd := m.applyList(screenNodes, func(m *Model) { m.nodes = msg })

		return next, tea.Batch(cmd, next.usageOnce())

	case variablesMsg:
		return m.applyList(screenVariables, func(m *Model) { m.variables = msg })

	case nodePoolsMsg:
		return m.applyList(screenNodePools, func(m *Model) { m.nodePools = msg })

	case serversMsg:
		return m.applyList(screenServers, func(m *Model) { m.servers = msg })

	case versionMsg:
		m.nomadVersion = string(msg)

		return m, nil

	case usageMsg:
		m.usage = nomad.Usage(msg)

		return m, tea.Tick(usageEvery, func(time.Time) tea.Msg { return pollUsageMsg{} })

	case pollUsageMsg:
		return m, fetchClusterUsage(m.client)

	case rowUsageMsg:
		m.rowUsage = msg.readings
		m.missingUsage, m.usageReason = msg.missing, msg.reason
		m.layout()

		return m, tea.Tick(rowUsageEvery, func(time.Time) tea.Msg { return pollUsageRow{} })

	case pollUsageRow:
		return m, m.fetchUsage()

	case errMsg:
		// The rows that are on the screen stay there. An empty table reads as
		// an empty cluster.
		m.err = msg.err

		return m, m.schedulePoll()

	case describeMsg:
		return m.showDescribe(msg)

	case editFileMsg:
		return m.startEdit(msg)

	case editedMsg:
		return m.finishEdit(msg)

	case shellDoneMsg:
		if msg.err != nil {
			m.err = msg.err

			return m, nil
		}

		m.said = fmt.Sprintf("Shell in %s closed.", msg.task)

		return m, nil

	case logStreamMsg:
		m.stream = msg.stream

		return m, m.waitForLog()

	case logLineMsg:
		if m.screen.kind != screenLogs {
			return m, nil
		}

		return m.appendLog(string(msg))

	case logEndMsg:
		m.stream = nil

		return m, nil

	case doneMsg:
		if msg.err != nil {
			m.err = msg.err

			return m, nil
		}

		m.err = nil
		m.said = msg.said

		// The list is stale the moment the cluster changed, ask again.
		return m, m.fetch()

	case nodeDetailMsg:
		// The answer belongs to the machine it was asked of: leaving one
		// client for another must not show the first one under the second.
		if m.screen.nodeID != msg.ID {
			return m, nil
		}

		m.nodeDetail = nomad.NodeDetail(msg)
		m.err = nil
		m.layout()

		return m, m.schedulePoll()

	case nodeMetaMsg:
		return m.applyList(screenNodeMeta, func(m *Model) { m.nodeMeta = msg })

	case serverMsg:
		if m.screen.kind != screenServer {
			return m, nil
		}

		m.server = nomad.Server(msg)
		m.err = nil
		m.layout()

		return m, m.schedulePoll()

	case raftMsg:
		m.raft, m.raftErr = msg.peers, msg.err
		m.layout()

		return m, nil

	case hostUseMsg:
		return m.keepHostUse(msg), nil

	case pollMsg:
		return m, m.fetch()
	}

	return m, nil
}

// applyList stores what the cluster sent, unless the screen it belongs to
// was left: it would show up under the wrong title.
func (m Model) applyList(kind screenKind, store func(*Model)) (Model, tea.Cmd) {
	if m.screen.kind != kind {
		return m, nil
	}

	store(&m)
	m.err = nil
	m.layout()

	return m, m.schedulePoll()
}

// handleKey is the one place that decides who gets a key press: an overlay
// first, then the way around the screen, then what the resource can do.
func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.overlay != overlayNone {
		return m.overlayKey(msg)
	}

	if m.screen.kind == screenLogs {
		if next, cmd, handled := m.logsKey(msg); handled {
			return next, cmd
		}
	}

	if next, handled := m.scrollKey(msg); handled {
		return next, nil
	}

	if next, cmd, handled := m.resourceKey(msg); handled {
		return next, cmd
	}

	return m.sessionKey(msg)
}

// overlayKey hands the key to whatever took the keyboard.
func (m Model) overlayKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch m.overlay {
	case overlayPrompt, overlayFilter, overlayScale:
		return m.promptKey(msg)

	case overlayHelp:
		return m.helpKey(msg)

	case overlayConfirm:
		return m.confirmKey(msg)
	}

	return m, nil
}

// resourceKey is what the open resource can do. Each of these looks at the
// row under the cursor and does nothing when the screen is not its own.
func (m Model) resourceKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	var (
		next Model
		cmd  tea.Cmd
	)

	switch msg.String() {
	case "enter":
		next, cmd = m.open()

	case "ctrl+s":
		next, cmd = m.startStopJob()

	case "e":
		// The same key opens the events of a client and edits what can be
		// edited, because a screen never offers both.
		if m.screen.isClient() {
			next, cmd = m.openNodeScreen(screenNodeEvents)
		} else {
			next, cmd = m.edit()
		}

	case "t":
		next, cmd = m.openTaskGroups()

	case "s":
		// The same key scales a task group and opens a shell in a task,
		// because a screen never offers both.
		if m.screen.kind == screenTasks {
			next, cmd = m.shell()
		} else {
			next, cmd = m.scaleGroup()
		}

	case "u":
		next, cmd = m.revertJob()

	case "r":
		next, cmd = m.restartAllocation()

	case "ctrl+k":
		next, cmd = m.stopAllocation()

	case "ctrl+d":
		// Draining is a key of the list of clients; on the screen of one
		// client the same key opens what it can run.
		if m.screen.isClient() {
			next, cmd = m.openNodeScreen(screenNodeDrivers)
		} else {
			next, cmd = m.drainNode()
		}

	case "ctrl+h":
		next, cmd = m.openNodeScreen(screenNodeVolumes)

	case "a":
		next, cmd = m.openNodeScreen(screenNodeAttributes)

	case "m":
		next, cmd = m.openNodeScreen(screenNodeMeta)

	case "i":
		next, cmd = m.toggleEligibility()

	case "p":
		next, cmd = m.promoteDeployment()

	case "f":
		next, cmd = m.failDeployment()

	case "d":
		next, cmd = m, m.describeCmd()

	case "h":
		next, cmd = m, m.jobSpecCmd()

	case "c":
		next, cmd = m.copyField()

	case "ctrl+e":
		next, cmd = m.openLogs(nomad.LogStderr)

	default:
		return m, nil, false
	}

	return next, cmd, true
}

// sessionKey is what works on every screen.
func (m Model) sessionKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit

	case ":":
		return m.openPrompt(promptPrefix)

	case "/":
		return m.openPrompt(filterPrefix)

	case "?":
		m.overlay = overlayHelp

		return m, nil

	case "!":
		m.troubled = !m.troubled
		m.layout()

		return m, nil

	case "esc":
		return m.back()

	case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
		return m.namespaceKey(int(msg.Code - '0'))

	default:
		// A capital letter names a column to order the list by.
		return m.sortKey(msg)
	}
}

// sortKey orders the list by the column a letter names, the way one key
// picks a column.
func (m Model) sortKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if len(msg.Text) != 1 {
		return m, nil
	}

	letter := []rune(msg.Text)[0]
	if !unicode.IsUpper(letter) {
		return m, nil
	}

	column, ok := columnOfLetter(m.screen.titles(), letter)
	if !ok {
		return m, nil
	}

	m.sort = m.sort.by(column)
	m.layout()

	return m, nil
}

// helpKey closes the help window, which is all it answers.
func (m Model) helpKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter", "?", "q":
		m.overlay = overlayNone
	}

	return m, nil
}

// View draws the screen. urga runs in the alternate screen, the terminal
// comes back as it was.
func (m Model) View() tea.View {
	view := tea.NewView(m.render())
	view.AltScreen = true

	return view
}

func (m Model) render() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	head := renderHeader(m.headerData(), m.width-2*headerPadX)

	parts := []string{strings.Repeat("\n", screenPadTop) + indent(head, headerPadX)}

	width := m.width - 2*screenPadX

	if m.overlay.asksForALine() {
		parts = append(parts, indent(m.prompt.view(width), screenPadX))
	}

	title, body := m.body(width - 2)

	framed := frame(title, body, width, m.bodyHeight())

	// A question floats over the screen, next to the row it was asked about.
	if m.overlay == overlayConfirm {
		framed = modal(framed, m.confirm.view(width-2*modalPadX), m.cursorLine())
	}

	parts = append(parts,
		indent(framed, screenPadX),
		indent(m.status(), headerPadX),
	)

	return strings.Join(parts, "\n")
}

// headerData is what the top of the screen says about the session.
func (m Model) headerData() header {
	return header{
		address:      m.client.Address(),
		version:      m.opts.Version,
		nomadVersion: m.nomadVersion,
		usage:        percentOf(m.usage.CPUPercent),
		memory:       percentOf(m.usage.MemoryPercent),
		namespaces:   m.namespaceColumnData(),
		hints:        m.screen.hints(),
	}
}

// cursorLine is where the row under the cursor is drawn inside the box: the
// top border and the header of the table come before it. A screen that reads
// as text has no such row.
func (m Model) cursorLine() int {
	if m.readsAsText() {
		return -1
	}

	return 2 + m.panelHeight() + m.table.cursor - m.table.top
}

// body is what fills the box: what took the screen, or what the screen
// shows.
func (m Model) body(width int) (title, content string) {
	switch {
	case m.overlay == overlayHelp:
		return "Help", renderHelp(m.helpSections(), width)

	case m.readsAsText():
		return m.title(), m.text.view()
	}

	if panel := m.panelHeight(); panel > 0 {
		return m.title(), strings.Join(append(m.hostPanel(width), m.table.view()), "\n")
	}

	return m.title(), m.table.view()
}

func (m Model) status() string {
	width := m.width - 2*headerPadX

	if m.err != nil {
		return styleError.Render(truncate("! "+m.err.Error(), width))
	}

	if m.said != "" {
		return styleValue.Render(truncate(m.said, width))
	}

	if m.troubled {
		return styleWarn.Render(truncate(
			fmt.Sprintf("only what needs attention, %d of %d   <!> all of them", m.shown, m.held), width))
	}

	if m.missingUsage > 0 && m.usageReason != nil {
		return styleMuted.Render(truncate(
			fmt.Sprintf("no readings for %d rows: %s", m.missingUsage, m.usageReason), width))
	}

	return styleMuted.Render(truncate("<:> command   </> filter   <?> help   <q> quit", width))
}

func (m Model) bodyHeight() int {
	height := m.height - screenPadTop - headerHeight - statusHeight

	if m.overlay.asksForALine() {
		height -= promptHeight
	}

	return max(height, 2)
}

// layout sizes the table to the window and fills it with what the cluster
// last said.
func (m *Model) layout() {
	if m.width == 0 {
		return
	}

	// The table sits inside the box: the margin, its two border lines and the
	// header row of the table itself are not rows.
	m.table.setSize(m.width-2*screenPadX-2, max(m.bodyHeight()-3-m.panelHeight(), 1))
	m.text.setSize(m.width-2*screenPadX-2, max(m.bodyHeight()-2, 1))

	m.text.filter = m.filter
	m.text.follow()

	all := m.rows()
	m.held = len(all)

	rows, index := filterRows(all, m.filter)

	if m.troubled {
		rows, index = troubledRows(rows, index)
	}

	rows, index = sortRows(rows, index, m.sort, m.screen.titles())
	m.shown = len(rows)

	m.index = index
	m.table.show(rows, m.sort)
}

func (m Model) schedulePoll() tea.Cmd {
	return tea.Tick(m.opts.PollEvery, func(time.Time) tea.Msg { return pollMsg{} })
}

// percentOf is a reading of the cluster, empty until there is one.
func percentOf(value int) string {
	if value == 0 {
		return ""
	}

	return fmt.Sprintf("%d%%", value)
}

// fetchClusterUsage reads what the whole cluster is busy with, which the
// header shows. What one row takes is read by Model.fetchUsage.
func fetchClusterUsage(client Client) tea.Cmd {
	return request(client.Usage, func(usage nomad.Usage) tea.Msg { return usageMsg(usage) })
}

func fetchVersion(client Client) tea.Cmd {
	return request(client.Version, func(version string) tea.Msg { return versionMsg(version) })
}
