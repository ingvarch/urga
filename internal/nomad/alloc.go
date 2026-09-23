package nomad

import (
	"context"
	"sort"
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
		allocs = append(allocs, newAlloc(&api.AllocationListStub{
			ID:            stub.ID,
			Name:          stub.Name,
			Namespace:     stub.Namespace,
			JobID:         stub.JobID,
			TaskGroup:     stub.TaskGroup,
			NodeID:        stub.NodeID,
			NodeName:      stub.NodeName,
			ClientStatus:  stub.ClientStatus,
			DesiredStatus: stub.DesiredStatus,
			TaskStates:    stub.TaskStates,
			CreateTime:    stub.CreateTime,
			ModifyTime:    stub.ModifyTime,
		}))
	}

	return allocs, nil
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
