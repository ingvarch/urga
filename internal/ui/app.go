// Package ui is the terminal interface: one root model that owns the screen
// and the keyboard.
package ui

import (
	"context"
	"fmt"
	"image/color"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ingvarch/urga/internal/config"
	"github.com/ingvarch/urga/internal/nomad"
)

// Options are what the session starts with.
type Options struct {
	// Cluster is the name the settings give the cluster, empty for the one
	// of the environment. Color is what the settings paint it in.
	Cluster string
	Color   string

	// Clusters are the names of the settings, and Connect connects to one
	// of them. Without it there is nothing to switch to.
	Clusters []string
	Connect  func(name string) (Connection, error)

	// Namespace the session looks at. Empty is every namespace.
	Namespace string

	// NamespaceGiven says the namespace was typed on the command line, so
	// it wins over the one the last session left.
	NamespaceGiven bool

	// ReadOnly takes away every key that changes the cluster.
	ReadOnly bool

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

	// InRegion is the same cluster asked in another region. It may be nil,
	// and switching regions then says so.
	InRegion func(region string) Client

	// NewerRelease asks whether a newer urga is out: its version, or empty.
	// Nil asks nothing.
	NewerRelease func(ctx context.Context) (string, error)
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
	allocMsg       nomad.Alloc
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

	// versionsMsg carries the job it was asked of: version numbers belong
	// to one job, and the ones of another must not stand under its name.
	versionsMsg struct {
		jobID    string
		versions []nomad.JobVersion
	}

	// nodeMetaMsg carries the machine it was asked of, like every answer
	// that belongs to one client.
	nodeMetaMsg struct {
		nodeID string
		meta   []nomad.MetaEntry
	}

	// raftMsg is what the raft of the cluster says about its servers. An
	// ACL may hold it back, and then the reason is shown where the answer
	// would have been.
	raftMsg struct {
		peers []nomad.RaftPeer
		err   error
	}

	agentMsg nomad.Agent
	errMsg   struct{ err error }
	pollMsg  struct{}
)

// Model is the whole interface. It owns the screen, the keyboard and what the
// cluster last said.
type Model struct {
	client Client
	opts   Options

	// namespaceState is the namespace the session looks at and the keys
	// that switch it.
	namespaceState

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

	// flash is the one thing the status line has to say: what came of an
	// action, something worth knowing, or something that went wrong.
	flash flash

	// editing is the file that is open in the editor.
	editing editFileMsg

	// troubled leaves only what the cluster is not happy with.
	troubled bool

	clusterData

	// newer is a release of urga newer than this one, empty when there is
	// none or it is not known. It is about urga, not the cluster: a switch
	// keeps it.
	newer string

	// list is how the rows of the open screen are read.
	list list

	text textModel
	logs logState
	plan planState

	// jobLogs are the logs of a task in every allocation that runs it.
	// Not an answer of the cluster: requests held open, closed on leaving.
	jobLogs jobLogsState

	// asked counts the screens put up, so that an answer to one that is no
	// longer up is dropped.
	asked int

	// polling says a timer is already on its way with the next ask. Every
	// answer would otherwise schedule one, and a screen that is answered
	// from several sides would end up with a timer per answer.
	polling bool

	// watch is the cluster saying when what the screen shows changed.
	watch watchState

	nomadVersion string

	// usage is what the cluster and the rows on the screen are busy with.
	usage usageState

	// regionState is where in the cluster the session looks.
	regionState

	// connection counts the clusters the session connected to. What was
	// asked of one it left is answered for no one.
	connection int

	// token is the token the session sends, as the cluster sees it; nil
	// until it has said.
	token *nomad.Token

	// answered says the screen that is open has had its answer: an empty
	// list before it is not an empty list yet.
	answered bool

	// refused is a request the cluster refused before it said whose token
	// the session sends.
	refused error
}

// clusterData is what the cluster last said in the region the session asks
// in, kept in one place so that leaving the region lets go of all of it.
type clusterData struct {
	allocs      []nomad.Alloc
	deployments []nomad.Deployment
	nodes       []nomad.Node

	// host is the machine a client screen is open on, and the readings
	// taken of it since it was opened.
	host hostModel

	// nodeDetail is what the machine of a client screen says about itself,
	// nodeMeta the metadata it carries.
	nodeDetail nomad.NodeDetail
	nodeMeta   []nomad.MetaEntry

	// dir is the directory of an allocation the files screen last listed.
	dir dirState

	// deployment is the deployment a deployment screen last read.
	deployment nomad.DeploymentDetail

	// logPick is the question which task to read the logs of.
	logPick logPick
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
		list:      newList(jobTitles),
	}
	m.screen = m.screenOf(screenJobs)

	return m.restore()
}

// Init asks the cluster for what the first screen shows, and for what the
// session needs whatever is open.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.fetch(),
		m.watchScreen(),
		m.askAboutTheCluster(),
		m.checkRelease(),
	)
}

// Update is the entry for every message. It keeps the concrete model, the
// interface method wraps it.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	before := m.flash.at

	next, cmd := m.update(msg)

	// A message that has just been put up asks for the redraw that will
	// take it down again.
	if next.flash.text != "" && next.flash.at != before {
		cmd = tea.Batch(cmd, flashTimer(next.flash.at))
	}

	return next, cmd
}

func (m Model) update(msg tea.Msg) (Model, tea.Cmd) {
	// An answer the open page asked for is the page's to keep.
	if p := m.screen.page; p != nil {
		if next, out, ok := p.take(msg, m.env()); ok {
			return m.took(next, out)
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.resize(msg), nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case tea.PasteMsg:
		return m.paste(msg.Content)

	case answerMsg:
		if msg.asked != m.asked {
			return m, nil
		}

		return m.update(msg.msg)

	case connectionMsg:
		if msg.connection != m.connection {
			return m, nil
		}

		return m.update(msg.msg)

	case connectedMsg:
		return m.connected(Connection(msg))

	case backMsg:
		return m.back()

	case openMsg:
		return m.push(screen(msg))

	case sayMsg:
		return m.say(string(msg)), nil

	case askMsg:
		return m.ask(msg.question, msg.apply)

	case markMsg:
		return mark(m)

	case markAllMsg:
		return markAll(m)

	case jobLogsMsg:
		return m.askLogScope(screen(msg))

	case scaleMsg:
		return m.askScale(nomad.TaskGroup(msg))

	case signalMsg:
		return m.askForSignal(taskRef(msg))

	case shellMsg:
		return m.openShell(shellCommand(msg))

	case logsMsg:
		return m.followLog(screen(msg))

	case openNodeMsg:
		return m.openNode(nomad.Node(msg))

	case failMsg:
		return m.fail(msg.err), nil

	case forgetMsg:
		return m.forget(), nil

	case switchRegionMsg:
		return m.switchRegion(string(msg))

	case narrowMsg:
		return m.narrow(string(msg), Model.back)

	case switchClusterMsg:
		return m.switchCluster(string(msg))

	case tokenMsg:
		return m.keepToken(msg), nil

	case allocsMsg:
		return hostOnce(usageOnce(m.applyWhen(m.screen.listsAllocs(), func(m *Model) { m.allocs = msg })))

	case deploymentsMsg:
		return m.applyList(screenDeployments, func(m *Model) { m.deployments = msg })

	case namespacesMsg:
		// The namespaces are kept whatever is on the screen: the command
		// line and the number keys need the list to switch between them.
		m.namespaces = msg
		m.rememberNamespaces(msg)

		next, cmd := m.applyList(screenNamespaces, func(*Model) {})

		return next, tea.Batch(cmd, next.remember())

	case nodesMsg:
		return usageOnce(m.applyList(screenNodes, func(m *Model) { m.nodes = m.nodesInView(msg) }))

	case agentMsg:
		return m.keepAgent(msg), nil

	case regionsMsg:
		return m.keepRegions(msg)

	case datacentersMsg:
		return m.keepDatacenters(msg)

	case usageMsg:
		return m.keepClusterUsage(msg)

	case pollUsageMsg:
		return m.readClusterUsage()

	case rowUsageMsg:
		return m.keepRowUsage(msg)

	case pollUsageRow:
		return m.readRows()

	case errMsg:
		// The rows that are on the screen stay there. An empty table reads as
		// an empty cluster.
		if why, ok := m.refusedBecause(msg.err); ok {
			return m.flashed(why, flashErr).schedulePoll()
		}

		// Refused before the cluster said whose token it is: said again
		// when it has.
		if nomad.Forbidden(msg.err) {
			m.refused = msg.err
		}

		return m.fail(msg.err).schedulePoll()

	case describeMsg:
		return m.showDescribe(msg)

	case planMsg:
		return m.showPlan(msg), nil

	case planDoneMsg:
		return m.finishPlan(msg), nil

	case editFileMsg:
		return m.startEdit(msg)

	case editedMsg:
		return m.finishEdit(msg)

	case shellDoneMsg:
		if msg.err != nil {
			return m.fail(msg.err), nil
		}

		return m.say(fmt.Sprintf("Shell in %s closed.", msg.task)), nil

	case logStreamMsg:
		return m.openedLog(msg)

	case fileMsg:
		return m.openedFile(msg)

	case logLineMsg:
		if allocID, ok := m.jobLogs.streams[msg.stream]; ok && m.screen.kind == screenJobLogs {
			return m.appendJobLog(msg.stream, allocID, msg.text)
		}

		if !m.fromOpenStream(msg.stream) {
			return m, nil
		}

		return m.appendLog(msg.text)

	case logEndMsg:
		if allocID, ok := m.jobLogs.streams[msg.stream]; ok {
			return m.endJobLog(msg.stream, allocID), nil
		}

		if m.fromOpenStream(msg.stream) {
			m.logs = m.logs.ended()
		}

		return m, nil

	case logScopeMsg:
		return m.showLogScope(msg)

	case jobLogOpenedMsg:
		return m.openedJobLog(msg)

	case jobLogAllocsMsg:
		return m.reloadedJobLogs(msg)

	case savedMsg:
		return m.say(fmt.Sprintf("Saved to %s.", msg.path)), nil

	case doneMsg:
		// What did not happen stays marked, so the same key tries it again;
		// what did happen is let go of, so the key does not undo it.
		m.list.marks = msg.kept

		if msg.err != nil {
			m = m.fail(msg.err)
		} else {
			m = m.say(msg.said)
		}

		m.layout()

		// The list is stale the moment the cluster changed, ask again.
		return m, m.fetch()

	case nodeDetailMsg:
		// The answer belongs to the machine it was asked of: leaving one
		// client for another must not show the first one under the second.
		return m.applyWhen(m.screen.nodeID == msg.ID, func(m *Model) { m.nodeDetail = nomad.NodeDetail(msg) })

	case nodeMetaMsg:
		return m.applyWhen(m.screen.kind == screenNodeMeta && m.screen.nodeID == msg.nodeID,
			func(m *Model) { m.nodeMeta = msg.meta })

	case hostMsg:
		return m.keepHost(msg), nil

	case hostUseMsg:
		return m.keepHostUse(msg)

	case pollHostMsg:
		return m.pollHost()

	case deploymentMsg:
		return m.keepDeployment(msg)

	case newerReleaseMsg:
		return m.keepRelease(msg)

	case checkReleaseMsg:
		return m, m.checkRelease()

	case refusedEditMsg:
		return m, m.reopenEdit(msg)

	case filesMsg:
		return m.applyList(screenFiles, func(m *Model) { m.dir = dirState(msg) })

	case watchingMsg:
		var cmd tea.Cmd
		m.watch, cmd = m.watch.start(msg)

		return m, cmd

	case changeMsg:
		var cmd tea.Cmd
		m.watch, cmd = m.watch.keepChange(msg)

		return m, cmd

	case settleMsg:
		return m.settled()

	case flashOverMsg:
		return m.clearFlash(msg), nil

	case watchEndedMsg:
		return m.watchEnded(msg), nil

	case pollMsg:
		return m.poll()
	}

	return m, nil
}

// applyList stores what the cluster sent, unless the screen it belongs to
// was left: it would show up under the wrong title.
func (m Model) applyList(kind screenKind, store func(*Model)) (Model, tea.Cmd) {
	return m.applyWhen(m.screen.kind == kind, store)
}

// applyWhen stores an answer that belongs to what is open, and asks again
// in a while.
func (m Model) applyWhen(ours bool, store func(*Model)) (Model, tea.Cmd) {
	if !ours {
		return m, nil
	}

	store(&m)
	m.answered = true
	m = m.forget()
	m.layout()

	return m.schedulePoll()
}

// handleKey is the one place that decides who gets a key press: an overlay
// first, then what the open resource can do, then the way around the screen.
// What the resource can do is the table of its screen: a key answers there
// or nowhere, and each one looks at the row under the cursor.
func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.overlay != overlayNone {
		return m.overlayKey(msg)
	}

	if b, ok := m.binding(msg.String()); ok {
		return b.do(m)
	}

	// A key read-only took away says why, rather than do nothing.
	if b, ok := m.withheldKey(msg.String()); ok {
		return m.warn(fmt.Sprintf("read-only: %s is off", b.label)), nil
	}

	if next, cmd, handled := m.planButtonKey(msg); handled {
		return next, cmd
	}

	if next, handled := m.scrollKey(msg); handled {
		return next, nil
	}

	return m.sessionKey(msg)
}

// overlayKey hands the key to whatever took the keyboard.
func (m Model) overlayKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch m.overlay {
	case overlayPrompt, overlayFilter, overlayScale, overlaySignal:
		return m.promptKey(msg)

	case overlayHelp:
		return m.helpKey(msg)

	case overlayConfirm:
		return m.confirmKey(msg)
	}

	return m, nil
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

	m.list.sort = m.list.sort.by(column)
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

	framed := paintedFrame(title, body, width, m.bodyHeight(), m.border())

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

// clusterColour is what the settings paint the cluster in, nil for none.
func (m Model) clusterColour() color.Color {
	return clusterColours[m.opts.Color]
}

// border is the style of the box around the screen: the colour of the
// cluster, seen out of the corner of an eye while reading the rows.
func (m Model) border() lipgloss.Style {
	if paint := m.clusterColour(); paint != nil {
		return lipgloss.NewStyle().Foreground(paint)
	}

	return styleBorder
}

// headerData is what the top of the screen says about the session.
func (m Model) headerData() header {
	return header{
		cluster:       m.opts.Cluster,
		clusterColour: m.clusterColour(),
		address:       m.client.Address(),
		region:        m.regionInUse(),
		datacenter:    m.datacenter,
		version:       m.opts.Version,
		newer:         m.newer,
		nomadVersion:  m.nomadVersion,
		usage:         percentOf(m.usage.cluster.CPUPercent),
		memory:        percentOf(m.usage.cluster.MemoryPercent),
		namespaces:    m.namespaceColumnData(),
		hints:         m.hints(),
		readOnly:      m.opts.ReadOnly,
	}
}

// cursorLine is where the row under the cursor is drawn inside the box: the
// top border and the header of the table come before it. A screen that reads
// as text has no such row.
func (m Model) cursorLine() int {
	if m.readsAsText() {
		return -1
	}

	return 2 + m.panelHeight() + m.list.table.cursor - m.list.table.top
}

// body is what fills the box: what took the screen, or what the screen
// shows.
func (m Model) body(width int) (title, content string) {
	switch {
	case m.overlay == overlayHelp:
		return "Help", renderHelp(m.helpSections(), width)

	case m.readsAsText():
		if m.toggleRows() > 0 {
			return m.title(), m.logToggles(width) + "\n" + m.text.view()
		}

		// The question stays at the foot of the box, however short the plan.
		if m.barRows() > 0 {
			view := m.text.view()
			gap := strings.Repeat("\n", max(m.text.height-strings.Count(view, "\n")-1, 0))

			return m.title(), view + gap + "\n" + m.planBar(width)
		}

		return m.title(), m.text.view()
	}

	if panel := m.panelHeight(); panel > 0 {
		return m.title(), strings.Join(append(m.panel(width), m.withHint(m.list.table.view(), width)), "\n")
	}

	return m.title(), m.withHint(m.list.table.view(), width)
}

func (m Model) status() string {
	width := m.width - 2*headerPadX

	if m.token == nil {
		return m.statusLeft(width)
	}

	value, style, ok := tokenStatus(*m.token, time.Now())
	if !ok {
		return m.statusLeft(width)
	}

	// The token against the right edge; what is on the left gives way to it.
	wide := ansi.StringWidth(tokenLabel + value)
	left := m.statusLeft(width - wide - columnGap)
	gap := max(width-ansi.StringWidth(left)-wide, columnGap)

	return left + strings.Repeat(" ", gap) + styleLabel.Render(tokenLabel) + style.Render(value)
}

// statusLeft is what the status line says on the left: a message, the mode
// the list is in, or the keys that work everywhere.
func (m Model) statusLeft(width int) string {
	if m.flash.fresh() {
		return m.flash.view(width)
	}

	if m.troubled {
		return styleWarn.Render(truncate(
			fmt.Sprintf("only what needs attention, %d of %d   <!> all of them", m.list.shown, m.list.held), width))
	}

	if missing := m.usage.missingNote(); missing != "" {
		return styleMuted.Render(truncate(missing, width))
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

// resize fits the screen to the window.
func (m Model) resize(msg tea.WindowSizeMsg) Model {
	m.width, m.height = msg.Width, msg.Height
	m.layout()

	return m
}

// layout sizes the table to the window and fills it with what the cluster
// last said.
func (m *Model) layout() {
	if m.width == 0 {
		return
	}

	// The table sits inside the box: the margin, its two border lines and the
	// header row of the table itself are not rows.
	m.list.table.setSize(m.width-2*screenPadX-2, max(m.bodyHeight()-3-m.panelHeight(), 1))
	m.text.setSize(m.width-2*screenPadX-2, max(m.bodyHeight()-2-m.toggleRows()-m.barRows(), 1))

	m.text.filter = m.list.filter
	m.text.follow()

	m.list = m.list.read(m.rows(), m.screen.titles(), m.ids(), m.troubled)
}

// schedulePoll asks for the next poll, unless one is already on its way.
// The caller keeps the model it is given: the promise to poll lives in it.
func (m Model) schedulePoll() (Model, tea.Cmd) {
	if m.polling {
		return m, nil
	}

	m.polling = true

	return m, tea.Tick(m.pollEvery(), func(time.Time) tea.Msg { return pollMsg{} })
}

// poll asks for the screen again.
func (m Model) poll() (Model, tea.Cmd) {
	// The timer has fired and there is room for the next one, which the
	// answer to this ask will set.
	m.polling = false

	return m, m.fetch()
}

// percentOf is a reading of the cluster, empty until there is one.
func percentOf(value int) string {
	if value == 0 {
		return ""
	}

	return fmt.Sprintf("%d%%", value)
}

func fetchAgent(client clusterClient) tea.Cmd {
	return request(client.Agent, func(agent nomad.Agent) tea.Msg { return agentMsg(agent) })
}

// keepAgent keeps what the agent says about itself: its version is in the
// header, and its region answers a session that names none.
func (m Model) keepAgent(agent agentMsg) Model {
	m.nomadVersion = agent.Version
	m.agentRegion = agent.Region

	return m
}
