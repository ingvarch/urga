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
