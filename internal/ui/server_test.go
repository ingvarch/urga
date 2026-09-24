package ui

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func twoServers() []nomad.Server {
	return []nomad.Server{
		{Name: "server-01.global", Address: "10.0.0.5", Port: 4648, Datacenter: "dc1", Region: "global", Version: "1.11.1", Status: "alive"},
		{Name: "server-02.global", Address: "10.0.0.6", Port: 4648, Datacenter: "dc1", Region: "global", Version: "1.11.1", Status: "alive", Leader: true},
	}
}

func TestServers_Screen(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{servers: twoServers()}

	m := newTestModel(client)
	m, _ = m.update(key(':'))
	m = typeIn(m, "servers")
	m, cmd := m.update(enter())

	r.Equal(screenServers, m.screen.kind)

	m = drain(m, cmd)

	out := plain(m.render())
	r.Contains(out, "Servers [2]")
	r.Contains(out, "server-01.global")
	r.Contains(out, "10.0.0.5")
	r.Contains(out, "dc1")
	r.Contains(out, "1.11.1")

	// Which one leads is the first thing you look for on that screen.
	r.Contains(out, "leader")
}

func TestServers_LeaderStandsOut(t *testing.T) {
	r := require.New(t)

	rows := serverRows(twoServers())

	r.Len(rows, 2)
	r.Equal("", rows[0].cells[len(rows[0].cells)-1])
	r.Equal("leader", rows[1].cells[len(rows[1].cells)-1])
	r.Equal(colorTitle, rows[1].color)
}

func TestServers_AServerThatIsNotAlive(t *testing.T) {
	r := require.New(t)

	rows := serverRows([]nomad.Server{{Name: "server-03.global", Status: "failed"}})

	// A server that is not alive is the one to look at.
	r.Equal(colorDead, rows[0].color)
}

func TestClients_IsWhatNomadCallsTheNodes(t *testing.T) {
	r := require.New(t)

	nodes := readyNode()
	client := &fakeClient{nodes: nodes}

	// Nomad calls them clients, the command line takes either word.
	for _, word := range []string{"clients", "nodes"} {
		m := newTestModel(client)
		m, _ = m.update(key(':'))
		m = typeIn(m, word)
		m, _ = m.update(enter())
		m, _ = m.update(nodesMsg(nodes))

		r.Equal(screenNodes, m.screen.kind, word)
		r.Contains(plain(m.render()), "Clients [1]", word)
	}
}

func aServer() nomad.Server {
	return nomad.Server{
		Name: "server-02.global", Address: "10.0.0.6", Port: 4648,
		Datacenter: "dc1", Region: "global", Version: "1.11.1", Status: "alive", Leader: true,
		Tags: map[string]string{
			"dc": "dc1", "region": "global", "build": "1.11.1", "port": "4647",
			"id": "9f3b6e04-8e2f-4f2b-9d5f-2f4a1c0b7e11", "role": "nomad",
			"expect": "3", "raft_vsn": "3", "rpc_addr": "10.0.0.6",
		},
		Protocol: 2, ProtocolMin: 1, ProtocolMax: 5,
	}
}

// serverScreen drills into the leader of the cluster.
func serverScreen(t *testing.T) (Model, *fakeClient) {
	t.Helper()

	client := &fakeClient{servers: twoServers(), server: aServer()}

	m := newTestModel(client)
	m, _ = m.update(key(':'))
	m = typeIn(m, "servers")
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	m, _ = m.update(key('j'))

	m, cmd = m.update(enter())

	return drain(m, cmd), client
}

func TestServer_OpensWhatTheAgentSaysAboutItself(t *testing.T) {
	r := require.New(t)

	m, client := serverScreen(t)

	r.Equal(screenServer, m.screen.kind)
	r.Equal("server-02.global", client.askedServer)

	out := plain(m.render())
	r.Contains(out, "Server server-02.global")

	// What the Nomad interface shows about a server, in the order it reads.
	r.Contains(out, "Status")
	r.Contains(out, "alive")
	r.Contains(out, "Leader")
	r.Contains(out, "Address")
	r.Contains(out, "10.0.0.6")
	r.Contains(out, "Datacenter")
	r.Contains(out, "Region")
	r.Contains(out, "Version")
	r.Contains(out, "1.11.1")

	// And the gossip it speaks, which the interface does not show.
	r.Contains(out, "Gossip")
	r.Contains(out, "2")
}

func TestServer_ShowsEveryTagTheAgentCarries(t *testing.T) {
	r := require.New(t)

	m, _ := serverScreen(t)

	out := plain(m.render())

	// A tag that no field above stands for is worth a row of its own: it is
	// how the cluster was built.
	r.Contains(out, "expect")
	r.Contains(out, "role")

	// A tag a field already says is not repeated.
	r.NotContains(out, "build")
}

func TestServer_SaysWhatRaftMakesOfIt(t *testing.T) {
	r := require.New(t)

	m, _ := serverScreen(t)

	m, _ = m.update(raftMsg{peers: []nomad.RaftPeer{
		{Node: "server-02.global", Address: "10.0.0.6:4647", Leader: true, Voter: true, Protocol: "3"},
	}})

	out := plain(m.render())

	// Alive in the gossip and voting in the raft are two different things.
	r.Contains(out, "Raft")
	r.Contains(out, "voter")
	r.Contains(out, "3")
}

func TestServer_WhenRaftIsNotAllowed(t *testing.T) {
	r := require.New(t)

	m, _ := serverScreen(t)

	m, _ = m.update(raftMsg{err: errTest})

	out := plain(m.render())

	// The one thing an ACL is likely to hold back says why, in its own row,
	// and the rest of the screen stands.
	r.Contains(out, "Raft")
	r.Contains(out, "no answer")
	r.Contains(out, "server-02.global")

	// It is not an error over the whole screen.
	r.NotEqual(flashErr, m.flash.level)
}

func TestServer_CopiesTheValueUnderTheCursor(t *testing.T) {
	r := require.New(t)

	m, _ := serverScreen(t)

	// Down to Address, which is the kind of thing one copies.
	for range 3 {
		m, _ = m.update(key('j'))
	}

	next, cmd := m.update(key('c'))
	r.NotNil(cmd)

	// The value goes to the clipboard, not the name of the field and not
	// the whole row.
	r.Equal("10.0.0.6", clipboardOf(cmd))
	r.Contains(plain(next.render()), "Copied Address")
}

func TestServer_CopyingWithNothingUnderTheCursor(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{servers: twoServers(), server: aServer()}

	m := newTestModel(client)
	m, _ = m.update(key(':'))
	m = typeIn(m, "servers")
	m, _ = m.update(enter())

	// An empty screen has nothing to copy and says nothing.
	next, cmd := m.update(key('c'))
	r.Nil(cmd)
	r.Empty(next.flash.text)
}

func TestServer_AnAnswerClearsTheErrorAndAsksAgain(t *testing.T) {
	r := require.New(t)

	m, _ := serverScreen(t)

	m, _ = m.update(errMsg{err: errTest})
	m, _ = m.update(pollMsg{})

	m, cmd := m.update(serverMsg(aServer()))

	// The server answered, so what went wrong is over, and the next ask is
	// on its way.
	r.NotContains(plain(m.render()), "no answer")
	r.NotNil(cmd)
	r.IsType(pollMsg{}, cmd())
}

func TestServer_AnAnswerAfterTheServerWasLeftIsDropped(t *testing.T) {
	r := require.New(t)

	m, _ := serverScreen(t)

	m, _ = m.update(escape())
	m, _ = m.update(pollMsg{})

	m, cmd := m.update(serverMsg(nomad.Server{Name: "server-09.global"}))

	r.Nil(cmd)
	r.Equal("server-02.global", m.server.Name)
}
