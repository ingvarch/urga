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
			"Tags": {"region": "global", "dc": "dc1", "build": "1.11.1", "port": "4647", "role": "nomad"}
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
