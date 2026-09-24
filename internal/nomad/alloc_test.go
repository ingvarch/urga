package nomad_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

const allocList = `[
	{
		"ID": "af1f37df-7b19-6b1c-da67-5e8f482b5a15",
		"Name": "web.frontend[0]",
		"Namespace": "production",
		"JobID": "web",
		"JobType": "service",
		"TaskGroup": "frontend",
		"NodeID": "a3e23694-4528-6395-3a51-1ffcbf3c2ba4",
		"NodeName": "nomad-server-01",
		"DesiredStatus": "run",
		"ClientStatus": "running",
		"CreateTime": 1758499200000000000,
		"ModifyTime": 1758585600000000000,
		"TaskStates": {
			"server": {"State": "running", "Failed": false, "Restarts": 2},
			"sidecar": {"State": "dead", "Failed": true}
		}
	}
]`

func TestAllocations_OfAJob(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, allocList)

	allocs, err := client.Allocations(context.Background(), "production", "web")
	r.NoError(err)

	// The allocations of one job, asked for in its namespace.
	r.Equal("/v1/job/web/allocations", asked.URL.Path)
	r.Equal("production", asked.URL.Query().Get("namespace"))

	r.Len(allocs, 1)

	alloc := allocs[0]
	r.Equal("af1f37df-7b19-6b1c-da67-5e8f482b5a15", alloc.ID)
	r.Equal("frontend", alloc.TaskGroup)
	r.Equal("nomad-server-01", alloc.NodeName)
	r.Equal("running", alloc.Status)
	r.Equal("run", alloc.DesiredStatus)
	r.False(alloc.Created.IsZero())
}

func TestAllocations_OfTheCluster(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, allocList)

	_, err := client.Allocations(context.Background(), nomad.AllNamespaces, "")
	r.NoError(err)

	// Without a job it is every allocation the namespace holds.
	r.Equal("/v1/allocations", asked.URL.Path)
	r.Equal("*", asked.URL.Query().Get("namespace"))
}

func TestAllocations_Tasks(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, allocList)

	allocs, err := client.Allocations(context.Background(), "production", "web")
	r.NoError(err)

	// The tasks come in the order their names read, so the list does not
	// shuffle between two polls.
	r.Len(allocs[0].Tasks, 2)
	r.Equal("server", allocs[0].Tasks[0].Name)
	r.Equal("running", allocs[0].Tasks[0].State)
	r.Equal(2, allocs[0].Tasks[0].Restarts)

	r.Equal("sidecar", allocs[0].Tasks[1].Name)
	r.True(allocs[0].Tasks[1].Failed)
}

func TestNodeAllocations_AsksTheNode(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `[{
		"ID": "af1f37df-4528-6395-3a51-1ffcbf3c2ba4",
		"Name": "pelmeni_buh_bot.pelmenis[0]",
		"Namespace": "production",
		"JobID": "pelmeni_buh_bot",
		"TaskGroup": "pelmenis",
		"NodeID": "node-1",
		"NodeName": "nomad-server-01",
		"ClientStatus": "running",
		"DesiredStatus": "run",
		"CreateTime": 1758499200000000000
	}]`)

	allocs, err := client.NodeAllocations(context.Background(), "node-1")
	r.NoError(err)
	r.Len(allocs, 1)

	// The machine is asked for its own work, in every namespace: a client
	// runs whatever the schedulers put on it.
	r.Equal("/v1/node/node-1/allocations", asked.URL.Path)
	r.Equal("*", asked.URL.Query().Get("namespace"))

	r.Equal("pelmeni_buh_bot", allocs[0].JobID)
	r.Equal("running", allocs[0].Status)
}

func TestAllocations_ReadWhatHappenedToATask(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `[{
		"ID": "af1f37df",
		"Name": "web.bot[0]",
		"TaskStates": {
			"bot": {
				"State": "running",
				"Events": [
					{"Type": "Received", "Time": 1758499200000000000, "DisplayMessage": "Task received by client"},
					{"Type": "Started", "Time": 1758499260000000000, "DisplayMessage": "Task started by client"},
					{"Type": "Terminated", "Time": 1758499320000000000, "DisplayMessage": "Exit Code: 1",
						"Details": {"exit_code": "1"}, "FailsTask": true}
				]
			}
		}
	}]`)

	allocs, err := client.Allocations(context.Background(), "production", "")
	r.NoError(err)
	r.Len(allocs[0].Tasks, 1)

	events := allocs[0].Tasks[0].Events
	r.Len(events, 3)

	// The newest is the one worth reading, so it comes first.
	r.Equal("Terminated", events[0].Type)
	r.Equal("Exit Code: 1", events[0].Message)
	r.True(events[0].Failed)
	r.False(events[0].Time.IsZero())

	r.Equal("Received", events[2].Type)
	r.False(events[2].Failed)
}

func TestAllocation(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{
		"ID": "af1f37df-7b19-6b1c-da67-5e8f482b5a15",
		"Namespace": "production",
		"JobID": "web",
		"TaskGroup": "frontend",
		"ClientStatus": "running",
		"DesiredStatus": "run",
		"TaskStates": {"server": {"State": "running", "Restarts": 3}}
	}`)

	alloc, err := client.Allocation(context.Background(), "production", "af1f37df-7b19-6b1c-da67-5e8f482b5a15")
	r.NoError(err)

	// One allocation, asked for in its own namespace: a screen about one
	// allocation reads that one, not the whole list of its job.
	r.Equal("/v1/allocation/af1f37df-7b19-6b1c-da67-5e8f482b5a15", asked.URL.Path)
	r.Equal("production", asked.URL.Query().Get("namespace"))

	r.Equal("running", alloc.Status)
	r.Len(alloc.Tasks, 1)
	r.Equal(3, alloc.Tasks[0].Restarts)
}

// replacedCanary is an allocation of web that replaced one that failed, is a
// canary of a deployment that has not judged it yet, and listens on two ports.
const replacedCanary = `{
	"ID": "af1f37df-7b19-6b1c-da67-5e8f482b5a15",
	"Namespace": "production",
	"JobID": "web",
	"Job": {"ID": "web", "Version": 7},
	"TaskGroup": "frontend",
	"ClientStatus": "running",
	"DesiredStatus": "run",
	"DeploymentID": "d74c1812-0000-0000-0000-000000000000",
	"DeploymentStatus": {"Healthy": null, "Canary": true},
	"AllocatedResources": {"Shared": {"Ports": [
		{"Label": "http", "Value": 21659, "To": 8080, "HostIP": "10.0.0.5"},
		{"Label": "admin", "Value": 22689, "To": 0, "HostIP": "10.0.0.5"}
	]}},
	"RescheduleTracker": {"Events": [{"PrevAllocID": "4f2a1c9e"}, {"PrevAllocID": "3e1b0a8d"}]},
	"PreviousAllocation": "4f2a1c9e-0000-0000-0000-000000000000",
	"NextAllocation": "9a1b2c3d-0000-0000-0000-000000000000",
	"FollowupEvalID": "8b1c2d3e-0000-0000-0000-000000000000"
}`

func TestAllocation_WhereItRunsAndListens(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, replacedCanary)

	alloc, err := client.Allocation(context.Background(), "production", "af1f37df-7b19-6b1c-da67-5e8f482b5a15")
	r.NoError(err)

	r.Equal(uint64(7), alloc.JobVersion)

	// A deployment that has not judged it yet is still checking it.
	r.Equal("checking", alloc.Health)
	r.True(alloc.Canary)

	// Where it listens, and where a port is mapped to inside the task.
	r.Equal([]nomad.Port{
		{Label: "http", Address: "10.0.0.5:21659", To: 8080},
		{Label: "admin", Address: "10.0.0.5:22689"},
	}, alloc.Ports)

	r.Equal(2, alloc.Reschedules)
	r.Equal("4f2a1c9e-0000-0000-0000-000000000000", alloc.Previous)
	r.Equal("9a1b2c3d-0000-0000-0000-000000000000", alloc.Next)
	r.Equal("8b1c2d3e-0000-0000-0000-000000000000", alloc.FollowUp)
}

func TestAllocation_HealthInItsDeployment(t *testing.T) {
	r := require.New(t)

	health := func(status string) string {
		client, _ := recorder(t, `{"ID": "a", "DeploymentID": "d", "DeploymentStatus": `+status+`}`)

		alloc, err := client.Allocation(context.Background(), "production", "a")
		r.NoError(err)

		return alloc.Health
	}

	r.Equal("healthy", health(`{"Healthy": true}`))
	r.Equal("unhealthy", health(`{"Healthy": false}`))
	r.Equal("checking", health(`null`))

	// Outside a deployment there is no health to speak of.
	client, _ := recorder(t, `{"ID": "a"}`)
	alloc, err := client.Allocation(context.Background(), "production", "a")
	r.NoError(err)
	r.Empty(alloc.Health)
}

// servedChecks are the checks of an allocation of served: its page answers,
// its admin port does not, and a check of the task has not run yet.
const servedChecks = `{
	"a1": {"ID": "a1", "Service": "served", "Check": "alive", "Status": "success",
		"Output": "nomad: http ok", "Timestamp": 1790270007, "Task": ""},
	"b2": {"ID": "b2", "Service": "served-admin", "Check": "admin-up", "Status": "failure",
		"Output": "dial tcp 127.0.0.1:22689: connect: connection refused\n", "Timestamp": 1790270008},
	"c3": {"ID": "c3", "Service": "served", "Check": "ready", "Status": "pending", "Task": "server"}
}`

func TestAllocationChecks(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, servedChecks)

	checks, err := client.AllocationChecks(context.Background(), "production", "af1f37df")
	r.NoError(err)

	// The client that runs the allocation runs its checks, and is asked
	// through the servers, in the namespace of the allocation.
	r.Equal("/v1/client/allocation/af1f37df/checks", asked.URL.Path)
	r.Equal("production", asked.URL.Query().Get("namespace"))

	// What fails first, then what has not run yet, then what passes.
	r.Equal([]nomad.Check{
		{Service: "served-admin", Name: "admin-up", Status: "failure",
			Output: "dial tcp 127.0.0.1:22689: connect: connection refused", Time: time.Unix(1790270008, 0)},
		{Service: "served", Name: "ready", Task: "server", Status: "pending"},
		{Service: "served", Name: "alive", Status: "success", Output: "nomad: http ok", Time: time.Unix(1790270007, 0)},
	}, checks)
}

func TestAllocationChecks_OfAnAllocationThatStopped(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `null`)

	checks, err := client.AllocationChecks(context.Background(), "production", "af1f37df")
	r.NoError(err)
	r.Empty(checks)
}

func TestAllocationChecks_InTheOrderOfTheirNames(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `{
		"x": {"Service": "web", "Check": "b", "Status": "success"},
		"y": {"Service": "api", "Check": "z", "Status": "success"},
		"z": {"Service": "web", "Check": "a", "Status": "success"}
	}`)

	checks, err := client.AllocationChecks(context.Background(), "production", "af1f37df")
	r.NoError(err)

	names := []string{}
	for _, check := range checks {
		names = append(names, check.Service+"/"+check.Name)
	}

	r.Equal([]string{"api/z", "web/a", "web/b"}, names)
}
