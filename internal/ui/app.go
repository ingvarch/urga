// Package ui is the terminal interface: one root model that owns the screen
// and the keyboard.
package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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
	Allocations(ctx context.Context, namespace, jobID string) ([]nomad.Alloc, error)
	Deployments(ctx context.Context, namespace string) ([]nomad.Deployment, error)
	Namespaces(ctx context.Context) ([]nomad.Namespace, error)
	Services(ctx context.Context, namespace string) ([]nomad.Service, error)
	Evaluations(ctx context.Context, namespace string) ([]nomad.Evaluation, error)
	Nodes(ctx context.Context) ([]nomad.Node, error)
	Variables(ctx context.Context, namespace string) ([]nomad.Variable, error)
	NodePools(ctx context.Context) ([]nomad.NodePool, error)
}

// Options are what the session starts with.
type Options struct {
	// Namespace the session looks at. Empty is every namespace.
	Namespace string

	// Version of urga, for the header.
	Version string

	// PollEvery is the wait between an answer and the next ask.
	PollEvery time.Duration

	// Timeout is how long one request may take.
	Timeout time.Duration
}

const (
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
	deploymentsMsg []nomad.Deployment
	namespacesMsg  []nomad.Namespace
	servicesMsg    []nomad.Service
	evaluationsMsg []nomad.Evaluation
	nodesMsg       []nomad.Node
	variablesMsg   []nomad.Variable
	nodePoolsMsg   []nomad.NodePool

	versionMsg string
	errMsg     struct{ err error }
	pollMsg    struct{}
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
	filter  string

	// index maps a row of the table back to the resource it came from, which
	// the filter shifts.
	index []int

	// namespaceOrder is which namespace each number key stands for.
	namespaceOrder []string

	jobs        []nomad.Job
	allocs      []nomad.Alloc
	deployments []nomad.Deployment
	namespaces  []nomad.Namespace
	services    []nomad.Service
	evaluations []nomad.Evaluation
	nodes       []nomad.Node
	variables   []nomad.Variable
	nodePools   []nomad.NodePool

	table tableModel
	text  textModel

	nomadVersion string
	err          error
}

// New builds the model. Nothing is asked of the cluster until Init runs.
func New(client Client, opts Options) Model {
	if opts.PollEvery == 0 {
		opts.PollEvery = defaultPollEvery
	}

	if opts.Timeout == 0 {
		opts.Timeout = defaultTimeout
	}

	return Model{
		client:    client,
		opts:      opts,
		namespace: opts.Namespace,
		screen:    screen{kind: screenJobs, namespace: opts.Namespace},
		table:     newTableModel(jobTitles),
	}
}

// Init asks the cluster for what the first screen shows, and for what the
// session needs whatever is open.
func (m Model) Init() tea.Cmd {
	client := m.client

	return tea.Batch(
		m.fetch(),
		fetchVersion(client),
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
		return m.handleKey(msg)

	case jobsMsg:
		return m.applyList(screenJobs, func(m *Model) { m.jobs = msg })

	case allocsMsg:
		return m.applyList(screenAllocations, func(m *Model) { m.allocs = msg })

	case deploymentsMsg:
		return m.applyList(screenDeployments, func(m *Model) { m.deployments = msg })

	case namespacesMsg:
		// The namespaces are kept whatever is on the screen: the command
		// line and the number keys need the list to switch between them.
		m.namespaces = msg
		m.rememberNamespaces(msg)

		return m.applyList(screenNamespaces, func(*Model) {})

	case servicesMsg:
		return m.applyList(screenServices, func(m *Model) { m.services = msg })

	case evaluationsMsg:
		return m.applyList(screenEvaluations, func(m *Model) { m.evaluations = msg })

	case nodesMsg:
		return m.applyList(screenNodes, func(m *Model) { m.nodes = msg })

	case variablesMsg:
		return m.applyList(screenVariables, func(m *Model) { m.variables = msg })

	case nodePoolsMsg:
		return m.applyList(screenNodePools, func(m *Model) { m.nodePools = msg })

	case versionMsg:
		m.nomadVersion = string(msg)

		return m, nil

	case errMsg:
		// The rows that are on the screen stay there. An empty table reads as
		// an empty cluster.
		m.err = msg.err

		return m, m.schedulePoll()

	case describeMsg:
		return m.showDescribe(msg)

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

// handleKey is the one place that decides who gets a key press. An overlay
// answers first and the screen never sees the key.
func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch m.overlay {
	case overlayPrompt, overlayFilter:
		return m.promptKey(msg)

	case overlayHelp:
		return m.helpKey(msg)
	}

	if m.screen.kind == screenDescribe {
		if next, cmd, handled := m.textKey(msg); handled {
			return next, cmd
		}
	}

	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit

	case "d":
		return m, m.describeCmd()

	case "h":
		return m, m.jobSpecCmd()

	case ":":
		return m.openPrompt(promptPrefix)

	case "/":
		return m.openPrompt(filterPrefix)

	case "?":
		m.overlay = overlayHelp

		return m, nil

	case "enter":
		return m.open()

	case "esc":
		return m.back()

	case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
		return m.namespaceKey(int(msg.Code - '0'))

	case "up", "k":
		m.table.move(-1)

	case "down", "j":
		m.table.move(1)

	case "pgup", "ctrl+b":
		m.table.move(-m.table.height)

	case "pgdown", "ctrl+f":
		m.table.move(m.table.height)

	case "home", "g":
		m.table.move(-len(m.table.rows))

	case "end", "G":
		m.table.move(len(m.table.rows))
	}

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

	if m.overlay == overlayPrompt || m.overlay == overlayFilter {
		parts = append(parts, indent(m.prompt.view(width), screenPadX))
	}

	body := m.table.view()
	title := m.title()

	if m.screen.kind == screenDescribe {
		body = m.text.view()
	}

	if m.overlay == overlayHelp {
		body = renderHelp(m.helpSections(), width-2)
		title = "Help"
	}

	parts = append(parts,
		indent(frame(title, body, width, m.bodyHeight()), screenPadX),
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
		namespace:    m.namespace,
		namespaces:   m.namespaceColumnData(),
		hints:        m.screen.hints(),
	}
}

func (m Model) status() string {
	width := m.width - 2*headerPadX

	if m.err != nil {
		return styleError.Render(ansi.Truncate("! "+m.err.Error(), width, "…"))
	}

	return styleMuted.Render(ansi.Truncate("q quit", width, "…"))
}

func (m Model) bodyHeight() int {
	height := m.height - screenPadTop - headerHeight - statusHeight

	if m.overlay == overlayPrompt || m.overlay == overlayFilter {
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
	m.table.setSize(m.width-2*screenPadX-2, max(m.bodyHeight()-3, 1))
	m.text.setSize(m.width-2*screenPadX-2, max(m.bodyHeight()-2, 1))

	rows, index := filterRows(m.rows(), m.filter)
	m.index = index
	m.table.setRows(rows)
}

func (m Model) schedulePoll() tea.Cmd {
	return tea.Tick(m.opts.PollEvery, func(time.Time) tea.Msg { return pollMsg{} })
}

func fetchVersion(client Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		version, err := client.Version(ctx)
		if err != nil {
			return errMsg{err: err}
		}

		return versionMsg(version)
	}
}
