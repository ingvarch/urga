package nomad_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// canaryDeployment is a deployment of web that waits for its canary to be
// promoted, while api is done.
const canaryDeployment = `{
	"ID": "dep-1", "JobID": "web", "Namespace": "production", "JobVersion": 7,
	"Status": "running", "StatusDescription": "Deployment is running but requires manual promotion",
	"TaskGroups": {
		"web": {"DesiredTotal": 3, "PlacedAllocs": 1, "HealthyAllocs": 0, "UnhealthyAllocs": 0,
			"DesiredCanaries": 1, "PlacedCanaries": ["a1"], "Promoted": false, "AutoRevert": true,
			"ProgressDeadline": 600000000000, "RequireProgressBy": "2026-09-24T18:10:00Z"},
		"api": {"DesiredTotal": 2, "PlacedAllocs": 2, "HealthyAllocs": 2, "UnhealthyAllocs": 0}
	}
}`

func TestDeployment(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, canaryDeployment)

	deployment, err := client.Deployment(context.Background(), "production", "dep-1")
	r.NoError(err)

	r.Equal("/v1/deployment/dep-1", asked.URL.Path)
	r.Equal("production", asked.URL.Query().Get("namespace"))

	r.Equal(nomad.Deployment{
		ID: "dep-1", JobID: "web", Namespace: "production", JobVersion: 7, Status: "running",
		StatusDescription: "Deployment is running but requires manual promotion",
	}, deployment.Deployment)

	// Each group in the order of their names.
	r.Equal([]nomad.DeploymentGroup{
		{Name: "api", DesiredTotal: 2, Placed: 2, Healthy: 2},
		{
			Name: "web", DesiredTotal: 3, Placed: 1,
			DesiredCanaries: 1, PlacedCanaries: 1, AutoRevert: true,
			ProgressDeadline:  10 * time.Minute,
			RequireProgressBy: time.Date(2026, 9, 24, 18, 10, 0, 0, time.UTC),
		},
	}, deployment.Groups)
}

func TestDeploymentGroup_WaitsForPromotion(t *testing.T) {
	r := require.New(t)

	r.True(nomad.DeploymentGroup{DesiredCanaries: 1}.WaitsForPromotion())
	r.False(nomad.DeploymentGroup{DesiredCanaries: 1, Promoted: true}.WaitsForPromotion())
	r.False(nomad.DeploymentGroup{}.WaitsForPromotion())
}

func TestDeploymentAllocations(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `[
		{"ID": "a1", "TaskGroup": "web", "ClientStatus": "running", "DeploymentStatus": {"Healthy": null, "Canary": true}},
		{"ID": "a2", "TaskGroup": "api", "ClientStatus": "running", "DeploymentStatus": {"Healthy": true}},
		{"ID": "a3", "TaskGroup": "api", "ClientStatus": "failed", "DeploymentStatus": {"Healthy": false}}
	]`)

	allocs, err := client.DeploymentAllocations(context.Background(), "production", "dep-1")
	r.NoError(err)

	r.Equal("/v1/deployment/allocations/dep-1", asked.URL.Path)
	r.Equal("production", asked.URL.Query().Get("namespace"))

	// Every one of them is in the deployment: one with no health set yet is
	// still being checked.
	r.Len(allocs, 3)
	r.Equal("checking", allocs[0].Health)
	r.True(allocs[0].Canary)
	r.Equal("healthy", allocs[1].Health)
	r.False(allocs[1].Canary)
	r.Equal("unhealthy", allocs[2].Health)
}

func TestPromoteGroups(t *testing.T) {
	r := require.New(t)

	client, asked := jobServer(t, map[string]string{"/v1/deployment/promote/dep-1": `{}`})

	r.NoError(client.PromoteGroups(context.Background(), "production", "dep-1", []string{"web"}))

	r.Len(*asked, 1)
	r.Equal("production", (*asked)[0].namespace)
	r.Equal([]any{"web"}, (*asked)[0].body["Groups"])
	r.NotEqual(true, (*asked)[0].body["All"])
}

func TestPauseDeployment(t *testing.T) {
	r := require.New(t)

	client, asked := jobServer(t, map[string]string{"/v1/deployment/pause/dep-1": `{}`})

	r.NoError(client.PauseDeployment(context.Background(), "production", "dep-1", true))
	r.NoError(client.PauseDeployment(context.Background(), "production", "dep-1", false))

	r.Equal(true, (*asked)[0].body["Pause"])
	r.Equal(false, (*asked)[1].body["Pause"])
	r.Equal("production", (*asked)[1].namespace)
}

func TestDeployment_Active(t *testing.T) {
	r := require.New(t)

	for _, status := range []string{"running", "paused", "pending", "blocked", "unblocking", "initializing"} {
		r.True(nomad.Deployment{Status: status}.Active(), status)
	}

	// A deployment that has ended is not active.
	for _, status := range []string{"successful", "failed", "cancelled"} {
		r.False(nomad.Deployment{Status: status}.Active(), status)
	}
}
