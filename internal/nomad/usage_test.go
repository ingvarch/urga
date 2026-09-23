package nomad_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestAllocationUsage(t *testing.T) {
	r := require.New(t)

	client := statsServer(t, `{
		"ResourceUsage": {
			"CpuStats": {"TotalTicks": 125},
			"MemoryStats": {"RSS": 134217728}
		}
	}`)

	use, err := client.AllocationUsage(context.Background(), "production", "af1f37df")
	r.NoError(err)

	// What it takes, and how much of what it asked for that is: 125 of 500
	// ticks, 128 of 256 megabytes.
	r.Equal(125, use.CPUTicks)
	r.Equal(500, use.CPUTicksAllowed)
	r.Equal(25, use.CPUPercent)

	r.Equal(128, use.MemoryMB)
	r.Equal(256, use.MemoryMBAllowed)
	r.Equal(50, use.MemoryPercent)
}

func TestNodeUsage(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{
		"CPUTicksConsumed": 1500,
		"Memory": {"Used": 4294967296, "Total": 8589934592},
		"CPU": [{"Idle": 60}, {"Idle": 40}]
	}`)

	use, err := client.NodeUsage(context.Background(), "node-1")
	r.NoError(err)

	r.Equal("/v1/client/stats", asked.URL.Path)
	r.Equal("node-1", asked.URL.Query().Get("node_id"))

	// The cores together: one 40% busy and one 60% busy, and the memory of
	// the machine.
	r.Equal(50, use.CPUPercent)

	// What the machine is doing in ticks, which reads next to the capacity
	// the node list carries.
	r.Equal(1500, use.CPUTicks)
	r.Equal(50, use.MemoryPercent)
	r.Equal(4096, use.MemoryMB)
	r.Equal(8192, use.MemoryMBAllowed)
}

// statsServer answers the allocation and its stats, which is what a reading
// takes.
func statsServer(t *testing.T, stats string) *nomad.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if req.URL.Path == "/v1/allocation/af1f37df" {
			_, _ = w.Write([]byte(`{
				"ID": "af1f37df",
				"AllocatedResources": {"Tasks": {"web": {"Cpu": {"CpuShares": 500}, "Memory": {"MemoryMB": 256}}}}
			}`))

			return
		}

		_, _ = w.Write([]byte(stats))
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	require.NoError(t, err)

	return client
}

func TestAllocationUsage_MemoryWithoutRSS(t *testing.T) {
	r := require.New(t)

	// On cgroups v2 Nomad fills Usage and leaves RSS at zero. Reading RSS
	// alone shows a running task as taking no memory at all.
	client := statsServer(t, `{
		"ResourceUsage": {
			"CpuStats": {"TotalTicks": 50},
			"MemoryStats": {"RSS": 0, "Usage": 201326592, "Measured": ["Cache", "Swap", "Usage"]}
		}
	}`)

	use, err := client.AllocationUsage(context.Background(), "production", "af1f37df")
	r.NoError(err)

	r.Equal(192, use.MemoryMB)
	r.Equal(75, use.MemoryPercent)
}

func TestAllocationUsage_OnlyTheTasksReport(t *testing.T) {
	r := require.New(t)

	// Some drivers leave the summary of the allocation empty and report per
	// task. Adding the tasks up is what the summary would have said.
	client := statsServer(t, `{
		"Tasks": {
			"web": {"ResourceUsage": {"CpuStats": {"TotalTicks": 100}, "MemoryStats": {"RSS": 67108864}}},
			"sidecar": {"ResourceUsage": {"CpuStats": {"TotalTicks": 25}, "MemoryStats": {"RSS": 67108864}}}
		}
	}`)

	use, err := client.AllocationUsage(context.Background(), "production", "af1f37df")
	r.NoError(err)

	r.Equal(125, use.CPUTicks)
	r.Equal(128, use.MemoryMB)
}
