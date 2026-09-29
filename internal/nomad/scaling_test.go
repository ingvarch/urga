package nomad_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// scaledJob is a job with a group scaled by a policy, one without a policy,
// and one whose policy is off.
const scaledJob = `{"ID": "web", "TaskGroups": [
	{"Name": "frontend", "Count": 3, "Scaling": {"ID": "pol-1", "Type": "horizontal", "Min": 1, "Max": 5, "Enabled": true}},
	{"Name": "worker", "Count": 1},
	{"Name": "cache", "Count": 2, "Scaling": {"ID": "pol-2", "Type": "horizontal", "Min": 2, "Max": 4, "Enabled": false}}
]}`

// scaleStatus is how the cluster answers for the scaling of that job: the
// events newest first, a report without a new count as null.
const scaleStatus = `{"JobID": "web", "Namespace": "production", "TaskGroups": {
	"frontend": {"Desired": 3, "Placed": 3, "Running": 3, "Healthy": 2, "Events": [
		{"Time": 1790671961775549000, "Count": 3, "PreviousCount": 2, "Message": "scaled up by hand", "Error": false},
		{"Time": 1790671937831483000, "Count": null, "PreviousCount": 2, "Message": "no metrics from nomad-apm", "Error": true}
	]},
	"worker": {"Desired": 1, "Placed": 1, "Running": 1, "Events": null}
}}`

func TestScalingPolicies(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{
		"/v1/scaling/policies": `[{"ID": "pol-1", "Enabled": false, "Type": "horizontal",
			"Target": {"Namespace": "production", "Job": "web", "Group": "frontend"}}]`,
	})

	policies, err := client.ScalingPolicies(context.Background(), "production")
	r.NoError(err)

	r.Equal("production", (*asked)[0].namespace)
	r.Equal([]nomad.ScalingPolicy{
		{ID: "pol-1", Namespace: "production", Job: "web", Group: "frontend", Type: "horizontal"},
	}, policies)
}

func TestDescribeScalingPolicy(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{
		"/v1/scaling/policy/pol-1": `{"ID": "pol-1", "Min": 1, "Max": 5, "Enabled": true, "Policy": {"cooldown": "1m"}}`,
	})

	spec, err := client.DescribeScalingPolicy(context.Background(), "production", "pol-1")
	r.NoError(err)

	// The whole policy, with the settings the autoscaler reads.
	r.Equal("production", (*asked)[0].namespace)
	r.Contains(spec, `"Max": 5`)
	r.Contains(spec, `"cooldown": "1m"`)
}

func TestGroupScaling(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{"/v1/job/web": scaledJob, "/v1/job/web/scale": scaleStatus})

	scaling, err := client.GroupScaling(context.Background(), "production", "web", "frontend")
	r.NoError(err)

	for _, sent := range *asked {
		r.Equal("production", sent.namespace, sent.path)
	}

	r.Equal(3, scaling.Count)
	r.Equal(3, scaling.Running)
	r.Equal(2, scaling.Healthy)
	r.Equal(&nomad.GroupPolicy{ID: "pol-1", Enabled: true, Min: 1, Max: 5}, scaling.Policy)

	// Newest first, at the time the cluster keeps in nanoseconds.
	r.Len(scaling.Events, 2)
	r.Equal(time.Unix(0, 1790671961775549000), scaling.Events[0].At)
	r.Equal(2, scaling.Events[0].From)
	r.Equal(3, *scaling.Events[0].To)
	r.Equal("scaled up by hand", scaling.Events[0].Message)
	r.False(scaling.Events[0].Error)

	// A report that left the count as it was.
	r.Nil(scaling.Events[1].To)
	r.True(scaling.Events[1].Error)
}

func TestGroupScaling_AGroupWithoutAPolicy(t *testing.T) {
	r := require.New(t)

	client, _ := clusterServer(t, map[string]string{"/v1/job/web": scaledJob, "/v1/job/web/scale": scaleStatus})

	scaling, err := client.GroupScaling(context.Background(), "production", "web", "worker")
	r.NoError(err)

	r.Equal(1, scaling.Count)
	r.Nil(scaling.Policy)
	r.Empty(scaling.Events)
}

func TestTaskGroups_ReadTheirPolicy(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, scaledJob)

	groups, err := client.TaskGroups(context.Background(), "production", "web")
	r.NoError(err)

	r.Equal(&nomad.GroupPolicy{ID: "pol-1", Enabled: true, Min: 1, Max: 5}, groups[0].Policy)
	r.Nil(groups[1].Policy)
	r.Equal(&nomad.GroupPolicy{ID: "pol-2", Min: 2, Max: 4}, groups[2].Policy)
}
