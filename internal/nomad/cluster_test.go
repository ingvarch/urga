package nomad_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestClusterUsage(t *testing.T) {
	r := require.New(t)

	var askedResources bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch req.URL.Path {
		case "/v1/nodes":
			askedResources = req.URL.Query().Get("resources") == "true"
			_, _ = w.Write([]byte(`[
				{"ID": "n1", "Status": "ready", "NodeResources": {"Cpu": {"CpuShares": 4000}, "Memory": {"MemoryMB": 8000}}},
				{"ID": "n2", "Status": "down", "NodeResources": {"Cpu": {"CpuShares": 4000}, "Memory": {"MemoryMB": 8000}}}
			]`))
		case "/v1/allocations":
			_, _ = w.Write([]byte(`[
				{"ID": "a1", "ClientStatus": "running", "AllocatedResources": {"Tasks": {"web": {"Cpu": {"CpuShares": 1000}, "Memory": {"MemoryMB": 2000}}}}},
				{"ID": "a2", "ClientStatus": "complete", "AllocatedResources": {"Tasks": {"batch": {"Cpu": {"CpuShares": 4000}, "Memory": {"MemoryMB": 4000}}}}}
			]`))
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()

	client, err := nomad.New(nomad.Config{Address: server.URL})
	r.NoError(err)

	usage, err := client.Usage(context.Background(), "")
	r.NoError(err)

	// Only ready nodes count as capacity, and only running allocations use
	// it: 1000 of 4000 shares, 2000 of 8000 megabytes.
	r.True(askedResources)
	r.Equal(25, usage.CPUPercent)
	r.Equal(25, usage.MemoryPercent)
}

func TestClusterUsage_OfOneDatacenter(t *testing.T) {
	r := require.New(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch req.URL.Path {
		case "/v1/nodes":
			_, _ = w.Write([]byte(`[
				{"ID": "n1", "Datacenter": "dc1", "Status": "ready", "NodeResources": {"Cpu": {"CpuShares": 4000}, "Memory": {"MemoryMB": 8000}}},
				{"ID": "n2", "Datacenter": "dc2", "Status": "ready", "NodeResources": {"Cpu": {"CpuShares": 4000}, "Memory": {"MemoryMB": 8000}}}
			]`))
		case "/v1/allocations":
			_, _ = w.Write([]byte(`[
				{"ID": "a1", "NodeID": "n1", "ClientStatus": "running", "AllocatedResources": {"Tasks": {"web": {"Cpu": {"CpuShares": 1000}, "Memory": {"MemoryMB": 2000}}}}},
				{"ID": "a2", "NodeID": "n2", "ClientStatus": "running", "AllocatedResources": {"Tasks": {"web": {"Cpu": {"CpuShares": 3000}, "Memory": {"MemoryMB": 6000}}}}}
			]`))
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()

	client, err := nomad.New(nomad.Config{Address: server.URL})
	r.NoError(err)

	// One datacenter counts only its own nodes and the allocations on them:
	// 1000 of 4000 shares, 2000 of 8000 megabytes.
	usage, err := client.Usage(context.Background(), "dc1")
	r.NoError(err)
	r.Equal(25, usage.CPUPercent)
	r.Equal(25, usage.MemoryPercent)

	// An empty datacenter is the whole region: 4000 of 8000, 8000 of 16000.
	usage, err = client.Usage(context.Background(), "")
	r.NoError(err)
	r.Equal(50, usage.CPUPercent)
	r.Equal(50, usage.MemoryPercent)
}

func TestClusterUsage_EmptyCluster(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `[]`)

	usage, err := client.Usage(context.Background(), "")

	// A cluster with no node ready reports nothing claimed, not a division
	// by zero.
	r.NoError(err)
	r.Zero(usage.CPUPercent)
	r.Zero(usage.MemoryPercent)
}
