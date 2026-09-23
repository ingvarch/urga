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
// allocations take of it. A datacenter narrows both to its own machines, an
// empty one is the whole region.
func (c *Client) Usage(ctx context.Context, datacenter string) (Usage, error) {
	nodes, _, err := c.api.Nodes().List(c.resourceQuery(ctx, ""))
	if err != nil {
		return Usage{}, err
	}

	allocs, _, err := c.api.Allocations().List(c.resourceQuery(ctx, AllNamespaces))
	if err != nil {
		return Usage{}, err
	}

	var cpuCapacity, memoryCapacity int64

	inside := map[string]bool{}

	for _, node := range nodes {
		if datacenter != "" && node.Datacenter != datacenter {
			continue
		}

		inside[node.ID] = true

		if node.Status != "ready" || node.NodeResources == nil {
			continue
		}

		cpuCapacity += node.NodeResources.Cpu.CpuShares
		memoryCapacity += node.NodeResources.Memory.MemoryMB
	}

	var cpuClaimed, memoryClaimed int64

	for _, alloc := range allocs {
		if datacenter != "" && !inside[alloc.NodeID] {
			continue
		}

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

	if stats != nil {
		ticks, memory := readUsage(stats.ResourceUsage)

		// Some drivers leave the summary of the allocation empty and report
		// per task instead. Adding the tasks up is what the summary would
		// have said.
		if ticks == 0 && memory == 0 {
			for _, task := range stats.Tasks {
				if task == nil {
					continue
				}

				taskTicks, taskMemory := readUsage(task.ResourceUsage)
				ticks += taskTicks
				memory += taskMemory
			}
		}

		use.CPUTicks = ticks
		use.MemoryMB = int(memory / megabyte)
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

	use.CPUTicks = int(stats.CPUTicksConsumed)

	if stats.Memory != nil {
		use.MemoryMB = int(stats.Memory.Used / megabyte)
		use.MemoryMBAllowed = int(stats.Memory.Total / megabyte)
		use.MemoryPercent = percent(int64(use.MemoryMB), int64(use.MemoryMBAllowed))
	}

	return use, nil
}

// readUsage takes what a report says, whichever field the driver filled. On
// cgroups v2 the memory of a task is in Usage and RSS stays at zero.
func readUsage(usage *api.ResourceUsage) (ticks int, memoryBytes uint64) {
	if usage == nil {
		return 0, 0
	}

	if cpu := usage.CpuStats; cpu != nil {
		ticks = int(cpu.TotalTicks)
	}

	if memory := usage.MemoryStats; memory != nil {
		memoryBytes = memory.RSS
		if memoryBytes == 0 {
			memoryBytes = memory.Usage
		}
	}

	return ticks, memoryBytes
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
