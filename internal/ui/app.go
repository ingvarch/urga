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
)

// Messages. Every answer from the cluster arrives as one of these, the model
// changes nowhere else.
type (
	jobsMsg    []nomad.Job
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

	jobs  []nomad.Job
	table tableModel

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
		table:     newTableModel(jobTitles),
	}
}

// Init asks the cluster for what the first screen shows.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		fetchJobs(m.client, m.namespace),
		fetchVersion(m.client),
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
		m.jobs = msg
		m.err = nil
		m.layout()

		return m, m.schedulePoll()

	case versionMsg:
		m.nomadVersion = string(msg)

		return m, nil

	case errMsg:
		// The rows that are on the screen stay there. An empty table reads as
		// an empty cluster.
		m.err = msg.err

		return m, m.schedulePoll()

	case pollMsg:
		return m, fetchJobs(m.client, m.namespace)
	}

	return m, nil
}

// handleKey is the one place that decides who gets a key press.
func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit

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

	parts := []string{
		renderHeader(header{
			address:      m.client.Address(),
			version:      m.opts.Version,
			nomadVersion: m.nomadVersion,
			namespace:    m.namespace,
		}, m.width),
		frame(jobsTitle(m.namespace, len(m.jobs)), m.table.view(), m.width, m.bodyHeight()),
		m.status(),
	}

	return strings.Join(parts, "\n")
}

func (m Model) status() string {
	if m.err != nil {
		return ansi.Truncate(styleError.Render("! "+m.err.Error()), m.width, "…")
	}

	return styleMuted.Render(ansi.Truncate("q quit", m.width, "…"))
}

func (m Model) bodyHeight() int {
	return max(m.height-headerHeight-statusHeight, 2)
}

// layout sizes the table to the window and fills it with what the cluster
// last said.
func (m *Model) layout() {
	if m.width == 0 {
		return
	}

	// The table sits inside the box: its two border lines and the header row
	// of the table itself are not rows.
	m.table.setSize(m.width-2, max(m.bodyHeight()-3, 1))
	m.table.setRows(jobRows(m.jobs))
}

func (m Model) schedulePoll() tea.Cmd {
	return tea.Tick(m.opts.PollEvery, func(time.Time) tea.Msg { return pollMsg{} })
}

// fetchJobs asks the cluster for the job list of a namespace.
func fetchJobs(client Client, namespace string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		jobs, err := client.Jobs(ctx, namespace)
		if err != nil {
			return errMsg{err: err}
		}

		return jobsMsg(jobs)
	}
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
