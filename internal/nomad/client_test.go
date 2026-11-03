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

// recorder answers with the given body and keeps the request it received.
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

	// The namespace is sent with the request. A request in the wrong one gets
	// an empty list or a 404, which looks like an empty cluster.
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

	// The counts of every task group added up, as the job list shows them.
	// Allocations that completed, failed or were lost are left out of
	// Desired: counting them would show a job that never comes up.
	r.Equal(4, job.Running)
	r.Equal(6, job.Desired)

	// Queued allocations are counted apart: why they wait is its own question.
	r.Equal(1, job.Queued)
}

func TestJobs_ReadsWhetherAJobWasStopped(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `[
		{"ID": "old", "Type": "service", "Status": "dead", "Stop": true},
		{"ID": "crashed", "Type": "service", "Status": "dead"}
	]`)

	jobs, err := client.Jobs(context.Background(), "production")
	r.NoError(err)
	r.Len(jobs, 2)

	// The list comes back sorted by ID, so each job is found by its ID.
	stopped := map[string]bool{}
	for _, job := range jobs {
		stopped[job.ID] = job.Stopped
	}

	r.Equal(map[string]bool{"old": true, "crashed": false}, stopped)
}

func TestJobs_ReadsWhatLaunchedAJob(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `[
		{"ID": "backup", "Type": "batch", "Status": "running", "Periodic": true},
		{"ID": "backup/periodic-1758499200", "ParentID": "backup", "Type": "batch", "Status": "dead"},
		{"ID": "report", "Type": "batch", "Status": "running", "ParameterizedJob": true}
	]`)

	jobs, err := client.Jobs(context.Background(), "production")
	r.NoError(err)
	r.Len(jobs, 3)

	r.True(jobs[0].Periodic)
	r.False(jobs[0].Parameterized)
	r.Empty(jobs[0].ParentID)

	// A launch names the job that launched it.
	r.Equal("backup", jobs[1].ParentID)
	r.False(jobs[1].Periodic)

	r.True(jobs[2].Parameterized)
	r.False(jobs[2].Periodic)
}

func TestJobs_ReadsWhetherAJobFailed(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `[
		{"ID": "failed", "JobSummary": {"Summary": {"g": {"Failed": 1}}}},
		{"ID": "lost", "JobSummary": {"Summary": {"g": {"Lost": 1}}}},
		{"ID": "retried", "JobSummary": {"Summary": {"g": {"Failed": 1, "Complete": 1}}}},
		{"ID": "one-group-failed", "JobSummary": {"Summary": {"a": {"Complete": 1}, "b": {"Failed": 1}}}},
		{"ID": "still-running", "JobSummary": {"Summary": {"g": {"Failed": 1, "Running": 1}}}},
		{"ID": "done", "JobSummary": {"Summary": {"g": {"Complete": 1}}}}
	]`)

	jobs, err := client.Jobs(context.Background(), "production")
	r.NoError(err)

	failed := map[string]bool{}
	for _, job := range jobs {
		failed[job.ID] = job.Failed
	}

	// A group failed when it ended with an allocation failed or lost and
	// none complete. One that completed on a retry did not fail, nor did
	// one that still runs.
	r.Equal(map[string]bool{
		"failed":           true,
		"lost":             true,
		"retried":          false,
		"one-group-failed": true,
		"still-running":    false,
		"done":             false,
	}, failed)
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

	// A missing submit time stays zero. Turning it into 1970 would show an
	// age of twenty thousand days in the list.
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
	r.True(agent.Server)
}

func TestAgent_AClientAgentReadsItsVersionFromItsConfig(t *testing.T) {
	for name, tc := range map[string]struct{ version, want string }{
		"metadata":            {`{"Version":"1.11.4","VersionPrerelease":"","VersionMetadata":"ent"}`, "1.11.4+ent"},
		"prerelease":          {`{"Version":"2.0.0","VersionPrerelease":"beta.1","VersionMetadata":""}`, "2.0.0-beta.1"},
		"prerelease and meta": {`{"Version":"1.11.4","VersionPrerelease":"dev","VersionMetadata":"ent"}`, "1.11.4-dev+ent"},
	} {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			client, _ := recorder(t, `{"config":{"Region":"eu","Version":`+tc.version+`},"member":{}}`)

			agent, err := client.Agent(context.Background())
			r.NoError(err)

			r.Equal(tc.want, agent.Version)
			r.Equal("eu", agent.Region)
			r.False(agent.Server)
		})
	}
}

func TestAgent_WithoutABuildTag(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `{"member": {"Name": "server-01"}}`)

	agent, err := client.Agent(context.Background())

	// An agent that does not report them gives empty values, not an error
	// the header would have to show.
	r.NoError(err)
	r.Empty(agent.Version)
	r.Empty(agent.Region)
}

// headersOf returns the headers and the query a client sends with a request.
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
	// A missing CA file is an error: the cluster cannot be trusted, and the
	// client does not connect without it.
	_, err := nomad.New(nomad.Config{
		Address: "https://nomad.prod:4646",
		Named:   true,
		TLS:     nomad.TLS{CACert: filepath.Join(t.TempDir(), "missing-ca.pem")},
	})
	require.ErrorContains(t, err, "missing-ca.pem")
}

func TestEveryNamespace(t *testing.T) {
	for namespace, want := range map[string]bool{"": true, "*": true, "default": false, "web": false} {
		require.Equal(t, want, nomad.EveryNamespace(namespace), "namespace %q", namespace)
	}
}

func TestTargetNamespace(t *testing.T) {
	for namespace, want := range map[string]string{"": "default", "*": "default", "default": "default", "web": "web"} {
		require.Equal(t, want, nomad.TargetNamespace(namespace), "namespace %q", namespace)
	}
}

// serviceWithAFailedAlloc is a list that holds a running service of two
// allocations up and one failed.
const serviceWithAFailedAlloc = `[
	{"ID": "web", "Namespace": "default", "Type": "service", "Status": "running",
		"JobSummary": {"Summary": {"web": {"Running": 2, "Failed": 1}}}}
]`

func TestJobs_AServiceAsksForTheCountsOfItsGroups(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{
		"/v1/jobs":          serviceWithAFailedAlloc,
		"/v1/jobs/statuses": `[{"ID": "web", "Namespace": "default", "GroupCountSum": 3}]`,
	})

	jobs, err := client.Jobs(context.Background(), "default")
	r.NoError(err)
	r.Len(jobs, 1)

	r.Equal(2, jobs[0].Running)
	r.Equal(3, jobs[0].Desired)

	r.Len(*asked, 2)
	r.Equal("/v1/jobs/statuses", (*asked)[1].path)
	r.Equal("default", (*asked)[1].namespace)
	r.Equal(`Type == "service"`, (*asked)[1].query.Get("filter"))
}

// A short service read with the numbers of the summary looks whole, so a
// failure that is not a missing endpoint or a refused token is shown.
func TestJobs_AFailedReadingOfTheCountsFailsTheList(t *testing.T) {
	for name, status := range map[string]int{
		"bad request":  http.StatusBadRequest,
		"server error": http.StatusInternalServerError,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.URL.Path == "/v1/jobs/statuses" {
					http.Error(w, "boom", status)

					return
				}

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(serviceWithAFailedAlloc))
			}))
			t.Cleanup(server.Close)

			client, err := nomad.New(nomad.Config{Address: server.URL})
			require.NoError(t, err)

			_, err = client.Jobs(context.Background(), "default")
			require.ErrorContains(t, err, "boom")
		})
	}
}

func TestJobs_ACancelledContextFailsTheList(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The list answers; the context is cancelled while the counts are read.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/v1/jobs/statuses" {
			cancel()
			<-req.Context().Done()

			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(serviceWithAFailedAlloc))
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	require.NoError(t, err)

	_, err = client.Jobs(ctx, "default")
	require.ErrorIs(t, err, context.Canceled)
}

func TestJobs_AClusterWithoutTheCountsKeepsTheSummary(t *testing.T) {
	r := require.New(t)

	client, _ := clusterServer(t, map[string]string{"/v1/jobs": serviceWithAFailedAlloc})

	jobs, err := client.Jobs(context.Background(), "default")
	r.NoError(err)
	r.Equal(2, jobs[0].Desired)
}

func TestJobs_ATokenThatMayNotReadTheCountsKeepsTheSummary(t *testing.T) {
	r := require.New(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/v1/jobs/statuses" {
			http.Error(w, "Permission denied", http.StatusForbidden)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(serviceWithAFailedAlloc))
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	r.NoError(err)

	jobs, err := client.Jobs(context.Background(), "default")
	r.NoError(err)
	r.Equal(2, jobs[0].Desired)
}

func TestJobs_AStoppedServiceKeepsItsSummaryNextToARunningOne(t *testing.T) {
	r := require.New(t)

	client, _ := clusterServer(t, map[string]string{
		"/v1/jobs": `[
			{"ID": "web", "Namespace": "default", "Type": "service", "Status": "running",
				"JobSummary": {"Summary": {"web": {"Running": 2}}}},
			{"ID": "old", "Namespace": "default", "Type": "service", "Status": "dead", "Stop": true,
				"JobSummary": {"Summary": {"old": {"Running": 1}}}}
		]`,
		"/v1/jobs/statuses": `[{"ID": "web", "Namespace": "default", "GroupCountSum": 3},
			{"ID": "old", "Namespace": "default", "GroupCountSum": 3}]`,
	})

	jobs, err := client.Jobs(context.Background(), "default")
	r.NoError(err)

	desired := map[string]int{}
	for _, job := range jobs {
		desired[job.ID] = job.Desired
	}

	r.Equal(map[string]int{"web": 3, "old": 1}, desired)
}

func TestJobs_EachNamespaceTakesTheCountOfItsOwnService(t *testing.T) {
	r := require.New(t)

	client, _ := clusterServer(t, map[string]string{
		"/v1/jobs": `[
			{"ID": "web", "Namespace": "prod", "Type": "service", "Status": "running",
				"JobSummary": {"Summary": {"web": {"Running": 2}}}},
			{"ID": "web", "Namespace": "dev", "Type": "service", "Status": "running",
				"JobSummary": {"Summary": {"web": {"Running": 1}}}}
		]`,
		"/v1/jobs/statuses": `[{"ID": "web", "Namespace": "prod", "GroupCountSum": 3},
			{"ID": "web", "Namespace": "dev", "GroupCountSum": 1}]`,
	})

	jobs, err := client.Jobs(context.Background(), "*")
	r.NoError(err)

	desired := map[string]int{}
	for _, job := range jobs {
		desired[job.Namespace] = job.Desired
	}

	r.Equal(map[string]int{"prod": 3, "dev": 1}, desired)
}

func TestJobs_AServiceTheCountsLeaveOutKeepsItsSummary(t *testing.T) {
	r := require.New(t)

	client, _ := clusterServer(t, map[string]string{
		"/v1/jobs":          serviceWithAFailedAlloc,
		"/v1/jobs/statuses": `[{"ID":"web","Namespace":"default"}]`,
	})

	jobs, err := client.Jobs(context.Background(), "default")
	r.NoError(err)
	r.Equal(2, jobs[0].Desired)
}

func TestJobs_OnlyAServiceThatWasNotStoppedAsksForACount(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{
		"/v1/jobs": `[
			{"ID": "agent", "Namespace": "default", "Type": "system", "Status": "running",
				"JobSummary": {"Summary": {"agent": {"Running": 5}}}},
			{"ID": "old", "Namespace": "default", "Type": "service", "Status": "dead", "Stop": true,
				"JobSummary": {"Summary": {"old": {"Running": 1}}}}
		]`,
		"/v1/jobs/statuses": `[{"ID": "old", "Namespace": "default", "GroupCountSum": 9},
			{"ID": "agent", "Namespace": "default", "GroupCountSum": 9}]`,
	})

	jobs, err := client.Jobs(context.Background(), "default")
	r.NoError(err)

	r.Equal(5, jobs[0].Desired)
	r.Equal(1, jobs[1].Desired)
	r.Len(*asked, 1, "no service to compare, no request")
}
