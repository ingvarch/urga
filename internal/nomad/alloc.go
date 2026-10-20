package nomad

import (
	"context"
	"net"
	"sort"
	"strconv"
	"strings"
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

	// Health is what the client reported of its checks: healthy, unhealthy,
	// or checking until it decides. One in a deployment with nothing
	// reported yet is checking; one with neither has none.
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

	// LastRestart is when the task was restarted last, zero for never.
	LastRestart time.Time

	// Events are what happened to the task, newest first: the client says
	// what it did with it and why it stopped.
	Events []TaskEvent
}

// TaskEvent is one thing that happened to a task.
type TaskEvent struct {
	Time    time.Time
	Type    string
	Message string

	// Failed says this event is what made the task fail.
	Failed bool

	// OOM says the kernel killed the task for the memory it used.
	OOM bool
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

	return allocsOf(stubs), nil
}

// allocsOf are the allocations of a list the cluster answered with.
func allocsOf(stubs []*api.AllocationListStub) []Alloc {
	out := make([]Alloc, 0, len(stubs))

	for _, stub := range stubs {
		if stub != nil {
			out = append(out, newAlloc(stub))
		}
	}

	return out
}

// HoldsPlace says the allocation counts against the limit of its client: the
// scheduler counts what is meant to run and has not ended.
func (a Alloc) HoldsPlace() bool { return holdsPlace(a.DesiredStatus, a.Status) }

func holdsPlace(desired, status string) bool {
	switch desired {
	case api.AllocDesiredStatusStop, api.AllocDesiredStatusEvict:
		return false
	}

	switch status {
	case api.AllocClientStatusComplete, api.AllocClientStatusFailed, api.AllocClientStatusLost:
		return false
	}

	return true
}

// NodeAllocations lists what one machine of the cluster runs, whichever
// namespace the work belongs to.
func (c *Client) NodeAllocations(ctx context.Context, nodeID string) ([]Alloc, error) {
	stubs, _, err := c.api.Nodes().Allocations(nodeID, c.query(ctx, AllNamespaces))
	if err != nil {
		return nil, err
	}

	// The node returns full allocations where the other lists return
	// stubs. Copying each into a stub lets newAlloc build every list.
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

// withDetail adds the fields only a full allocation has: its deployment
// health, its ports, the allocation before it and the evaluation after it.
func withDetail(out Alloc, alloc *api.Allocation) Alloc {
	out.Health = healthOf(alloc.DeploymentID, alloc.DeploymentStatus)

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
	out.FollowUp = alloc.FollowupEvalID

	return out
}

// The health a deployment reports for an allocation.
const (
	HealthChecking  = "checking"
	HealthHealthy   = "healthy"
	HealthUnhealthy = "unhealthy"
)

// healthOf is the health the client reported for an allocation, in a
// deployment or not. The client checks a rescheduled or moved one too.
func healthOf(deploymentID string, status *api.AllocDeploymentStatus) string {
	switch {
	case status != nil:
		return statusHealth(status)
	case deploymentID != "":
		return HealthChecking
	}

	return ""
}

// statusHealth is the health a status holds, checking until the client
// decides.
func statusHealth(status *api.AllocDeploymentStatus) string {
	switch {
	case status == nil || status.Healthy == nil:
		return HealthChecking
	case *status.Healthy:
		return HealthHealthy
	}

	return HealthUnhealthy
}

// stubOf copies a full allocation into a list stub. The Stub method of the
// API reads the type of the job without a nil check.
func stubOf(alloc *api.Allocation) *api.AllocationListStub {
	stub := &api.AllocationListStub{
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

		DeploymentStatus: alloc.DeploymentStatus,
		NextAllocation:   alloc.NextAllocation,
	}

	if alloc.Job != nil {
		if alloc.Job.Type != nil {
			stub.JobType = *alloc.Job.Type
		}

		if alloc.Job.Version != nil {
			stub.JobVersion = *alloc.Job.Version
		}
	}

	return stub
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
		JobVersion:    stub.JobVersion,
		Next:          stub.NextAllocation,
	}

	// A list stub has no deployment ID: its health is what the client
	// reported.
	if stub.DeploymentStatus != nil {
		alloc.Health = statusHealth(stub.DeploymentStatus)
		alloc.Canary = stub.DeploymentStatus.Canary
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
			OOM:     event.Details["oom_killed"] == "true",
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
			task.LastRestart = state.LastRestart
			task.Events = newTaskEvents(state.Events)
		}

		tasks = append(tasks, task)
	}

	sort.Slice(tasks, func(i, j int) bool { return tasks[i].Name < tasks[j].Name })

	return tasks
}

// Check is one check of a service of an allocation, as its client last ran
// it: success, failure, or pending until it has run.
type Check struct {
	Service string
	Name    string
	Task    string
	Status  string
	Output  string
	Time    time.Time
}

// AllocationChecks are the checks of the services an allocation registers
// with the cluster itself.
func (c *Client) AllocationChecks(ctx context.Context, namespace, allocID string) ([]Check, error) {
	statuses, err := c.api.Allocations().Checks(allocID, c.query(ctx, namespace))
	if err != nil {
		return nil, err
	}

	checks := make([]Check, 0, len(statuses))

	for _, status := range statuses {
		check := Check{
			Service: status.Service,
			Name:    status.Check,
			Task:    status.Task,
			Status:  status.Status,
			Output:  strings.TrimSpace(status.Output),
		}

		// One that has not run yet has no time.
		if status.Timestamp > 0 {
			check.Time = time.Unix(status.Timestamp, 0)
		}

		checks = append(checks, check)
	}

	// Failing checks come first, they are what the list is for. The rest are
	// sorted, since a map returns them in a new order on every read.
	sort.Slice(checks, func(i, j int) bool {
		a, b := checks[i], checks[j]

		if checkRank(a.Status) != checkRank(b.Status) {
			return checkRank(a.Status) < checkRank(b.Status)
		}

		if a.Service != b.Service {
			return a.Service < b.Service
		}

		return a.Name < b.Name
	})

	return checks, nil
}

// checkRank puts a failing check before one that has not run, and that
// before one that passes.
func checkRank(status string) int {
	switch status {
	case "failure":
		return 0
	case "success":
		return 2
	}

	return 1
}
