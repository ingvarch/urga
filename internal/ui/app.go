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
	// of the environment. Color is the colour the settings give it.
	Cluster string
	Color   string

	// Clusters are the names of the settings, and Connect connects to one
	// of them. Without it there is nothing to switch to.
	Clusters []string
	Connect  func(name string) (Connection, error)

	// Namespace the session looks at. Empty is every namespace.
	Namespace string

	// NamespaceGiven means the namespace was typed on the command line, so
	// it replaces the one the last session saved.
	NamespaceGiven bool

	// ReadOnly disables every key that changes the cluster.
	ReadOnly bool

	// Version of urga, for the header.
	Version string

	// PollEvery is the wait between an answer and the next ask.
	PollEvery time.Duration

	// Config is what the last session saved. It may be nil.
	Config *config.Config

	// Editor opens a resource in the user's editor. It may be nil; editing
	// then shows an error.
	Editor Editor

	// Shell runs a shell inside a task. It may be nil.
	Shell Shell

	// InRegion is a client for the same cluster in another region. It may
	// be nil; switching regions then shows an error.
	InRegion func(region string) Client

	// NewerRelease checks for a newer release of urga: its version, or
	// empty. Nil skips the check.
	NewerRelease func(ctx context.Context) (string, error)
}

const (
	defaultPollEvery = 2 * time.Second
	defaultTimeout   = 10 * time.Second

	statusHeight = 1

	// The screen keeps a margin from the edges of the terminal: a blank
	// line on top, the box one column in, the text of the header and the
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

	// versionsMsg carries the job it was requested for: version numbers
	// belong to one job, and those of another must not show under its name.
	versionsMsg struct {
		jobID    string
		versions []nomad.JobVersion
	}

	// nodeMetaMsg carries the node it was requested for, like every answer
	// about one client.
	nodeMetaMsg struct {
		nodeID string
		meta   []nomad.MetaEntry
	}

	// raftMsg holds the raft peers of the cluster. An ACL may deny the
	// request, and then the error is shown where the peers would have
	// been.
	raftMsg struct {
		peers []nomad.RaftPeer
		err   error
	}

	agentMsg nomad.Agent
	errMsg   struct{ err error }
	pollMsg  struct{}
)

// Model is the whole interface. It owns the screen, the keyboard and the last
// answers of the cluster.
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

	// flash is the one message the status line shows: the result of an
	// action, something worth knowing, or an error.
	flash flash

	// editing is the file that is open in the editor.
	editing editFileMsg

	// troubled shows only the rows that need attention.
	troubled bool

	// newer is a release of urga newer than this one, empty when there is
	// none or it is not known. It is about urga, not the cluster:
	// switching clusters keeps it.
	newer string

	// list is how the rows of the open screen are read.
	list list

	text textModel

	// asked counts the screens opened, so that an answer for one that is
	// closed is dropped.
	asked int

	// polling means a timer for the next poll is already running. Every
	// answer would otherwise schedule one, and a screen that gets answers
	// from several requests would end up with a timer per answer.
	polling bool

	// watch is the event stream that signals when the screen is stale.
	watch watchState

	nomadVersion string

	// usage is the CPU and memory use of the cluster and of the rows.
	usage usageState

	// regionState is the region and datacenter the session shows.
	regionState

	// connection counts the clusters the session connected to. An answer
	// from a cluster the session left is dropped.
	connection int

	// token is the token the session sends, as the cluster sees it; nil
	// until the cluster answers.
	token *nomad.Token

	// answered means the open screen has got its answer: before that, an
	// empty list does not yet mean there is nothing.
	answered bool

	// refused is a request the cluster refused before it reported whose
	// token the session sends.
	refused error
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
	m.screen = jobsView.opened()

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

	// A message that was just shown schedules the redraw that will hide
	// it again.
	if next.flash.text != "" && next.flash.at != before {
		cmd = tea.Batch(cmd, flashTimer(next.flash.at))
	}

	return next, cmd
}

// applyList stores what the cluster sent, unless the screen it belongs to
// was left: it would show up under the wrong title.
func (m Model) applyList(v *view, store func(*Model)) (Model, tea.Cmd) {
	return m.applyWhen(m.screen.view == v, store)
}

// takeError shows the error. The rows that are on the screen stay there:
// an empty table looks like an empty cluster.
func (m Model) takeError(err error) (Model, tea.Cmd) {
	if why, ok := m.refusedBecause(err); ok {
		return m.flashed(why, flashErr).schedulePoll()
	}

	// Refused before the cluster reported whose token it is: the error is
	// shown again, with the reason, once it has.
	if nomad.Forbidden(err) {
		m.refused = err
	}

	return m.fail(err).schedulePoll()
}

// applyWhen stores an answer for what is open, and schedules the next poll.
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
// first, then the actions of the open resource, then navigation. The actions
// of a resource are the key table of its screen: a key is handled there or
// not at all, and each one acts on the row under the cursor.
func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.overlay != overlayNone {
		return m.overlayKey(msg)
	}

	if k, ok := m.offeredKey(msg.String()); ok {
		return m.pressPage(k.press)
	}

	// A key disabled by read-only shows why, rather than do nothing.
	if k, ok := m.withheldKey(msg.String()); ok {
		return m.warn(fmt.Sprintf("read-only: %s is off", k.label)), nil
	}

	if next, cmd, handled := m.buttonKey(msg); handled {
		return next, cmd
	}

	if next, handled := m.scrollKey(msg); handled {
		return next, nil
	}

	return m.sessionKey(msg)
}

// overlayKey passes the key to the overlay that has the keyboard.
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

// sortKey orders the list by the column a letter names, so that one key
// picks a column.
func (m Model) sortKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if len(msg.Text) != 1 {
		return m, nil
	}

	letter := []rune(msg.Text)[0]
	if !unicode.IsUpper(letter) {
		return m, nil
	}

	column, ok := columnOfLetter(m.screen.page.titles(), letter)
	if !ok {
		return m, nil
	}

	m.list.sort = m.list.sort.by(column)
	m.layout()

	return m, nil
}

// helpKey closes the help window; it handles no other key.
func (m Model) helpKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter", "?", "q":
		m.overlay = overlayNone
	}

	return m, nil
}

// View draws the screen. urga runs in the alternate screen, so the terminal
// is restored on exit.
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

	// A question is drawn over the screen, next to the row it is about.
	if m.overlay == overlayConfirm {
		framed = modal(framed, m.confirm.view(width-2*modalPadX), m.cursorLine())
	}

	parts = append(parts,
		indent(framed, screenPadX),
		indent(m.status(), headerPadX),
	)

	return strings.Join(parts, "\n")
}

// clusterColour is the colour the settings give the cluster, nil for none.
func (m Model) clusterColour() color.Color {
	return clusterColours[m.opts.Color]
}

// border is the style of the box around the screen: the colour of the
// cluster, so it stays in sight while reading the rows.
func (m Model) border() lipgloss.Style {
	if paint := m.clusterColour(); paint != nil {
		return lipgloss.NewStyle().Foreground(paint)
	}

	return styleBorder
}

// headerData is what the header shows about the session.
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

// body is what fills the box: the help window when it is open, or what the
// screen shows.
func (m Model) body(width int) (title, content string) {
	switch {
	case m.overlay == overlayHelp:
		return "Help", renderHelp(m.helpSections(), width)

	case m.readsAsText():
		if m.toggleRows() > 0 {
			return m.title(), m.logToggles(width) + "\n" + m.text.view()
		}

		// The question stays at the foot of the box, however short the plan.
		if bar := m.bar(width); len(bar) > 0 {
			view := m.text.view()
			gap := strings.Repeat("\n", max(m.text.height-strings.Count(view, "\n")-1, 0))

			return m.title(), view + gap + "\n" + strings.Join(bar, "\n")
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

	// The token sits at the right edge; the left part is cut to make room.
	wide := ansi.StringWidth(tokenLabel + value)
	left := m.statusLeft(width - wide - columnGap)
	gap := max(width-ansi.StringWidth(left)-wide, columnGap)

	return left + strings.Repeat(" ", gap) + styleLabel.Render(tokenLabel) + style.Render(value)
}

// statusLeft is what the status line shows on the left: a message, the mode
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

// layout sizes the table to the window and fills it with the last answers
// of the cluster.
func (m *Model) layout() {
	if m.width == 0 {
		return
	}

	// The table sits inside the box: the margin, its two border lines and the
	// header row of the table itself are not rows.
	m.list.table.setSize(m.width-2*screenPadX-2, max(m.bodyHeight()-3-m.panelHeight(), 1))

	grew := false
	if r, ok := m.screen.page.(reader); ok {
		content := r.text(m.env())
		grew = len(content.lines) > len(m.text.lines)
		m.text.textContent = content
	}

	m.text.setSize(m.width-2*screenPadX-2, max(m.bodyHeight()-2-m.toggleRows()-m.barRows(), 1))

	m.text.filter = m.list.filter
	m.text.follow()

	// A window that follows a stream scrolls to the end as lines arrive.
	if grew && m.text.following {
		m.text.toEnd()
	}

	m.list = m.list.read(m.rows(), m.screen.page.titles(), m.ids(), m.troubled)
}

// schedulePoll schedules the next poll, unless one is already scheduled.
// The caller keeps the model it returns: that model records the schedule.
func (m Model) schedulePoll() (Model, tea.Cmd) {
	if m.polling {
		return m, nil
	}

	m.polling = true

	return m, tea.Tick(m.pollEvery(), func(time.Time) tea.Msg { return pollMsg{} })
}

// poll asks for the screen again.
func (m Model) poll() (Model, tea.Cmd) {
	// The timer has fired, so the next one may be set. The answer to this
	// request sets it.
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

// keepAgent stores what the agent reports about itself: its version goes in
// the header, and its region is used when the session names none.
func (m Model) keepAgent(agent agentMsg) Model {
	m.nomadVersion = agent.Version
	m.agentRegion = agent.Region

	return m
}
