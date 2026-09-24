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

// copyField puts the value of the field under the cursor on the clipboard,
// which is how an address or an id gets out of the screen and into a command
// somewhere else. It goes over OSC52, so it works through ssh as well.
func copyField(m Model) (Model, tea.Cmd) {
	if !m.screen.of().fields {
		return m, nil
	}

	row, ok := m.table.selected()
	if !ok || len(row.cells) < 2 {
		return m, nil
	}

	// The value is the second column on every screen of fields; a screen
	// may carry more after it, like where the value came from.
	field, value := row.cells[0], row.cells[1]

	return m.say(sprintf("Copied %s.", field)), tea.SetClipboard(value)
}

// fetchRaft asks what the raft of the cluster makes of its servers. An ACL
// may hold the answer back, which is not an error of the screen: the reason
// takes the place of the answer.
func fetchRaft(client Client) tea.Cmd {
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

// openServer opens what the agent of the server under the cursor says about
// itself.
func openServer(m Model) (Model, tea.Cmd) {
	server, ok := selectedOf(m, screenServers, m.servers)
	if !ok {
		return m, nil
	}

	m.server, m.raft, m.raftErr = server, nil, nil

	return m.push(screen{kind: screenServer, label: server.Name})
}
