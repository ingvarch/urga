package nomad_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestInRegion_AsksInThatRegion(t *testing.T) {
	t.Setenv("NOMAD_REGION", "")

	r := require.New(t)

	client, asked := recorder(t, `[]`)

	_, err := client.InRegion("eu").Jobs(context.Background(), "default")
	r.NoError(err)

	// The region is sent with the request, like the namespace. The cluster
	// forwards it to the servers of that region.
	r.Equal("eu", asked.URL.Query().Get("region"))
}

func TestInRegion_WritesInThatRegion(t *testing.T) {
	t.Setenv("NOMAD_REGION", "")

	r := require.New(t)

	client, asked := recorder(t, `{"EvalID": "eval-1"}`)

	r.NoError(client.InRegion("eu").StopJob(context.Background(), "production", "web"))

	r.Equal(http.MethodDelete, asked.Method)
	r.Equal("eu", asked.URL.Query().Get("region"))
}

func TestInRegion_LeavesTheClientItCameFrom(t *testing.T) {
	t.Setenv("NOMAD_REGION", "")

	r := require.New(t)

	client, asked := recorder(t, `[]`)

	eu := client.InRegion("eu")
	r.Equal("eu", eu.Region())

	// A request still running when the session switches keeps the region
	// it was sent in.
	_, err := client.Jobs(context.Background(), "default")
	r.NoError(err)

	r.Empty(client.Region())
	r.False(asked.URL.Query().Has("region"))
}

func TestNew_TakesTheRegionOfTheEnvironment(t *testing.T) {
	t.Setenv("NOMAD_REGION", "ap")

	r := require.New(t)

	client, asked := recorder(t, `[]`)

	_, err := client.Jobs(context.Background(), "default")
	r.NoError(err)

	// NOMAD_REGION is read the way the nomad command reads it.
	r.Equal("ap", client.Region())
	r.Equal("ap", asked.URL.Query().Get("region"))
}

func TestNew_TakesTheRegionOfTheConfig(t *testing.T) {
	t.Setenv("NOMAD_REGION", "ap")

	r := require.New(t)

	client, err := nomad.New(nomad.Config{Address: "http://127.0.0.1:4646", Region: "us"})
	r.NoError(err)

	// A region asked for on the command line wins over the environment.
	r.Equal("us", client.Region())
}

func TestRegions(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `["us", "eu"]`)

	regions, err := client.Regions(context.Background())
	r.NoError(err)

	r.Equal("/v1/regions", asked.URL.Path)
	r.Equal([]string{"eu", "us"}, regions)
}

func TestDatacenters_AreWhereTheClientsAre(t *testing.T) {
	t.Setenv("NOMAD_REGION", "")

	r := require.New(t)

	client, asked := recorder(t, `[
		{"ID": "n1", "Datacenter": "dc2"},
		{"ID": "n2", "Datacenter": "dc1"},
		{"ID": "n3", "Datacenter": "dc2"}
	]`)

	datacenters, err := client.InRegion("eu").Datacenters(context.Background())
	r.NoError(err)

	// Every datacenter once, sorted.
	r.Equal("/v1/nodes", asked.URL.Path)
	r.Equal("eu", asked.URL.Query().Get("region"))
	r.Equal([]string{"dc1", "dc2"}, datacenters)
}

func TestJob_RunsIn(t *testing.T) {
	tests := []struct {
		name        string
		datacenters []string
		datacenter  string
		runs        bool
	}{
		{name: "named", datacenters: []string{"dc1"}, datacenter: "dc1", runs: true},
		{name: "not named", datacenters: []string{"dc1"}, datacenter: "dc2", runs: false},
		{name: "one of several", datacenters: []string{"dc1", "dc3"}, datacenter: "dc3", runs: true},

		// Nomad takes a star for any run of letters, and a job that names
		// no datacenter at all is placed in every one.
		{name: "every datacenter", datacenters: []string{"*"}, datacenter: "eu-west", runs: true},
		{name: "a prefix", datacenters: []string{"eu-*"}, datacenter: "eu-west", runs: true},
		{name: "another prefix", datacenters: []string{"eu-*"}, datacenter: "us-east", runs: false},
		{name: "a suffix", datacenters: []string{"*-west"}, datacenter: "eu-west", runs: true},
		{name: "a middle", datacenters: []string{"eu-*-a"}, datacenter: "eu-west-a", runs: true},
		{name: "another middle", datacenters: []string{"eu-*-a"}, datacenter: "eu-west-b", runs: false},
		{name: "none named", datacenters: nil, datacenter: "dc1", runs: true},

		// No datacenter chosen means every one of them.
		{name: "no choice", datacenters: []string{"dc1"}, datacenter: "", runs: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			job := nomad.Job{ID: "web", Datacenters: test.datacenters}

			require.Equal(t, test.runs, job.RunsIn(test.datacenter))
		})
	}
}

func TestJobs_ReadWhereTheyRun(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `[{"ID": "web", "Datacenters": ["dc1", "eu-*"]}]`)

	jobs, err := client.Jobs(context.Background(), "default")
	r.NoError(err)

	r.Equal([]string{"dc1", "eu-*"}, jobs[0].Datacenters)
}

func TestServers_MarkTheLeaderOfTheRegion(t *testing.T) {
	t.Setenv("NOMAD_REGION", "")

	r := require.New(t)

	leaderAsked := ""

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if req.URL.Path == "/v1/status/leader" {
			leaderAsked = req.URL.Query().Get("region")
			_, _ = w.Write([]byte(`"10.0.0.6:4647"`))

			return
		}

		_, _ = w.Write([]byte(members))
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	r.NoError(err)

	_, err = client.InRegion("eu").Servers(context.Background())
	r.NoError(err)

	// Every region has its own leader, so the leader is requested in the
	// region in use.
	r.Equal("eu", leaderAsked)
}

// federation is a member list of two regions: the agent answering is a
// server of global, and it lists the servers of eu as well.
const federation = `{
	"ServerName": "server-01",
	"ServerRegion": "global",
	"Members": [
		{"Name": "server-01.global", "Addr": "10.0.0.5", "Status": "alive", "Tags": {"region": "global", "dc": "dc1", "port": "4647"}},
		{"Name": "server-01.eu", "Addr": "10.1.0.5", "Status": "alive", "Tags": {"region": "eu", "dc": "dc1", "port": "4647"}}
	]
}`

func TestServers_AreThoseOfTheRegionAskedIn(t *testing.T) {
	r := require.New(t)

	client := serversServer(t, federation, `""`)

	servers, err := client.InRegion("eu").Servers(context.Background())
	r.NoError(err)

	// The gossip spans every region; the list keeps the region in use.
	r.Len(servers, 1)
	r.Equal("server-01.eu", servers[0].Name)
}

func TestServers_WithoutARegionAreThoseOfTheAgent(t *testing.T) {
	r := require.New(t)

	client := serversServer(t, federation, `""`)

	servers, err := client.Servers(context.Background())
	r.NoError(err)

	// A client that names no region gets the servers of the agent's region,
	// which the same response reports.
	r.Len(servers, 1)
	r.Equal("server-01.global", servers[0].Name)
}
