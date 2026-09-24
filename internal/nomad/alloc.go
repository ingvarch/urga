package nomad

import (
	"context"
	"net"
	"sort"
	"strconv"
	"time"

	"github.com/hashicorp/nomad/api"
)

// Alloc is one allocation: a task group of a job placed on a node.
type Alloc struct {
	ID        string
	Name      string
	Namespace string
	JobID     string
	JobType   string
	TaskGroup string

	NodeID   string
	NodeName string

	Status        string
	DesiredStatus string

	Tasks []Task

	Created  time.Time
	Modified time.Time

	JobVersion uint64

	// Health is what its deployment made of it: healthy, unhealthy, or
	// checking while it has not judged yet. Outside a deployment there is
	// none.
	Health string
	Canary bool

	Ports []Port

	// Reschedules is how many times the allocations before it were placed
	// again.
	Reschedules int

	// Previous is the allocation it replaced, Next the one that replaced it,
	// and FollowUp the evaluation that will place it again.
	Previous string
	Next     string
	FollowUp string
}

// Port is where an allocation listens, and the port inside the task it is
// mapped to, when it is mapped.
type Port struct {
	Label   string
	Address string
	To      int
}

// Task is one task of an allocation, as the cluster last reported it.
type Task struct {
	Name     string
	State    string
	Failed   bool
	Restarts int

	Started  time.Time
	Finished time.Time

	// Events are what happened to the task, newest first: the client says
	// what it did with it and why it stopped.
	Events []TaskEvent
}

// TaskEvent is one thing that happened to a task.
type TaskEvent struct {
	Time    time.Time
	Type    string
	Message string

	// Failed says this event is what took the task down.
	Failed bool
}

// Allocations lists the allocations of a job. An empty job lists every
// allocation of the namespace.
func (c *Client) Allocations(ctx context.Context, namespace, jobID string) ([]Alloc, error) {
	var (
		stubs []*api.AllocationListStub
		err   error
	)

	if jobID == "" {
		stubs, _, err = c.api.Allocations().List(c.query(ctx, namespace))
	} else {
		stubs, _, err = c.api.Jobs().Allocations(jobID, true, c.query(ctx, namespace))
	}

	if err != nil {
		return nil, err
	}

	allocs := make([]Alloc, 0, len(stubs))
	for _, stub := range stubs {
		allocs = append(allocs, newAlloc(stub))
	}

	return allocs, nil
}

// NodeAllocations lists what one machine of the cluster runs, whichever
// namespace the work belongs to.
func (c *Client) NodeAllocations(ctx context.Context, nodeID string) ([]Alloc, error) {
	stubs, _, err := c.api.Nodes().Allocations(nodeID, c.query(ctx, AllNamespaces))
	if err != nil {
		return nil, err
	}

	// The node answers with whole allocations where the other lists answer
	// with stubs. Cutting one down to a stub keeps the reading of a list in
	// one place.
	allocs := make([]Alloc, 0, len(stubs))
	for _, stub := range stubs {
		allocs = append(allocs, newAlloc(stubOf(stub)))
	}

	return allocs, nil
}

// Allocation is one allocation as the cluster holds it now.
func (c *Client) Allocation(ctx context.Context, namespace, allocID string) (Alloc, error) {
	alloc, _, err := c.api.Allocations().Info(allocID, c.query(ctx, namespace))
	if err != nil {
		return Alloc{}, err
	}

	return withDetail(newAlloc(stubOf(alloc)), alloc), nil
}

// withDetail adds what only the full reading of an allocation holds: what
// it is to its job and its deployment, where it listens, and what came
// before and after it.
func withDetail(out Alloc, alloc *api.Allocation) Alloc {
	if alloc.Job != nil && alloc.Job.Version != nil {
		out.JobVersion = *alloc.Job.Version
	}

	out.Health = healthOf(alloc.DeploymentID, alloc.DeploymentStatus)
	out.Canary = alloc.DeploymentStatus != nil && alloc.DeploymentStatus.Canary

	if alloc.AllocatedResources != nil {
		for _, port := range alloc.AllocatedResources.Shared.Ports {
			out.Ports = append(out.Ports, Port{
				Label:   port.Label,
				Address: net.JoinHostPort(port.HostIP, strconv.Itoa(port.Value)),
				To:      port.To,
			})
		}
	}

	if alloc.RescheduleTracker != nil {
		out.Reschedules = len(alloc.RescheduleTracker.Events)
	}

	out.Previous = alloc.PreviousAllocation
	out.Next = alloc.NextAllocation
	out.FollowUp = alloc.FollowupEvalID

	return out
}

// healthOf is what the deployment of an allocation made of it. One it has
// not judged yet carries no verdict.
func healthOf(deploymentID string, status *api.AllocDeploymentStatus) string {
	switch {
	case deploymentID == "":
		return ""
	case status == nil || status.Healthy == nil:
		return "checking"
	case *status.Healthy:
		return "healthy"
	}

	return "unhealthy"
}

// stubOf cuts a whole allocation down to what a list holds of it. The stub
// the API builds itself reads the type of the job without asking whether the
// job is there.
func stubOf(alloc *api.Allocation) *api.AllocationListStub {
	return &api.AllocationListStub{
		ID:            alloc.ID,
		Name:          alloc.Name,
		Namespace:     alloc.Namespace,
		JobID:         alloc.JobID,
		TaskGroup:     alloc.TaskGroup,
		NodeID:        alloc.NodeID,
		NodeName:      alloc.NodeName,
		ClientStatus:  alloc.ClientStatus,
		DesiredStatus: alloc.DesiredStatus,
		TaskStates:    alloc.TaskStates,
		CreateTime:    alloc.CreateTime,
		ModifyTime:    alloc.ModifyTime,
	}
}

func newAlloc(stub *api.AllocationListStub) Alloc {
	alloc := Alloc{
		ID:            stub.ID,
		Name:          stub.Name,
		Namespace:     stub.Namespace,
		JobID:         stub.JobID,
		JobType:       stub.JobType,
		TaskGroup:     stub.TaskGroup,
		NodeID:        stub.NodeID,
		NodeName:      stub.NodeName,
		Status:        stub.ClientStatus,
		DesiredStatus: stub.DesiredStatus,
		Tasks:         newTasks(stub.TaskStates),
	}

	alloc.Created = unixTime(stub.CreateTime)
	alloc.Modified = unixTime(stub.ModifyTime)

	return alloc
}

// newTaskEvents puts the newest first, which is the one worth reading.
func newTaskEvents(events []*api.TaskEvent) []TaskEvent {
	out := make([]TaskEvent, 0, len(events))

	for _, event := range events {
		if event == nil {
			continue
		}

		message := event.DisplayMessage
		if message == "" {
			message = event.Message
		}

		out = append(out, TaskEvent{
			Time:    unixTime(event.Time),
			Type:    event.Type,
			Message: message,
			Failed:  event.FailsTask,
		})
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })

	return out
}

// newTasks puts the tasks in the order of their names. A map hands them over
// in a different order every time, and the list would shuffle under the
// cursor.
func newTasks(states map[string]*api.TaskState) []Task {
	tasks := make([]Task, 0, len(states))

	for name, state := range states {
		task := Task{Name: name}

		if state != nil {
			task.State = state.State
			task.Failed = state.Failed
			task.Restarts = int(state.Restarts)
			task.Started = state.StartedAt
			task.Finished = state.FinishedAt
			task.Events = newTaskEvents(state.Events)
		}

		tasks = append(tasks, task)
	}

	sort.Slice(tasks, func(i, j int) bool { return tasks[i].Name < tasks[j].Name })

	return tasks
}
