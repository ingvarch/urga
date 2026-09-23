package nomad_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStopJob(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{"EvalID": "eval-1"}`)

	r.NoError(client.StopJob(context.Background(), "production", "web"))

	r.Equal(http.MethodDelete, asked.Method)
	r.Equal("/v1/job/web", asked.URL.Path)
	r.Equal("production", asked.URL.Query().Get("namespace"))
}

func TestStartJob(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{"ID": "web", "Stop": true, "Version": 2}`)

	r.NoError(client.StartJob(context.Background(), "production", "web"))

	// Starting a stopped job is submitting it again with the stop flag off.
	r.Equal(http.MethodPut, asked.Method)
	r.Equal("/v1/jobs", asked.URL.Path)
}

func TestRestartAllocation(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{"ID": "af1f37df"}`)

	r.NoError(client.RestartAllocation(context.Background(), "production", "af1f37df"))

	r.Equal("/v1/client/allocation/af1f37df/restart", asked.URL.Path)
}

func TestStopAllocation(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{"ID": "af1f37df"}`)

	r.NoError(client.StopAllocation(context.Background(), "production", "af1f37df"))

	r.Equal("/v1/allocation/af1f37df/stop", asked.URL.Path)
}

func TestRevertJob(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{"ID": "web", "Version": 3}`)

	r.NoError(client.RevertJob(context.Background(), "production", "web"))

	// The version before the one that runs is what it goes back to.
	r.Equal("/v1/job/web/revert", asked.URL.Path)
}

func TestRevertJob_AtTheFirstVersion(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `{"ID": "web", "Version": 0}`)

	// There is nothing behind the first version, and saying so is better than
	// a cluster error.
	r.ErrorContains(client.RevertJob(context.Background(), "production", "web"), "no earlier version")
}

func TestScaleJob(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{"EvalID": "eval-1"}`)

	r.NoError(client.ScaleJob(context.Background(), "production", "web", "frontend", 3))

	r.Equal("/v1/job/web/scale", asked.URL.Path)
	r.Equal("production", asked.URL.Query().Get("namespace"))
}

func TestDrainNode(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{"NodeModifyIndex": 7}`)

	r.NoError(client.DrainNode(context.Background(), "node-1", true))

	r.Equal("/v1/node/node-1/drain", asked.URL.Path)
}

func TestDrainNode_Stop(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{"NodeModifyIndex": 7}`)

	// Stopping a drain puts the node back to taking work, otherwise it sits
	// there empty and nobody notices.
	r.NoError(client.DrainNode(context.Background(), "node-1", false))

	r.Equal("/v1/node/node-1/drain", asked.URL.Path)
}

func TestNodeEligibility(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{"NodeModifyIndex": 7}`)

	r.NoError(client.SetNodeEligible(context.Background(), "node-1", false))

	r.Equal("/v1/node/node-1/eligibility", asked.URL.Path)
}

func TestPromoteDeployment(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{"EvalID": "eval-1"}`)

	r.NoError(client.PromoteDeployment(context.Background(), "production", "dep-1"))

	r.Equal("/v1/deployment/promote/dep-1", asked.URL.Path)
	r.Equal("production", asked.URL.Query().Get("namespace"))
}

func TestFailDeployment(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{"EvalID": "eval-1"}`)

	r.NoError(client.FailDeployment(context.Background(), "production", "dep-1"))

	r.Equal("/v1/deployment/fail/dep-1", asked.URL.Path)
}
