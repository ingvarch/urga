package nomad_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

const nodeDetail = `{
	"ID": "node-1",
	"Name": "nomad-server-01",
	"Status": "ready",
	"Attributes": {"cpu.arch": "amd64", "os.name": "ubuntu", "driver.docker": "1"},
	"Meta": {"role": "bot", "owner": "igor"},
	"Events": [
		{"Message": "Node registered", "Subsystem": "Cluster", "Timestamp": "2026-09-01T10:00:00Z"},
		{"Message": "Driver docker is healthy", "Subsystem": "Driver", "Timestamp": "2026-09-23T09:00:00Z", "Details": {"driver": "docker"}}
	],
	"Drivers": {
		"docker": {
			"Detected": true,
			"Healthy": true,
			"HealthDescription": "Healthy",
			"UpdateTime": "2026-09-23T09:00:00Z",
			"Attributes": {"driver.docker.version": "27.1.1", "driver.docker.runtimes": "runc"}
		},
		"exec": {"Detected": false, "Healthy": false, "HealthDescription": "Driver must run as root"}
	},
	"HostVolumes": {
		"certs": {"Path": "/etc/ssl/certs", "ReadOnly": true},
		"data": {"Path": "/srv/data"}
	}
}`

func TestNodeDetail_Read(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, nodeDetail)

	detail, err := client.NodeDetail(context.Background(), "node-1")
	r.NoError(err)

	r.Equal("/v1/node/node-1", asked.URL.Path)
	r.Equal("nomad-server-01", detail.Name)

	// The newest event is the one worth reading, so it comes first.
	r.Len(detail.Events, 2)
	r.Equal("Driver docker is healthy", detail.Events[0].Message)
	r.Equal("Driver", detail.Events[0].Subsystem)
	r.Equal("docker", detail.Events[0].Details["driver"])

	// The drivers come out of a map, so they are put in the order of their
	// names: the list would shuffle under the cursor otherwise.
	r.Len(detail.Drivers, 2)
	r.Equal("docker", detail.Drivers[0].Name)
	r.True(detail.Drivers[0].Detected)
	r.True(detail.Drivers[0].Healthy)
	r.Equal("27.1.1", detail.Drivers[0].Attributes["driver.docker.version"])

	r.Equal("exec", detail.Drivers[1].Name)
	r.False(detail.Drivers[1].Detected)
	r.Equal("Driver must run as root", detail.Drivers[1].Description)

	r.Len(detail.Volumes, 2)
	r.Equal("certs", detail.Volumes[0].Name)
	r.Equal("/etc/ssl/certs", detail.Volumes[0].Path)
	r.True(detail.Volumes[0].ReadOnly)
	r.False(detail.Volumes[1].ReadOnly)

	r.Equal("amd64", detail.Attributes["cpu.arch"])
}

func TestNode_ReadsTheMachineItself(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{
		"ID": "node-1",
		"Name": "nomad-server-01",
		"Datacenter": "dc1",
		"NodePool": "default",
		"Status": "down",
		"SchedulingEligibility": "ineligible",
		"Drain": true,
		"NodeResources": {"Cpu": {"CpuShares": 4000}, "Memory": {"MemoryMB": 3820}}
	}`)

	node, err := client.Node(context.Background(), "node-1")
	r.NoError(err)

	// The list fields of one client are read from that client, so a screen
	// open on it does not keep showing old values.
	r.Equal("/v1/node/node-1", asked.URL.Path)
	r.Equal("nomad-server-01", node.Name)
	r.Equal("down", node.Status)
	r.True(node.Drain)
	r.Equal(4000, node.CPUShares)
}

// metaServer answers with the given body and records the request and its
// body: a call sends some of its data in the body and some in the query.
func metaServer(t *testing.T, body string) (*nomad.Client, *[]byte, *http.Request) {
	t.Helper()

	sent := &[]byte{}
	asked := &http.Request{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		*asked = *req

		if req.Method == http.MethodPost || req.Method == http.MethodPut {
			*sent, _ = io.ReadAll(req.Body)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	require.NoError(t, err)

	return client, sent, asked
}

const nodeMeta = `{
	"Meta": {"role": "bot", "owner": "igor"},
	"Dynamic": {"owner": "igor"},
	"Static": {"role": "bot"}
}`

func TestNodeMeta_SaysWhereEachKeyComesFrom(t *testing.T) {
	r := require.New(t)

	client, _, _ := metaServer(t, nodeMeta)

	meta, err := client.NodeMeta(context.Background(), "node-1")
	r.NoError(err)
	r.Len(meta, 2)

	// Sorted by key, and each says whether the API set it or the agent
	// configuration did: only one of the two can be changed from here.
	r.Equal("owner", meta[0].Key)
	r.Equal("igor", meta[0].Value)
	r.True(meta[0].Dynamic)

	r.Equal("role", meta[1].Key)
	r.False(meta[1].Dynamic)
}

func TestNodeMetaSpec_IsWhatCanBeChanged(t *testing.T) {
	r := require.New(t)

	client, _, _ := metaServer(t, nodeMeta)

	spec, err := client.NodeMetaSpec(context.Background(), "node-1")
	r.NoError(err)

	// The file holds what the API can set, not what the agent was started
	// with: editing a static key would change nothing.
	r.Contains(spec, "owner")
	r.NotContains(spec, "role")
}

func TestSubmitNodeMeta_SendsWhatChanged(t *testing.T) {
	r := require.New(t)

	client, sent, _ := metaServer(t, nodeMeta)

	err := client.SubmitNodeMeta(context.Background(), "node-1", `{"team": "pelmeni"}`)
	r.NoError(err)

	request := struct {
		NodeID string
		Meta   map[string]*string
	}{}
	r.NoError(json.Unmarshal(*sent, &request))

	r.Equal("node-1", request.NodeID)

	// The key that was added is set, and the one that was taken out of the
	// file is removed rather than left behind.
	r.Equal("pelmeni", *request.Meta["team"])

	value, ok := request.Meta["owner"]
	r.True(ok, "the key that was dropped is sent as removed")
	r.Nil(value)
}
