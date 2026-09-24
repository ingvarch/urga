package nomad_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// recorder answers with the given body and keeps the request it was asked.
func recorder(t *testing.T, body string) (*nomad.Client, *http.Request) {
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

func TestJobs_AsksInTheNamespace(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `[]`)

	_, err := client.Jobs(context.Background(), "production")
	r.NoError(err)

	// The namespace belongs to the request. Asking in the wrong one answers
	// with an empty list or a 404, which reads like an empty cluster.
	r.Equal("/v1/jobs", asked.URL.Path)
	r.Equal("production", asked.URL.Query().Get("namespace"))
}

func TestJobs_AsksInEveryNamespace(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `[]`)

	_, err := client.Jobs(context.Background(), nomad.AllNamespaces)
	r.NoError(err)

	// The wildcard is what Nomad understands as "all of them".
	r.Equal("*", asked.URL.Query().Get("namespace"))
}

func TestJobs_ReadsTheList(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `[
		{
			"ID": "web",
			"Name": "web",
			"Namespace": "production",
			"Type": "service",
			"Status": "running",
			"SubmitTime": 1758499200000000000,
			"JobSummary": {
				"Summary": {
					"frontend": {"Running": 2, "Starting": 1},
					"backend": {"Running": 1},
					"worker": {"Running": 1, "Queued": 1, "Failed": 2, "Complete": 5, "Lost": 1}
				}
			}
		}
	]`)

	jobs, err := client.Jobs(context.Background(), "production")
	r.NoError(err)
	r.Len(jobs, 1)

	job := jobs[0]
	r.Equal("web", job.ID)
	r.Equal("production", job.Namespace)
	r.Equal("service", job.Type)
	r.Equal("running", job.Status)
	r.Equal(time.Unix(0, 1758499200000000000).UTC(), job.SubmitTime.UTC())

	// The counts of every task group together, the way the job list shows
	// them. Allocations that ended, failed or were lost are not waited for,
	// counting them reads as a job that never comes up.
	r.Equal(4, job.Running)
	r.Equal(6, job.Desired)

	// What waits for a place is a question of its own: why it is not placed.
	r.Equal(1, job.Queued)
}

func TestJobs_WithoutASummary(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `[{"ID": "web", "Name": "web", "Status": "pending"}]`)

	jobs, err := client.Jobs(context.Background(), "default")
	r.NoError(err)
	r.Len(jobs, 1)

	// A job the scheduler has not looked at yet has no summary at all.
	r.Zero(jobs[0].Running)
	r.Zero(jobs[0].Desired)

	// No submit time stays no submit time. Turning it into 1970 puts an age
	// of twenty thousand days in the list.
	r.True(jobs[0].SubmitTime.IsZero())
}

func TestAgent_ReadsTheBuildAndTheRegion(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{
		"config": {"Region": "eu"},
		"member": {"Name": "server-01", "Tags": {"build": "1.11.1"}}
	}`)

	agent, err := client.Agent(context.Background())
	r.NoError(err)

	r.Equal("/v1/agent/self", asked.URL.Path)
	r.Equal("1.11.1", agent.Version)

	// The region of the agent is where a request without one goes.
	r.Equal("eu", agent.Region)
}

func TestAgent_WithoutABuildTag(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `{"member": {"Name": "server-01"}}`)

	agent, err := client.Agent(context.Background())

	// An agent that does not say answers with nothing, not with an error the
	// header would have to show.
	r.NoError(err)
	r.Empty(agent.Version)
	r.Empty(agent.Region)
}

// headersOf is what a client sends with a request, and what it asks.
func headersOf(t *testing.T, cfg nomad.Config) (http.Header, url.Values) {
	t.Helper()

	var header http.Header

	var query url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header, query = r.Header.Clone(), r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(server.Close)

	cfg.Address = server.URL

	client, err := nomad.New(cfg)
	require.NoError(t, err)

	_, err = client.Jobs(context.Background(), "default")
	require.NoError(t, err)

	return header, query
}

func TestNew_ANamedClusterTakesNothingFromTheEnvironment(t *testing.T) {
	r := require.New(t)

	// Set for another cluster, or for none.
	t.Setenv("NOMAD_TOKEN", "the-token-of-dev")
	t.Setenv("NOMAD_REGION", "dev-region")
	t.Setenv("NOMAD_HTTP_AUTH", "dev:secret")

	header, query := headersOf(t, nomad.Config{Named: true})

	r.Empty(header.Get("X-Nomad-Token"))
	r.Empty(header.Get("Authorization"))
	r.Empty(query.Get("region"))

	// Its own token, and its own region.
	header, query = headersOf(t, nomad.Config{Named: true, Token: "the-token-of-prod", Region: "eu"})

	r.Equal("the-token-of-prod", header.Get("X-Nomad-Token"))
	r.Equal("eu", query.Get("region"))
}

func TestNew_WithoutANameTheEnvironmentIsTheCluster(t *testing.T) {
	r := require.New(t)

	t.Setenv("NOMAD_TOKEN", "the-token-of-dev")

	header, _ := headersOf(t, nomad.Config{})

	r.Equal("the-token-of-dev", header.Get("X-Nomad-Token"))
}

func TestNew_TheCertificatesOfANamedCluster(t *testing.T) {
	// A CA file that is not there is a cluster that cannot be trusted,
	// not one to talk to without it.
	_, err := nomad.New(nomad.Config{
		Address: "https://nomad.prod:4646",
		Named:   true,
		TLS:     nomad.TLS{CACert: filepath.Join(t.TempDir(), "missing-ca.pem")},
	})
	require.ErrorContains(t, err, "missing-ca.pem")
}
