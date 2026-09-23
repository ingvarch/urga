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
