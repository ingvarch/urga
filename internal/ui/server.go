package ui

import (
	"context"
	"fmt"
	"image/color"
	"sort"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// fieldTitles are the columns of a screen that reads as a list of fields
// rather than as a list of resources.
var fieldTitles = []string{"Field", "Value"}

// serverTags are the tags a field of the detail already stands for. The rest
// of them are shown as they come: they say how the cluster was built.
var serverTags = map[string]bool{
	"dc": true, "region": true, "build": true, "port": true, "rpc_addr": true, "id": true,
}

// fetchRaft asks what the raft of the cluster makes of its servers. An ACL
// may hold the answer back, which is not an error of the screen: the reason
// takes the place of the answer.
func fetchRaft(client serversClient) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		peers, err := client.RaftPeers(ctx)

		return raftMsg{peers: peers, err: err}
	}
}

// serverDetailRows are everything the cluster says about one server: what
// the Nomad interface shows, what the raft adds, and every tag the agent
// carries that no field above stands for.
func serverDetailRows(server nomad.Server, peers []nomad.RaftPeer, failed error) []tableRow {
	peer, known := peerOf(server, peers)

	rows := []fieldRow{
		{"Name", server.Name, nil},
		{"Status", server.Status, serverColor(server)},
		{"Leader", yesNo(server.Leader), leaderColor(server.Leader)},
		{"Address", server.Address, nil},
		{"Serf port", port(server.Port), nil},
		{"RPC address", server.RPCAddress, nil},
		{"Datacenter", server.Datacenter, nil},
		{"Region", server.Region, nil},
		{"Version", server.Version, nil},
		{"Node ID", server.Tags["id"], nil},
		{"Raft", raftStanding(peer, known, failed), raftColor(peer, known, failed)},
		{"Raft protocol", peer.Protocol, nil},
		{"Gossip protocol", gossip(server), nil},
	}

	for _, name := range otherTags(server.Tags) {
		rows = append(rows, fieldRow{"tag " + name, server.Tags[name], nil})
	}

	out := make([]tableRow, 0, len(rows))

	for _, row := range rows {
		if row.value == "" {
			continue
		}

		out = append(out, tableRow{cells: []string{row.field, row.value}, color: row.color})
	}

	return out
}

// fieldRow is one line of a detail before it becomes a row of the table.
type fieldRow struct {
	field string
	value string
	color color.Color
}

// peerOf is the server as the raft knows it, by the name it goes by there,
// or by the address it talks on.
func peerOf(server nomad.Server, peers []nomad.RaftPeer) (nomad.RaftPeer, bool) {
	for _, peer := range peers {
		if peer.Node == server.Name || peer.Address == server.RPCAddress {
			return peer, true
		}
	}

	return nomad.RaftPeer{}, false
}

// raftStanding is what the raft makes of the server: whether it votes, or
// why that is not known.
func raftStanding(peer nomad.RaftPeer, known bool, failed error) string {
	if failed != nil {
		return "unknown: " + failed.Error()
	}

	if !known {
		return "not in the raft configuration"
	}

	if peer.Voter {
		return "voter"
	}

	return "non-voter"
}

func raftColor(peer nomad.RaftPeer, known bool, failed error) color.Color {
	switch {
	case failed != nil:
		return colorMuted
	case !known, !peer.Voter:
		return colorAttention
	}

	return nil
}

func leaderColor(leader bool) color.Color {
	if leader {
		return colorTitle
	}

	return nil
}

// gossip is the version of the protocol the members speak to each other, and
// the range this one can speak.
func gossip(server nomad.Server) string {
	if server.Protocol == 0 {
		return ""
	}

	return fmt.Sprintf("%d (%d to %d)", server.Protocol, server.ProtocolMin, server.ProtocolMax)
}

// otherTags are the tags no field of the detail stands for, in a settled
// order: a map hands them over differently every time.
func otherTags(tags map[string]string) []string {
	names := make([]string, 0, len(tags))

	for name := range tags {
		if !serverTags[name] {
			names = append(names, name)
		}
	}

	sort.Strings(names)

	return names
}

// port is a port as it reads in a field, empty when there is none.
func port(number int) string {
	if number == 0 {
		return ""
	}

	return strconv.Itoa(number)
}

func yesNo(yes bool) string {
	if yes {
		return "yes"
	}

	return "no"
}

// serversPage is the servers of the region in use.
type serversPage struct {
	ofTheSession

	servers []nomad.Server
}

func (serversPage) title(_ env, count int) string { return sprintf("Servers [%d]", count) }
func (serversPage) titles() []string              { return serverTitles }
func (serversPage) topics() []string              { return nil }

func (serversPage) fetch(e env) tea.Cmd {
	return fetchList(e.client.Servers, func(items []nomad.Server) tea.Msg { return serversMsg(items) })
}

func (p serversPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	servers, ok := msg.(serversMsg)
	if !ok {
		return p, outcome{}, false
	}

	p.servers = servers

	return p, outcome{}, true
}

// visible are the servers of the datacenter the session is narrowed to. The
// rows and the keys that find a row by its place read the same list, so a
// key finds the server on the screen.
func (p serversPage) visible(e env) []nomad.Server { return serversIn(e.datacenter, p.servers) }

func (p serversPage) rows(e env) []tableRow { return serverRows(p.visible(e)) }

var serversKeys = []pageKey[serversPage]{{press: "enter", label: "Details", do: openServer}}

func (p serversPage) keys(e env) []keyHint { return hintsOf(p, e, serversKeys) }

func (p serversPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, serversKeys, k)
}

// openServer opens what the agent of the server under the cursor says about
// itself.
func openServer(p serversPage, e env) (serversPage, outcome) {
	server, ok := pickedFrom(e, p.visible(e))
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg(screen{kind: screenServer, page: serverPage{server: server}}))
}

// serverPage is what the agent of one server says about itself, and what
// the raft of the cluster makes of it.
type serverPage struct {
	server  nomad.Server
	peers   []nomad.RaftPeer
	raftErr error
}

func (p serverPage) title(env, int) string { return sprintf("Server %s", p.server.Name) }
func (serverPage) titles() []string        { return fieldTitles }
func (serverPage) topics() []string        { return nil }

func (p serverPage) fetch(e env) tea.Cmd {
	client, name := e.client, p.server.Name

	// The agent answers for itself, the raft says whether the rest of the
	// cluster still counts it.
	return tea.Batch(
		request(func(ctx context.Context) (nomad.Server, error) {
			return client.Server(ctx, name)
		}, func(server nomad.Server) tea.Msg { return serverMsg(server) }),
		fetchRaft(client),
	)
}

func (p serverPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	switch msg := msg.(type) {
	case serverMsg:
		p.server = nomad.Server(msg)
	case raftMsg:
		p.peers, p.raftErr = msg.peers, msg.err
	default:
		return p, outcome{}, false
	}

	return p, outcome{}, true
}

func (p serverPage) rows(env) []tableRow { return serverDetailRows(p.server, p.peers, p.raftErr) }

var serverKeys = []pageKey[serverPage]{copyKey[serverPage]()}

func (p serverPage) keys(e env) []keyHint { return hintsOf(p, e, serverKeys) }

func (p serverPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, serverKeys, k)
}
