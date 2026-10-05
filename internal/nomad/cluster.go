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

// Usage reads the capacity of the nodes that are ready, without what each
// keeps for itself, and what the running allocations take of it. A datacenter narrows both to its own machines, an
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

		cpu, memory := schedulable(node)
		cpuCapacity += cpu
		memoryCapacity += memory
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

// schedulable is what a client gives to jobs: what it has without what it
// keeps for itself. A client that keeps more than it has gives nothing.
func schedulable(node *api.NodeListStub) (cpu, memory int64) {
	cpu, memory = node.NodeResources.Cpu.CpuShares, node.NodeResources.Memory.MemoryMB

	if kept := node.ReservedResources; kept != nil {
		cpu -= int64(kept.Cpu.CpuShares)
		memory -= int64(kept.Memory.MemoryMB)
	}

	return max(cpu, 0), max(memory, 0)
}

// AllocationUsage is how much one allocation uses of what it asked for.
func (c *Client) AllocationUsage(ctx context.Context, namespace, allocID string) (ResourceUse, error) {
	tasks, err := c.taskReadings(ctx, namespace, allocID)
	if err != nil {
		return ResourceUse{}, err
	}

	all := reading{}
	for _, task := range tasks {
		all = all.add(task)
	}

	return all.use(), nil
}

// TaskUsage is how much one task of an allocation uses of what it asked for.
func (c *Client) TaskUsage(ctx context.Context, namespace, allocID, task string) (ResourceUse, error) {
	tasks, err := c.taskReadings(ctx, namespace, allocID)
	if err != nil {
		return ResourceUse{}, err
	}

	return tasks[task].use(), nil
}

// reading is what a running task uses and what it asked for. The memory it
// uses stays in bytes until the tasks are added up.
type reading struct {
	ticks, ticksAllowed int
	memory              uint64
	memoryMBAllowed     int
}

func (r reading) add(other reading) reading {
	r.ticks += other.ticks
	r.ticksAllowed += other.ticksAllowed
	r.memory += other.memory
	r.memoryMBAllowed += other.memoryMBAllowed

	return r
}

func (r reading) use() ResourceUse {
	use := ResourceUse{
		CPUTicks:        r.ticks,
		CPUTicksAllowed: r.ticksAllowed,
		MemoryMB:        int(r.memory / megabyte),
		MemoryMBAllowed: r.memoryMBAllowed,
	}

	use.CPUPercent = percent(int64(use.CPUTicks), int64(use.CPUTicksAllowed))
	use.MemoryPercent = percent(int64(use.MemoryMB), int64(use.MemoryMBAllowed))

	return use
}

// taskReadings reads the running tasks of an allocation, by their names.
func (c *Client) taskReadings(ctx context.Context, namespace, allocID string) (map[string]reading, error) {
	alloc, _, err := c.api.Allocations().Info(allocID, c.query(ctx, namespace))
	if err != nil {
		return nil, err
	}

	stats, err := c.api.Allocations().Stats(alloc, c.query(ctx, namespace))
	if err != nil {
		return nil, err
	}

	tasks := map[string]reading{}

	if alloc.AllocatedResources != nil {
		for name, task := range alloc.AllocatedResources.Tasks {
			if !runs(alloc, name) {
				continue
			}

			tasks[name] = reading{ticksAllowed: int(task.Cpu.CpuShares), memoryMBAllowed: int(task.Memory.MemoryMB)}
		}
	}

	if stats != nil {
		// The summary of the allocation adds up each field apart, and tasks
		// of different drivers fill different fields. Each task is read by
		// the field it fills, and the tasks are added up from that.
		for name, task := range stats.Tasks {
			if task == nil || !runs(alloc, name) {
				continue
			}

			read := tasks[name]
			read.ticks, read.memory = readUsage(task.ResourceUsage)
			tasks[name] = read
		}
	}

	return tasks, nil
}

// runs says whether a task of an allocation is running. A task that ended,
// or waits for the others to stop, uses nothing of what it asked for, and
// the answer still holds its last reading. A task the allocation holds no
// state of is counted.
func runs(alloc *api.Allocation, task string) bool {
	state, known := alloc.TaskStates[task]

	return !known || state == nil || state.State == taskStateRunning
}

// NodeUsage is how much CPU and memory a node of the cluster uses.
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

// readUsage reads ticks and memory from a report, whichever field the
// driver filled. Usage is what the kernel counts against the limit of a
// task, tmpfs and file cache with it; RSS leaves them out, and is read from
// a driver that fills nothing else.
func readUsage(usage *api.ResourceUsage) (ticks int, memoryBytes uint64) {
	if usage == nil {
		return 0, 0
	}

	if cpu := usage.CpuStats; cpu != nil {
		ticks = int(cpu.TotalTicks)
	}

	if memory := usage.MemoryStats; memory != nil {
		memoryBytes = memory.Usage
		if memoryBytes == 0 {
			memoryBytes = memory.RSS
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
