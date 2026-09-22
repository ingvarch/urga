package nomad_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// writeRecorder answers every request and keeps the last one, with its body.
func writeRecorder(t *testing.T, body string) (*nomad.Client, *http.Request) {
	t.Helper()

	asked := &http.Request{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*asked = *r
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	require.NoError(t, err)

	return client, asked
}

func TestStopJob(t *testing.T) {
	r := require.New(t)

	client, asked := writeRecorder(t, `{"EvalID": "eval-1"}`)

	r.NoError(client.StopJob(context.Background(), "production", "web"))

	r.Equal(http.MethodDelete, asked.Method)
	r.Equal("/v1/job/web", asked.URL.Path)
	r.Equal("production", asked.URL.Query().Get("namespace"))
}

func TestStartJob(t *testing.T) {
	r := require.New(t)

	client, asked := writeRecorder(t, `{"ID": "web", "Stop": true, "Version": 2}`)

	r.NoError(client.StartJob(context.Background(), "production", "web"))

	// Starting a stopped job is submitting it again with the stop flag off.
	r.Equal(http.MethodPut, asked.Method)
	r.Equal("/v1/jobs", asked.URL.Path)
}

func TestRestartAllocation(t *testing.T) {
	r := require.New(t)

	client, asked := writeRecorder(t, `{"ID": "af1f37df"}`)

	r.NoError(client.RestartAllocation(context.Background(), "production", "af1f37df"))

	r.Equal("/v1/client/allocation/af1f37df/restart", asked.URL.Path)
}

func TestStopAllocation(t *testing.T) {
	r := require.New(t)

	client, asked := writeRecorder(t, `{"ID": "af1f37df"}`)

	r.NoError(client.StopAllocation(context.Background(), "production", "af1f37df"))

	r.Equal("/v1/allocation/af1f37df/stop", asked.URL.Path)
}

func TestRevertJob(t *testing.T) {
	r := require.New(t)

	client, asked := writeRecorder(t, `{"ID": "web", "Version": 3}`)

	r.NoError(client.RevertJob(context.Background(), "production", "web"))

	// The version before the one that runs is what it goes back to.
	r.Equal("/v1/job/web/revert", asked.URL.Path)
}

func TestRevertJob_AtTheFirstVersion(t *testing.T) {
	r := require.New(t)

	client, _ := writeRecorder(t, `{"ID": "web", "Version": 0}`)

	// There is nothing behind the first version, and saying so is better than
	// a cluster error.
	r.ErrorContains(client.RevertJob(context.Background(), "production", "web"), "no earlier version")
}

func TestScaleJob(t *testing.T) {
	r := require.New(t)

	client, asked := writeRecorder(t, `{"EvalID": "eval-1"}`)

	r.NoError(client.ScaleJob(context.Background(), "production", "web", "frontend", 3))

	r.Equal("/v1/job/web/scale", asked.URL.Path)
	r.Equal("production", asked.URL.Query().Get("namespace"))
}
