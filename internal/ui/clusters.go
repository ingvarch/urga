package ui

import (
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/ingvarch/urga/internal/nomad"
)

// Connection is a cluster of the settings, connected to.
type Connection struct {
	Name      string
	Color     string
	ReadOnly  bool
	Namespace string

	Client   Client
	InRegion func(region string) Client
	Shell    Shell
}

// Messages of the clusters.
type (
	// connectionMsg is an answer about the cluster, with the connection it
	// was asked on.
	connectionMsg struct {
		connection int
		msg        tea.Msg
	}

	// connectedMsg is a cluster connected to.
	connectedMsg Connection
)

// errNoClusters is a session started without clusters in its settings.
var errNoClusters = errors.New("no clusters to switch to: the settings file names none")

var clusterBindings = []binding{{press: "enter", label: "Switch", do: chooseCluster}}

// clusterCommand switches to a cluster of the settings by name, or opens the
// list of them to pick one from.
func (m Model) clusterCommand(name string) (Model, tea.Cmd) {
	if m.opts.Connect == nil {
		return m.fail(errNoClusters), nil
	}

	if name == "" {
		return m.show(screenClusters)
	}

	return m.pick(name, m.opts.Clusters, "cluster", Model.switchCluster)
}

// chooseCluster switches to the cluster under the cursor. The one in use has
// nothing to switch, and the list goes back to where it was opened from.
func chooseCluster(m Model) (Model, tea.Cmd) {
	name, ok := selectedOf(m, screenClusters, m.opts.Clusters)
	if !ok {
		return m, nil
	}

	if name == m.opts.Cluster {
		return m.back()
	}

	return m.switchCluster(name)
}

// switchCluster connects to another cluster of the settings. Its token may
// take a while to read, a password manager may ask first, so it is read off
// the path that answers keys.
func (m Model) switchCluster(name string) (Model, tea.Cmd) {
	if name == m.opts.Cluster {
		return m, nil
	}

	connect := m.opts.Connect

	return m.say(fmt.Sprintf("Connecting to %s...", name)), func() tea.Msg {
		conn, err := connect(name)
		if err != nil {
			return errMsg{err: err}
		}

		return connectedMsg(conn)
	}
}

// connected puts the session on the cluster it connected to. What it knew
// belongs to the cluster it left: it starts over where the new one was
// left, and what was still asked of the old one is answered for no one.
func (m Model) connected(conn Connection) (Model, tea.Cmd) {
	m = m.stopLogs()

	m.connection++
	m.client = conn.Client
	m.opts.Cluster, m.opts.Color, m.opts.ReadOnly = conn.Name, conn.Color, conn.ReadOnly
	m.opts.InRegion, m.opts.Shell = conn.InRegion, conn.Shell
	m.opts.NamespaceGiven = false

	m = m.forgetRegion()
	m.regionState, m.nomadVersion, m.token = regionState{}, "", nil

	m.namespace, m.namespaceOrder = NamespaceOrAll(conn.Namespace), nil
	m.screen, m.history = screen{kind: screenJobs, namespace: m.namespace}, nil
	m = m.restore()

	next, cmd := m.arrive()
	next = next.say(fmt.Sprintf("Connected to %s.", conn.Name))

	return next, tea.Batch(cmd, next.askAboutTheCluster())
}

// askAboutTheCluster is what the header and the command line need whatever
// is open: what the agent is, the namespaces, regions and datacenters, and
// what the cluster is busy with.
func (m Model) askAboutTheCluster() tea.Cmd {
	client := m.client

	return tea.Batch(
		m.onConnection(tea.Batch(
			fetchAgent(client),
			fetchToken(client),
			fetchList(client.Namespaces, func(items []nomad.Namespace) tea.Msg { return namespacesMsg(items) }),
			fetchRegions(client),
			fetchDatacenters(client),
		)),
		m.fetchClusterUsage(),
	)
}

// onConnection labels what a command answers with the connection it was
// asked on.
func (m Model) onConnection(cmd tea.Cmd) tea.Cmd {
	connection := m.connection

	return labelled(cmd, func(msg tea.Msg) tea.Msg { return connectionMsg{connection: connection, msg: msg} })
}
