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
		},
		"Tasks": {
			"web": {"ResourceUsage": {"CpuStats": {"TotalTicks": 125}, "MemoryStats": {"RSS": 134217728}}}
		}
	}`)

	use, err := client.AllocationUsage(context.Background(), "production", "af1f37df")
	r.NoError(err)

	// What it uses, and how much of what it reserved that is: 125 of 500
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

	// CPU use in ticks, to show next to the capacity from the node list.
	r.Equal(1500, use.CPUTicks)
	r.Equal(50, use.MemoryPercent)
	r.Equal(4096, use.MemoryMB)
	r.Equal(8192, use.MemoryMBAllowed)
}

// webAlloc is an allocation of one task that asked for 500 ticks and 256
// megabytes.
const webAlloc = `{
	"ID": "af1f37df",
	"AllocatedResources": {"Tasks": {"web": {"Cpu": {"CpuShares": 500}, "Memory": {"MemoryMB": 256}}}}
}`

// statsServer answers the two requests a reading makes: the allocation and
// its stats.
func statsServer(t *testing.T, stats string) *nomad.Client {
	t.Helper()

	return allocServer(t, webAlloc, stats)
}

// allocServer answers the same two requests for the allocation it is given.
func allocServer(t *testing.T, alloc, stats string) *nomad.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if req.URL.Path == "/v1/allocation/af1f37df" {
			_, _ = w.Write([]byte(alloc))

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

	// Some versions of Nomad fill Usage on cgroups v2 and leave RSS at
	// zero. Reading RSS alone shows a running task as taking no memory.
	client := statsServer(t, `{
		"ResourceUsage": {
			"CpuStats": {"TotalTicks": 50},
			"MemoryStats": {"RSS": 0, "Usage": 201326592, "Measured": ["Cache", "Swap", "Usage"]}
		},
		"Tasks": {
			"web": {"ResourceUsage": {
				"CpuStats": {"TotalTicks": 50},
				"MemoryStats": {"RSS": 0, "Usage": 201326592, "Measured": ["Cache", "Swap", "Usage"]}
			}}
		}
	}`)

	use, err := client.AllocationUsage(context.Background(), "production", "af1f37df")
	r.NoError(err)

	r.Equal(192, use.MemoryMB)
	r.Equal(75, use.MemoryPercent)
}

func TestAllocationUsage_TasksThatFillDifferentFields(t *testing.T) {
	r := require.New(t)

	// The summary adds up each field apart: its RSS holds only the task that
	// fills RSS, its Usage only the one that fills Usage. Reading one field
	// of it loses the other task.
	client := statsServer(t, `{
		"ResourceUsage": {
			"CpuStats": {"TotalTicks": 125},
			"MemoryStats": {"RSS": 134217728, "Usage": 67108864}
		},
		"Tasks": {
			"web": {"ResourceUsage": {"CpuStats": {"TotalTicks": 100}, "MemoryStats": {"RSS": 134217728}}},
			"sidecar": {"ResourceUsage": {"CpuStats": {"TotalTicks": 25}, "MemoryStats": {"RSS": 0, "Usage": 67108864}}}
		}
	}`)

	use, err := client.AllocationUsage(context.Background(), "production", "af1f37df")
	r.NoError(err)

	r.Equal(125, use.CPUTicks)
	r.Equal(192, use.MemoryMB)
}

func TestAllocationUsage_OnlyTheTasksReport(t *testing.T) {
	r := require.New(t)

	// An answer with an empty summary still holds its tasks, and they are
	// what is added up.
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

func TestAllocationUsage_OnlyTheTasksThatRun(t *testing.T) {
	r := require.New(t)

	// One task ran before the others and has ended, one waits for them to
	// stop. What they asked for is not what the allocation may use now, and
	// the last reading of a task that ended is old.
	client := allocServer(t, `{
		"ID": "af1f37df",
		"TaskStates": {"init": {"State": "dead"}, "web": {"State": "running"}, "cleanup": {"State": "pending"}},
		"AllocatedResources": {"Tasks": {
			"init": {"Cpu": {"CpuShares": 1000}, "Memory": {"MemoryMB": 1000}},
			"web": {"Cpu": {"CpuShares": 500}, "Memory": {"MemoryMB": 256}},
			"cleanup": {"Cpu": {"CpuShares": 200}, "Memory": {"MemoryMB": 100}}
		}}
	}`, `{
		"Tasks": {
			"init": {"ResourceUsage": {"CpuStats": {"TotalTicks": 40}, "MemoryStats": {"RSS": 16777216}}},
			"web": {"ResourceUsage": {"CpuStats": {"TotalTicks": 125}, "MemoryStats": {"RSS": 134217728}}}
		}
	}`)

	use, err := client.AllocationUsage(context.Background(), "production", "af1f37df")
	r.NoError(err)

	r.Equal(125, use.CPUTicks)
	r.Equal(500, use.CPUTicksAllowed)
	r.Equal(25, use.CPUPercent)

	r.Equal(128, use.MemoryMB)
	r.Equal(256, use.MemoryMBAllowed)
	r.Equal(50, use.MemoryPercent)
}

func TestAllocationUsage_MemoryOutsideRSS(t *testing.T) {
	r := require.New(t)

	// A container that filled a tmpfs: RSS holds only its anonymous memory,
	// and the kernel counts all of Usage against its limit.
	client := statsServer(t, `{
		"Tasks": {
			"web": {"ResourceUsage": {"MemoryStats": {
				"RSS": 135168, "Cache": 200278016, "Usage": 201326592,
				"Measured": ["RSS", "Cache", "Swap", "Usage"]
			}}}
		}
	}`)

	use, err := client.AllocationUsage(context.Background(), "production", "af1f37df")
	r.NoError(err)

	r.Equal(192, use.MemoryMB)
	r.Equal(75, use.MemoryPercent)
}

func TestTaskUsage(t *testing.T) {
	r := require.New(t)

	client := allocServer(t, `{
		"ID": "af1f37df",
		"TaskStates": {"web": {"State": "running"}, "sidecar": {"State": "running"}},
		"AllocatedResources": {"Tasks": {
			"web": {"Cpu": {"CpuShares": 500}, "Memory": {"MemoryMB": 256}},
			"sidecar": {"Cpu": {"CpuShares": 100}, "Memory": {"MemoryMB": 64}}
		}}
	}`, `{
		"Tasks": {
			"web": {"ResourceUsage": {"CpuStats": {"TotalTicks": 125}, "MemoryStats": {"Usage": 134217728}}},
			"sidecar": {"ResourceUsage": {"CpuStats": {"TotalTicks": 50}, "MemoryStats": {"RSS": 33554432}}}
		}
	}`)

	use, err := client.TaskUsage(context.Background(), "production", "af1f37df", "sidecar")
	r.NoError(err)

	// Its own reading against what it asked for itself, not the allocation:
	// 50 of 100 ticks, 32 of 64 megabytes.
	r.Equal(50, use.CPUTicks)
	r.Equal(100, use.CPUTicksAllowed)
	r.Equal(50, use.CPUPercent)

	r.Equal(32, use.MemoryMB)
	r.Equal(64, use.MemoryMBAllowed)
	r.Equal(50, use.MemoryPercent)
}
