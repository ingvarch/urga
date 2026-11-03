package ui

import (
	"io"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// agentLogOf is what the open agent log reads its stream through.
func agentLogOf(m Model) logState {
	return agentLogsPageOf(m).read
}

func agentLogsPageOf(m Model) agentLogsPage {
	p, _ := m.screen.page.(agentLogsPage)

	return p
}

// onAgentLogs opens the log of the agent urga talks to by its command, with
// the stream opened and waiting for its first line.
func onAgentLogs(t *testing.T, client *fakeClient) Model {
	t.Helper()

	m := newTestModel(client)
	m, _ = m.update(key(':'))
	m = typeIn(m, "agentlogs")
	m, cmd := m.update(enter())

	return drain(m, cmd)
}

// clientListAndScreen opens the clients and, from the list, the screen of the
// first one.
func clientListAndScreen(t *testing.T, client *fakeClient) (list, screen Model) {
	t.Helper()

	list = typeCommand(newTestModel(client), "clients")
	list, _ = list.update(nodesMsg(client.nodes))

	screen, cmd := list.update(enter())

	return list, drain(screen, cmd)
}

func TestAgentLogs_TheCommandFollowsTheAgentUrgaTalksTo(t *testing.T) {
	r := require.New(t)

	lines := make(chan string, 1)
	lines <- "2026-11-03 [INFO]  agent: started\n"

	client := &fakeClient{agentLogs: &nomad.LogStream{Lines: lines}}
	m := onAgentLogs(t, client)

	r.IsType(agentLogsPage{}, m.screen.page)
	r.Equal([]nomad.AgentTarget{{}}, client.agentTargets)
	r.Equal([]string{"info"}, client.agentLevels)

	m = drain(m, agentLogOf(m).waitForLog())

	out := plain(m.render())
	r.Contains(out, "Agent Logs (Agent: https://nomad.example.com) [info]")
	r.Contains(out, "agent: started")
	r.Contains(out, "Autoscroll:On")
}

func TestAgentLogs_CtrlLOnAClientFollowsThatClient(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{nodes: busyClient(), nodeDetail: clientDetail(), nodeAllocs: clientAllocs()}
	list, screen := clientListAndScreen(t, client)

	r.IsType(clientPage{}, screen.screen.page)

	for name, from := range map[string]Model{"list": list, "screen": screen} {
		client.agentTargets = nil

		m, cmd := from.update(ctrlKey('l'))
		m = drain(m, cmd)

		r.IsType(agentLogsPage{}, m.screen.page, name)
		r.Equal([]nomad.AgentTarget{{NodeID: "node-1"}}, client.agentTargets, name)
		r.Contains(plain(m.render()), "Agent Logs (Client: nomad-server-01) [info]", name)
	}
}

func TestAgentLogs_CtrlLOnAServerFollowsThatServer(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{servers: twoServers()}

	list, screen := serversListAndScreen(t, client, true)
	r.IsType(serverPage{}, screen.screen.page)

	for name, from := range map[string]Model{"list": list, "screen": screen} {
		client.agentTargets = nil

		m, cmd := from.update(ctrlKey('l'))
		m = drain(m, cmd)

		r.IsType(agentLogsPage{}, m.screen.page, name)
		r.Equal([]nomad.AgentTarget{{ServerID: "server-01.global"}}, client.agentTargets, name)
		r.Contains(plain(m.render()), "Agent Logs (Server: server-01.global) [info]", name)
	}
}

// A client agent hands the request for a server log to nobody and answers
// with its own log, so urga does not offer the key then.
func TestAgentLogs_CtrlLOnAServerNeedsAServerAgent(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{servers: twoServers()}
	list, screen := serversListAndScreen(t, client, false)

	for name, from := range map[string]Model{"list": list, "screen": screen} {
		r.NotContains(plain(from.render()), "Agent Logs", name)

		m, cmd := from.update(ctrlKey('l'))
		m = drain(m, cmd)

		r.Equal(from.screen.page, m.screen.page, name)
		r.Empty(client.agentTargets, name)
	}

	for name, readOnly := range map[string]bool{"read-write": false, "read-only": true} {
		m := serversListOf(client, true)
		m.opts.ReadOnly = readOnly

		r.Contains(plain(m.render()), "Agent Logs", name)
	}
}

func TestAgentLogs_ASwitchForgetsTheServerAgentOfTheClusterLeft(t *testing.T) {
	r := require.New(t)

	m, clusters := onDev(t)
	m, _ = m.update(agentMsg(nomad.Agent{Version: "1.11.1", Server: true}))

	clusters.prod.servers = twoServers()

	// On prod, before prod said what its agent is.
	m, _ = m.update(connectedMsg{conn: Connection{Name: "prod", Client: clusters.prod}})
	m = typeCommand(m, "servers")
	m, _ = m.update(serversMsg(clusters.prod.servers))

	r.IsType(serversPage{}, m.screen.page)
	r.NotContains(plain(m.render()), "Agent Logs")

	m, cmd := m.update(ctrlKey('l'))
	m = drain(m, cmd)

	r.IsType(serversPage{}, m.screen.page)
	r.Empty(clusters.prod.agentTargets)
}

// serversListOf is the servers list of an agent that is a server or not.
func serversListOf(client *fakeClient, server bool) Model {
	m := newTestModel(client)
	m, _ = m.update(agentMsg(nomad.Agent{Version: "1.11.1", Server: server}))
	m = typeCommand(m, "servers")
	m, _ = m.update(serversMsg(client.servers))

	return m
}

func serversListAndScreen(t *testing.T, client *fakeClient, server bool) (Model, Model) {
	t.Helper()

	list := serversListOf(client, server)
	screen, _ := list.update(enter())

	return list, screen
}

func TestAgentLogs_VStepsToMoreDetailAndStartsOver(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onAgentLogs(t, client)

	// Lines read at one level are gone at the next.
	m, _ = m.update(logLineMsg{stream: agentLogOf(m).stream, text: "old line\n"})
	r.Contains(plain(m.render()), "old line")

	for i, level := range []string{"debug", "trace", "error", "warn", "info"} {
		// Autoscroll turned off is on again at the next level.
		m, _ = m.update(key('s'))
		r.Contains(plain(m.render()), "Autoscroll:Off", level)

		var cmd tea.Cmd

		m, cmd = m.update(key('v'))
		m = drain(m, cmd)

		r.Equal(i+1, client.agentLogsClosed, level)
		r.Equal(level, client.agentLevels[i+1])

		out := plain(m.render())
		r.Contains(out, "["+level+"]", level)
		r.NotContains(out, "old line", level)
		r.Contains(out, "Autoscroll:On", level)
	}

	r.Equal([]string{"info", "debug", "trace", "error", "warn", "info"}, client.agentLevels)
}

func TestAgentLogs_TheHeaderNamesTheLevelVGoesTo(t *testing.T) {
	r := require.New(t)

	m := onAgentLogs(t, &fakeClient{})

	r.Equal([]hint{
		{Key: "<v>", Description: "Debug Level"},
		{Key: "<s>", Description: "Toggle Autoscroll"},
		{Key: "<w>", Description: "Toggle Wrap"},
		{Key: "<ctrl-s>", Description: "Save"},
		{Key: "<c>", Description: "Copy"},
		{Key: "<f>", Description: "Find"},
	}, m.hints())

	toggles := togglesOf(m)
	r.Contains(toggles, "Autoscroll")
	r.Contains(toggles, "Wrap")
	r.NotContains(toggles, "Timestamps")

	for range 2 {
		var cmd tea.Cmd

		m, cmd = m.update(key('v'))
		m = drain(m, cmd)
	}

	r.Equal("trace", agentLogsPageOf(m).level)
	r.Contains(m.hints(), hint{Key: "<v>", Description: "Error Level"})
}

func TestAgentLogs_LeavingClosesTheStream(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onAgentLogs(t, client)

	m, _ = m.update(escape())

	r.Equal(1, client.agentLogsClosed)
	r.IsType(jobsPage{}, m.screen.page)
}

func TestAgentLogs_AStreamOfALevelLeftIsClosed(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onAgentLogs(t, client)

	m, _ = m.update(key('v'))
	r.Nil(agentLogOf(m).stream)

	closed := false
	other := &nomad.LogStream{Lines: make(chan string), OnClose: func() { closed = true }}

	m, _ = m.update(agentLogOpenedMsg{stream: other, level: "info"})

	r.True(closed)
	r.Nil(agentLogOf(m).stream)
}

func TestAgentLogs_ARefusalShowsLikeOnAnyScreen(t *testing.T) {
	r := require.New(t)

	errs := make(chan error, 1)
	errs <- forbidden(t)

	m := onAgentLogs(t, &fakeClient{agentLogs: &nomad.LogStream{Lines: make(chan string), Err: errs}})
	m = drain(m, agentLogOf(m).waitForLog())

	r.Contains(statusLine(m), "Permission denied")
}

// The stream sends its error before it closes its lines, so the wait after
// the last line finds both, and a select takes either: in twenty tries it
// takes the end at least once.
func TestAgentLogs_ABreakAfterALineShowsItsError(t *testing.T) {
	r := require.New(t)

	for range 20 {
		lines := make(chan string, 1)
		lines <- "a [INFO]  agent: started\n"

		errs := make(chan error, 1)

		m := onAgentLogs(t, &fakeClient{agentLogs: &nomad.LogStream{Lines: lines, Err: errs}})
		m = drain(m, agentLogOf(m).waitForLog())

		errs <- io.ErrUnexpectedEOF

		close(lines)

		m = drain(m, agentLogOf(m).waitForLog())

		r.Contains(plain(m.render()), "agent: started")
		r.Contains(statusLine(m), "unexpected EOF")
	}
}

func TestAgentLogs_SavedUnderTheAgentAndTheLevel(t *testing.T) {
	r := require.New(t)

	r.Contains(saveName(agentLogsOf(nomad.AgentTarget{NodeID: "node-1"}, "nomad-server-01")), "agent-nomad-server-01-info-")
	r.Contains(saveName(agentLogsOf(nomad.AgentTarget{}, "")), "agent-info-")
	r.Contains(saveName(agentLogsOf(nomad.AgentTarget{ServerID: "server-01.global"}, "")), "agent-server-01.global-info-")
}
