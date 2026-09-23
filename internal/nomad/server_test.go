package nomad_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func serversServer(t *testing.T, members, leader string) *nomad.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if req.URL.Path == "/v1/status/leader" {
			_, _ = w.Write([]byte(leader))

			return
		}

		_, _ = w.Write([]byte(members))
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	require.NoError(t, err)

	return client
}

const members = `{
	"ServerName": "server-01",
	"ServerRegion": "global",
	"Members": [
		{
			"Name": "server-01.global",
			"Addr": "10.0.0.5",
			"Port": 4648,
			"Status": "alive",
			"Tags": {"region": "global", "dc": "dc1", "build": "1.11.1", "port": "4647", "role": "nomad"}
		},
		{
			"Name": "server-02.global",
			"Addr": "10.0.0.6",
			"Port": 4648,
			"Status": "alive",
			"ProtocolCur": 2,
			"ProtocolMin": 1,
			"ProtocolMax": 5,
			"Tags": {"region": "global", "dc": "dc1", "build": "1.11.1", "port": "4647", "role": "nomad",
				"id": "9f3b6e04-8e2f-4f2b-9d5f-2f4a1c0b7e11", "raft_vsn": "3", "expect": "3",
				"rpc_addr": "10.0.0.6"}
		}
	]
}`

func TestServers_Read(t *testing.T) {
	r := require.New(t)

	client := serversServer(t, members, `"10.0.0.6:4647"`)

	servers, err := client.Servers(context.Background())
	r.NoError(err)
	r.Len(servers, 2)

	first := servers[0]
	r.Equal("server-01.global", first.Name)
	r.Equal("10.0.0.5", first.Address)
	r.Equal("dc1", first.Datacenter)
	r.Equal("global", first.Region)
	r.Equal("1.11.1", first.Version)
	r.Equal("alive", first.Status)

	// The cluster has one leader, and the list says which one it is.
	r.False(first.Leader)
	r.True(servers[1].Leader)
}

func TestServers_TheLeaderBehindAnAdvertisedAddress(t *testing.T) {
	r := require.New(t)

	// The agent gossips on one address and takes calls on another, which is
	// what an advertise block does. The cluster names its leader by the
	// address it takes calls on.
	members := `{"Members": [{
		"Name": "server-01.global",
		"Addr": "10.0.0.5",
		"Port": 4648,
		"Status": "alive",
		"Tags": {"rpc_addr": "192.168.1.5", "port": "4647", "dc": "dc1"}
	}]}`

	client := serversServer(t, members, `"192.168.1.5:4647"`)

	servers, err := client.Servers(context.Background())
	r.NoError(err)
	r.Len(servers, 1)

	r.True(servers[0].Leader)
	r.Equal("192.168.1.5:4647", servers[0].RPCAddress)
}

func TestServers_TakeTheContext(t *testing.T) {
	r := require.New(t)

	client := serversServer(t, members, `""`)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// The list of servers is a request like any other: it stops when the
	// screen that asked for it is gone.
	_, err := client.Servers(ctx)
	r.Error(err)
}

func TestServers_WithoutALeader(t *testing.T) {
	r := require.New(t)

	// An election is running, or the address cannot be read. The list still
	// shows the servers, with nobody marked.
	client := serversServer(t, members, `""`)

	servers, err := client.Servers(context.Background())
	r.NoError(err)
	r.Len(servers, 2)

	for _, server := range servers {
		r.False(server.Leader)
	}
}

func TestServer_ReadsOneByName(t *testing.T) {
	r := require.New(t)

	client := serversServer(t, members, `"10.0.0.6:4647"`)

	server, err := client.Server(context.Background(), "server-02.global")
	r.NoError(err)

	r.Equal("server-02.global", server.Name)
	r.True(server.Leader)

	// Everything the agent gossips about itself is kept, not only what the
	// list shows: the detail of a server is its tags.
	r.Equal("3", server.Tags["raft_vsn"])
	r.Equal("nomad", server.Tags["role"])

	// What the members speak to each other.
	r.Equal(2, server.Protocol)
	r.Equal(1, server.ProtocolMin)
	r.Equal(5, server.ProtocolMax)
}

func TestServer_ThatIsNotThere(t *testing.T) {
	r := require.New(t)

	client := serversServer(t, members, `""`)

	_, err := client.Server(context.Background(), "server-09.global")
	r.ErrorIs(err, nomad.ErrNoServer)
}

func TestRaftPeers_Read(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{
		"Index": 22,
		"Servers": [
			{"ID": "9f3b6e04", "Node": "server-01.global", "Address": "10.0.0.5:4647", "Leader": false, "Voter": true, "RaftProtocol": "3"},
			{"ID": "1a2b3c4d", "Node": "server-02.global", "Address": "10.0.0.6:4647", "Leader": true, "Voter": true, "RaftProtocol": "3"}
		]
	}`)

	peers, err := client.RaftPeers(context.Background())
	r.NoError(err)
	r.Len(peers, 2)

	r.Equal("/v1/operator/raft/configuration", asked.URL.Path)

	r.Equal("server-02.global", peers[1].Node)
	r.Equal("10.0.0.6:4647", peers[1].Address)
	r.True(peers[1].Leader)
	r.True(peers[1].Voter)
	r.Equal("3", peers[1].Protocol)
}
