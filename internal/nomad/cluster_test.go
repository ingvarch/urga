package nomad_test

import (
	"context"
	"fmt"
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

// usageServer answers the two lists the load of the cluster is read from.
func usageServer(t *testing.T, nodes, allocs string) *nomad.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch req.URL.Path {
		case "/v1/nodes":
			_, _ = w.Write([]byte(nodes))
		case "/v1/allocations":
			_, _ = w.Write([]byte(allocs))
		default:
			http.NotFound(w, req)
		}
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	require.NoError(t, err)

	return client
}

func TestClusterUsage_WithoutWhatAClientKeepsForItself(t *testing.T) {
	r := require.New(t)

	client := usageServer(t, `[
		{"ID": "n1", "Status": "ready",
			"NodeResources": {"Cpu": {"CpuShares": 4000}, "Memory": {"MemoryMB": 8000}},
			"ReservedResources": {"Cpu": {"CpuShares": 1000}, "Memory": {"MemoryMB": 2000}}}
	]`, `[
		{"ID": "a1", "ClientStatus": "running", "AllocatedResources": {"Tasks": {"web": {"Cpu": {"CpuShares": 1500}, "Memory": {"MemoryMB": 3000}}}}}
	]`)

	usage, err := client.Usage(context.Background(), "")
	r.NoError(err)

	// The scheduler gives jobs what is left of a client after its own share:
	// 1500 of 3000 shares, 3000 of 6000 megabytes.
	r.Equal(50, usage.CPUPercent)
	r.Equal(50, usage.MemoryPercent)
}

func TestClusterUsage_AClientThatKeepsMoreThanItHas(t *testing.T) {
	r := require.New(t)

	client := usageServer(t, `[
		{"ID": "n1", "Status": "ready",
			"NodeResources": {"Cpu": {"CpuShares": 4000}, "Memory": {"MemoryMB": 8000}}},
		{"ID": "n2", "Status": "ready",
			"NodeResources": {"Cpu": {"CpuShares": 48}, "Memory": {"MemoryMB": 1000}},
			"ReservedResources": {"Cpu": {"CpuShares": 2000}, "Memory": {"MemoryMB": 4000}}}
	]`, `[
		{"ID": "a1", "ClientStatus": "running", "AllocatedResources": {"Tasks": {"web": {"Cpu": {"CpuShares": 1000}, "Memory": {"MemoryMB": 2000}}}}}
	]`)

	usage, err := client.Usage(context.Background(), "")
	r.NoError(err)

	// Such a client has nothing to give, and takes nothing from the others:
	// 1000 of 4000 shares, 2000 of 8000 megabytes.
	r.Equal(25, usage.CPUPercent)
	r.Equal(25, usage.MemoryPercent)
}

func TestClusterUsage_TasksThatDoNotRunTogether(t *testing.T) {
	task := func(cpu, memory int, cores string) string {
		return fmt.Sprintf(`{"Cpu": {"CpuShares": %d, "ReservedCores": %s}, "Memory": {"MemoryMB": %d}}`, cpu, cores, memory)
	}

	tests := []struct {
		name       string
		tasks      string
		lifecycles string
		cpu        int
		memory     int
	}{
		{
			name: "prestart, main and poststop take the largest",
			tasks: `{"init": ` + task(4000, 1000, "null") + `, "web": ` + task(2000, 2000, "null") +
				`, "cleanup": ` + task(500, 6000, "null") + `}`,
			lifecycles: `{"init": {"Hook": "prestart"}, "web": null, "cleanup": {"Hook": "poststop"}}`,
			cpu:        40, memory: 60,
		},
		{
			name: "a prestart sidecar runs with main",
			tasks: `{"init": ` + task(2500, 500, "null") + `, "proxy": ` + task(1000, 1000, "null") +
				`, "web": ` + task(2000, 2000, "null") + `}`,
			lifecycles: `{"init": {"Hook": "prestart"}, "proxy": {"Hook": "prestart", "Sidecar": true}}`,
			cpu:        35, memory: 30,
		},
		{
			name: "a poststart task runs with main",
			tasks: `{"init": ` + task(2500, 2500, "null") + `, "web": ` + task(2000, 2000, "null") +
				`, "after": ` + task(1000, 1000, "null") + `}`,
			lifecycles: `{"init": {"Hook": "prestart"}, "after": {"Hook": "poststart"}}`,
			cpu:        30, memory: 30,
		},
		{
			name:       "reserved cores are added outside the largest",
			tasks:      `{"init": ` + task(3000, 1000, "null") + `, "web": ` + task(2000, 1000, "[0, 1]") + `}`,
			lifecycles: `{"init": {"Hook": "prestart"}}`,
			cpu:        50, memory: 10,
		},
		{
			name:       "an older cluster sends no lifecycles",
			tasks:      `{"web": ` + task(1000, 1000, "null") + `, "worker": ` + task(2000, 2000, "null") + `}`,
			lifecycles: `null`,
			cpu:        30, memory: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := usageOfOneAllocation(t, tt.tasks, tt.lifecycles)

			require.Equal(t, tt.cpu, usage.CPUPercent)
			require.Equal(t, tt.memory, usage.MemoryPercent)
		})
	}
}

// usageOfOneAllocation reads the usage of a cluster of one ready node of
// 10000 shares and 10000 megabytes with one running allocation.
func usageOfOneAllocation(t *testing.T, tasks, lifecycles string) nomad.Usage {
	t.Helper()

	client := usageServer(t, `[
		{"ID": "n1", "Status": "ready",
			"NodeResources": {"Cpu": {"CpuShares": 10000}, "Memory": {"MemoryMB": 10000}}}
	]`, `[
		{"ID": "a1", "ClientStatus": "running", "AllocatedResources": {"Tasks": `+tasks+`, "TaskLifecycles": `+lifecycles+`}}
	]`)

	usage, err := client.Usage(context.Background(), "")
	require.NoError(t, err)

	return usage
}

func TestPercent_RoundsToTheNearestAndIsNothingOfNothing(t *testing.T) {
	r := require.New(t)

	r.Equal(1, nomad.Percent(600, 40960))
	r.Equal(50, nomad.Percent(1000, 2000))
	r.Equal(67, nomad.Percent(2, 3))
	r.Equal(0, nomad.Percent(5, 0))
}
