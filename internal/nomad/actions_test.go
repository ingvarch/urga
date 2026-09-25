package nomad_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
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

// stoppedJob is a job as the cluster returns it after a stop: a new
// version, with the scaling policy turned off.
const stoppedJob = `{
	"ID": "web",
	"Name": "web",
	"Version": 1,
	"Stop": true,
	"TaskGroups": [{"Name": "g", "Scaling": {"Enabled": false, "Min": 1, "Max": 3}}]
}`

// register is the request that submitted the job.
func register(t *testing.T, asked []sent) sent {
	t.Helper()

	for _, req := range asked {
		if req.path == "/v1/jobs" {
			return req
		}
	}

	t.Fatal("the job was not submitted")

	return sent{}
}

func TestStartJob_KeepsTheSource(t *testing.T) {
	r := require.New(t)

	client, asked := jobServer(t, map[string]string{
		"/v1/job/web": `{"ID": "web", "Name": "web", "Version": 1, "Stop": true}`,
		"/v1/job/web/submission": `{
			"Source": "job \"web\" {}",
			"Format": "hcl2",
			"VariableFlags": {"image": "nginx:1.27"},
			"Variables": "count = 2\n"
		}`,
		"/v1/jobs": `{"EvalID": "eval-1"}`,
	})

	r.NoError(client.StartJob(context.Background(), "production", "web"))

	// Starting a job makes a new version. Without the file it was
	// submitted with, that version has no source to show or to edit.
	r.Equal("1", (*asked)[1].query.Get("version"))

	kept := submission(t, register(t, *asked))
	r.Equal("job \"web\" {}", kept["Source"])
	r.Equal("hcl2", kept["Format"])
	r.Equal(map[string]any{"image": "nginx:1.27"}, kept["VariableFlags"])
	r.Equal("count = 2\n", kept["Variables"])

	job, ok := register(t, *asked).body["Job"].(map[string]any)
	r.True(ok)
	r.Equal(false, job["Stop"])
}

func TestStartJob_WithoutASource(t *testing.T) {
	r := require.New(t)

	client, asked := jobServer(t, map[string]string{
		"/v1/job/web": `{"ID": "web", "Name": "web", "Version": 1, "Stop": true}`,
		"/v1/jobs":    `{"EvalID": "eval-1"}`,
	})

	// A job registered through the API has no file, and starts all the same.
	r.NoError(client.StartJob(context.Background(), "production", "web"))

	r.Nil(register(t, *asked).body["Submission"])
}

func TestStartJob_TurnsTheScalingPoliciesBackOn(t *testing.T) {
	r := require.New(t)

	client, asked := jobServer(t, map[string]string{
		"/v1/job/web": stoppedJob,
		"/v1/job/web/submission": `{
			"Source": "job \"web\" {}",
			"Format": "hcl2",
			"Variables": "max = 3\n"
		}`,
		"/v1/jobs/parse": `{"ID": "web", "TaskGroups": [{"Name": "g", "Scaling": {"Enabled": true}}]}`,
		"/v1/jobs":       `{"EvalID": "eval-1"}`,
	})

	r.NoError(client.StartJob(context.Background(), "production", "web"))

	// Stopping a job turns its scaling policies off. Starting sets them back
	// to what the file says, or the autoscaler ignores the job.
	parse := (*asked)[2]
	r.Equal("/v1/jobs/parse", parse.path)
	r.Equal("production", parse.namespace)
	r.Contains(parse.body["Variables"], "max = 3")

	job := register(t, *asked).body["Job"].(map[string]any)
	group := job["TaskGroups"].([]any)[0].(map[string]any)
	r.Equal(true, group["Scaling"].(map[string]any)["Enabled"])
}

func TestStartJob_WithoutScalingParsesNothing(t *testing.T) {
	r := require.New(t)

	client, asked := jobServer(t, map[string]string{
		"/v1/job/web":            `{"ID": "web", "Name": "web", "Version": 1, "Stop": true, "TaskGroups": [{"Name": "g"}]}`,
		"/v1/job/web/submission": `{"Source": "job \"web\" {}", "Format": "hcl2"}`,
		"/v1/jobs":               `{"EvalID": "eval-1"}`,
	})

	r.NoError(client.StartJob(context.Background(), "production", "web"))

	// A job with no scaling policy has nothing to turn back on. Its file is
	// not read, so an old one the cluster can no longer parse starts too.
	for _, req := range *asked {
		r.NotEqual("/v1/jobs/parse", req.path)
	}
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

func TestScaleJob(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{"EvalID": "eval-1"}`)

	r.NoError(client.ScaleJob(context.Background(), "production", "web", "frontend", 3))

	r.Equal("/v1/job/web/scale", asked.URL.Path)
	r.Equal("production", asked.URL.Query().Get("namespace"))
}

// drainRequest is what the cluster was asked to do with a node.
func drainRequest(t *testing.T, drain bool) map[string]any {
	t.Helper()

	var body map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "/v1/node/node-1/drain", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"NodeModifyIndex": 7}`))
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	require.NoError(t, err)

	require.NoError(t, client.DrainNode(context.Background(), "node-1", drain))

	return body
}

func TestDrainNode(t *testing.T) {
	r := require.New(t)

	body := drainRequest(t, true)

	// Draining asks for the allocations to be moved off, with a deadline.
	r.NotNil(body["DrainSpec"])
}

func TestDrainNode_Stop(t *testing.T) {
	r := require.New(t)

	body := drainRequest(t, false)

	// Stopping a drain cancels it and marks the node eligible again,
	// otherwise it stays empty and nobody notices.
	r.Nil(body["DrainSpec"])
	r.Equal(true, body["MarkEligible"])
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

func TestRestartTask(t *testing.T) {
	r := require.New(t)

	client, asked := jobServer(t, map[string]string{"/v1/client/allocation/af1f37df/restart": `{}`})

	r.NoError(client.RestartTask(context.Background(), "production", "af1f37df", "server"))

	// Only one task of the allocation restarts, the rest keep running.
	r.Len(*asked, 1)
	r.Equal("production", (*asked)[0].namespace)
	r.Equal("server", (*asked)[0].body["TaskName"])
	r.NotEqual(true, (*asked)[0].body["AllTasks"])
}

func TestSignalTask(t *testing.T) {
	r := require.New(t)

	client, asked := jobServer(t, map[string]string{"/v1/client/allocation/af1f37df/signal": `{}`})

	r.NoError(client.SignalTask(context.Background(), "production", "af1f37df", "server", "SIGHUP"))

	r.Len(*asked, 1)
	r.Equal("production", (*asked)[0].namespace)
	r.Equal("server", (*asked)[0].body["Task"])
	r.Equal("SIGHUP", (*asked)[0].body["Signal"])
}
