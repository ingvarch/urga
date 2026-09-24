package nomad_test

import (
	"context"
	"testing"

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
