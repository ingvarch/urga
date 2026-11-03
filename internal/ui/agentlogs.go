package ui

import (
	"context"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// agentLogOpenedMsg is the stream of an agent log, with the level it was
// opened at: the level can change while it is on its way.
type agentLogOpenedMsg struct {
	stream *nomad.LogStream
	level  string
}

func (m agentLogOpenedMsg) release() { m.stream.Close() }

// agentLogsPage is what an agent logs, followed as it is written: the agent
// urga talks to, or a client or a server asked through the cluster.
type agentLogsPage struct {
	target nomad.AgentTarget

	// name is the name of the client, when the target is one.
	name string

	// level is the most detailed level shown; a more detailed one is left out.
	level string

	read logState
}

// agentLogsOf is the log of an agent, at info.
func agentLogsOf(target nomad.AgentTarget, name string) agentLogsPage {
	return agentLogsPage{target: target, name: name, level: "info"}
}

func (p agentLogsPage) title(e env, _ int) string {
	switch {
	case p.target.NodeID != "":
		return sprintf("Agent Logs (Client: %s) [%s]", p.name, p.level)
	case p.target.ServerID != "":
		return sprintf("Agent Logs (Server: %s) [%s]", p.target.ServerID, p.level)
	}

	return sprintf("Agent Logs (Agent: %s) [%s]", e.client.Address(), p.level)
}

func (agentLogsPage) titles() []string       { return nil }
func (agentLogsPage) topics() []string       { return nil }
func (agentLogsPage) fetch(env) tea.Cmd      { return nil }
func (agentLogsPage) rows(env) []tableRow    { return nil }
func (agentLogsPage) follows() bool          { return true }
func (p agentLogsPage) text(env) textContent { return p.read.content }

func (p agentLogsPage) saveAs() (string, string) {
	who := p.name
	if p.target.ServerID != "" {
		who = p.target.ServerID
	}

	parts := slices.DeleteFunc([]string{"agent", who, p.level}, func(s string) bool { return s == "" })

	return strings.Join(parts, "-"), "log"
}

// open reads the log from its start again: what the page read before is not
// kept, since another level holds other lines.
func (p agentLogsPage) open(e env) (page, tea.Cmd) {
	p.read.stop()
	p.read = logState{}

	return p, p.start(e.client)
}

func (p agentLogsPage) close() page {
	p.read.stop()

	return p
}

// start opens the stream. The lines arrive as messages, one command at a
// time, so nothing writes to the model from a goroutine.
func (p agentLogsPage) start(client clusterClient) tea.Cmd {
	target, level := p.target, p.level

	return func() tea.Msg {
		return agentLogOpenedMsg{stream: client.AgentLogs(context.Background(), target, level), level: level}
	}
}

func (p agentLogsPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	var (
		out  outcome
		took bool
	)

	// A stream opened at a level that is no longer the page's is closed by
	// the session.
	if opened, ok := msg.(agentLogOpenedMsg); ok {
		p.read, out, took = p.read.keep(opened.stream, opened.level == p.level)
	} else {
		p.read, out, took = p.read.take(msg)
	}

	return p, out, took
}

// agentLogsKeys: no key for times, every line of the agent has its own.
var agentLogsKeys = append(append(levelKeys(),
	followKey[agentLogsPage](),
	wrapKey[agentLogsPage](),
	saveKey[agentLogsPage](),
	copyTextKey[agentLogsPage](),
), findWordKeys[agentLogsPage]()...)

func (p agentLogsPage) keys(e env) []keyHint { return hintsOf(p, e, agentLogsKeys) }

func (p agentLogsPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, agentLogsKeys, k)
}

// levelKeys are one key per level: the one that is offered is the one
// that goes to the next level, more detail first and then back to the
// quietest.
func levelKeys() []pageKey[agentLogsPage] {
	keys := make([]pageKey[agentLogsPage], 0, len(nomad.AgentLevels))

	for i, level := range nomad.AgentLevels {
		next := nomad.AgentLevels[(i+1)%len(nomad.AgentLevels)]

		keys = append(keys, pageKey[agentLogsPage]{
			press:   "v",
			label:   strings.ToUpper(next[:1]) + next[1:] + " Level",
			offered: func(p agentLogsPage, _ env) bool { return p.level == level },
			do: func(p agentLogsPage, _ env) (agentLogsPage, outcome) {
				p.level = next

				return p, then(reopenMsg{})
			},
		})
	}

	return keys
}

// nodeAgentLogs follows the agent of the client under the cursor.
func nodeAgentLogs(p nodesPage, e env) (nodesPage, outcome) {
	node, ok := pickedFrom(e, p.visible(e))
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg{agentLogsOf(nomad.AgentTarget{NodeID: node.ID}, node.Name)})
}

// clientAgentLogs follows the agent of the open client.
func clientAgentLogs(p clientPage, _ env) (clientPage, outcome) {
	return p, then(openMsg{agentLogsOf(nomad.AgentTarget{NodeID: p.nodeID}, p.name)})
}

// onServerAgent offers a key only while urga talks to a server: a client
// agent ignores the server a log is asked for and sends its own.
func onServerAgent[P any](_ P, e env) bool { return e.agentServer }

// serversAgentLogs follows the agent of the server under the cursor.
func serversAgentLogs(p serversPage, e env) (serversPage, outcome) {
	server, ok := pickedFrom(e, p.visible(e))
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg{agentLogsOf(nomad.AgentTarget{ServerID: server.Name}, "")})
}

// serverAgentLogs follows the agent of the open server.
func serverAgentLogs(p serverPage, _ env) (serverPage, outcome) {
	return p, then(openMsg{agentLogsOf(nomad.AgentTarget{ServerID: p.server.Name}, "")})
}
