package nomad

import (
	"context"

	"github.com/hashicorp/nomad/api"
)

// Usage is how much of the cluster the running allocations claim.
type Usage struct {
	CPUPercent    int
	MemoryPercent int
}

// withResources asks Nomad to put the resources in a list answer.
var withResources = map[string]string{"resources": "true"}

// Usage reads the capacity of the nodes that are ready and what the running
// allocations take of it.
func (c *Client) Usage(ctx context.Context) (Usage, error) {
	nodes, _, err := c.api.Nodes().List(c.resourceQuery(ctx, ""))
	if err != nil {
		return Usage{}, err
	}

	allocs, _, err := c.api.Allocations().List(c.resourceQuery(ctx, AllNamespaces))
	if err != nil {
		return Usage{}, err
	}

	var cpuCapacity, memoryCapacity int64

	for _, node := range nodes {
		if node.Status != "ready" || node.NodeResources == nil {
			continue
		}

		cpuCapacity += node.NodeResources.Cpu.CpuShares
		memoryCapacity += node.NodeResources.Memory.MemoryMB
	}

	var cpuClaimed, memoryClaimed int64

	for _, alloc := range allocs {
		if alloc.ClientStatus != "running" || alloc.AllocatedResources == nil {
			continue
		}

		for _, task := range alloc.AllocatedResources.Tasks {
			cpuClaimed += task.Cpu.CpuShares
			memoryClaimed += task.Memory.MemoryMB
		}
	}

	return Usage{
		CPUPercent:    percent(cpuClaimed, cpuCapacity),
		MemoryPercent: percent(memoryClaimed, memoryCapacity),
	}, nil
}

// AllocationUsage is what one allocation takes of what it asked for.
func (c *Client) AllocationUsage(ctx context.Context, namespace, allocID string) (ResourceUse, error) {
	alloc, _, err := c.api.Allocations().Info(allocID, c.query(ctx, namespace))
	if err != nil {
		return ResourceUse{}, err
	}

	stats, err := c.api.Allocations().Stats(alloc, c.query(ctx, namespace))
	if err != nil {
		return ResourceUse{}, err
	}

	use := ResourceUse{}

	if alloc.AllocatedResources != nil {
		for _, task := range alloc.AllocatedResources.Tasks {
			use.CPUTicksAllowed += int(task.Cpu.CpuShares)
			use.MemoryMBAllowed += int(task.Memory.MemoryMB)
		}
	}

	if stats != nil && stats.ResourceUsage != nil {
		if cpu := stats.ResourceUsage.CpuStats; cpu != nil {
			use.CPUTicks = int(cpu.TotalTicks)
		}

		if memory := stats.ResourceUsage.MemoryStats; memory != nil {
			use.MemoryMB = int(memory.RSS / megabyte)
		}
	}

	use.CPUPercent = percent(int64(use.CPUTicks), int64(use.CPUTicksAllowed))
	use.MemoryPercent = percent(int64(use.MemoryMB), int64(use.MemoryMBAllowed))

	return use, nil
}

// NodeUsage is what a machine of the cluster is busy with.
func (c *Client) NodeUsage(ctx context.Context, nodeID string) (ResourceUse, error) {
	stats, err := c.api.Nodes().Stats(nodeID, c.query(ctx, ""))
	if err != nil {
		return ResourceUse{}, err
	}

	use := ResourceUse{}

	// The cores of the machine together. Nomad reports how idle each core
	// is, busy is what is left of it.
	if cores := len(stats.CPU); cores > 0 {
		busy := 0.0
		for _, cpu := range stats.CPU {
			busy += 100 - cpu.Idle
		}

		use.CPUPercent = int(busy/float64(cores) + 0.5)
	}

	if stats.Memory != nil {
		use.MemoryMB = int(stats.Memory.Used / megabyte)
		use.MemoryMBAllowed = int(stats.Memory.Total / megabyte)
		use.MemoryPercent = percent(int64(use.MemoryMB), int64(use.MemoryMBAllowed))
	}

	return use, nil
}

// megabyte is what the stats are turned into, they arrive in bytes.
const megabyte = 1024 * 1024

func (c *Client) resourceQuery(ctx context.Context, namespace string) *api.QueryOptions {
	q := c.query(ctx, namespace)
	q.Params = withResources

	return q
}

func percent(claimed, capacity int64) int {
	if capacity <= 0 {
		return 0
	}

	return int((claimed*100 + capacity/2) / capacity)
}
