package nomad_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// indexed answers each path with its body (an empty list when it has none)
// at the index the path is given, 0 for none.
func indexed(t *testing.T, indexes map[string]uint64, bodies map[string]string) *nomad.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if index := indexes[r.URL.Path]; index != 0 {
			w.Header().Set("X-Nomad-Index", strconv.FormatUint(index, 10))
		}

		w.Header().Set("Content-Type", "application/json")

		body, ok := bodies[r.URL.Path]
		if !ok {
			body = `[]`
		}

		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	require.NoError(t, err)

	return client
}

func TestReads_ARequestKeepsTheIndexOfEachTopicItRead(t *testing.T) {
	r := require.New(t)

	client := indexed(t, map[string]uint64{"/v1/jobs": 42, "/v1/allocations": 57}, nil)

	ctx, reads := nomad.WithReads(context.Background())

	_, err := client.Jobs(ctx, "default")
	r.NoError(err)

	_, err = client.Allocations(ctx, "default", "")
	r.NoError(err)

	r.Equal(map[string]uint64{nomad.TopicJob: 42, nomad.TopicAllocation: 57}, reads.Indexes())
}

func TestReads_ATopicReadTwiceKeepsTheLowestIndex(t *testing.T) {
	r := require.New(t)

	index := uint64(30)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Nomad-Index", strconv.FormatUint(index, 10))
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	client, err := nomad.New(nomad.Config{Address: server.URL})
	r.NoError(err)

	ctx, reads := nomad.WithReads(context.Background())

	_, err = client.Jobs(ctx, "default")
	r.NoError(err)

	index = 20

	_, err = client.Jobs(ctx, "default")
	r.NoError(err)

	r.Equal(map[string]uint64{nomad.TopicJob: 20}, reads.Indexes())
}

func TestReads_AnAnswerWithoutAnIndexNotesNothing(t *testing.T) {
	r := require.New(t)

	client := indexed(t, nil, nil)

	ctx, reads := nomad.WithReads(context.Background())

	_, err := client.Jobs(ctx, "default")
	r.NoError(err)
	r.Empty(reads.Indexes())
}

func TestReads_AFailedReadNotesNothing(t *testing.T) {
	r := require.New(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Nomad-Index", "9")
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := nomad.New(nomad.Config{Address: server.URL})
	r.NoError(err)

	ctx, reads := nomad.WithReads(context.Background())

	_, err = client.Jobs(ctx, "default")
	r.Error(err)
	r.Empty(reads.Indexes())
}

func TestReads_AReadWithoutAHolderAnswersAsBefore(t *testing.T) {
	r := require.New(t)

	client := indexed(t, map[string]uint64{"/v1/jobs": 42}, map[string]string{
		"/v1/jobs": `[{"ID": "web", "Name": "web"}]`,
	})

	jobs, err := client.Jobs(context.Background(), "default")
	r.NoError(err)
	r.Len(jobs, 1)
}

// The index of the counts is the newest change of the services on the page
// and of their allocations, not of the jobs: a newer change of another job
// is in the list already.
func TestReads_TheCountsOfTheServicesKeepTheIndexOfTheList(t *testing.T) {
	r := require.New(t)

	client := indexed(t, map[string]uint64{"/v1/jobs": 50, "/v1/jobs/statuses": 45}, map[string]string{
		"/v1/jobs": `[{"ID": "web", "Name": "web", "Type": "service"}]`,
	})

	ctx, reads := nomad.WithReads(context.Background())

	_, err := client.Jobs(ctx, "default")
	r.NoError(err)

	r.Equal(map[string]uint64{nomad.TopicJob: 50}, reads.Indexes())
}

func TestReads_TheScaleStatusOfAGroupNotesItsIndexToo(t *testing.T) {
	r := require.New(t)

	client := indexed(t, map[string]uint64{"/v1/job/web": 30, "/v1/job/web/scale": 25}, map[string]string{
		"/v1/job/web":       `{}`,
		"/v1/job/web/scale": `{}`,
	})

	ctx, reads := nomad.WithReads(context.Background())

	_, err := client.GroupScaling(ctx, "default", "web", "g")
	r.NoError(err)

	r.Equal(map[string]uint64{nomad.TopicJob: 25}, reads.Indexes())
}

func TestReads_ClientsShownWithoutALimitDependOnNoAllocation(t *testing.T) {
	limited := map[string]int{"node-1": 2}

	for name, cluster := range map[string]*limitsCluster{
		"no client has a limit":              {limits: map[string]int{}},
		"the cluster refuses the filter":     {listStatus: http.StatusBadRequest},
		"the token may not list allocations": {limits: limited, allocsStatus: http.StatusForbidden},
	} {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			ctx, reads := nomad.WithReads(context.Background())

			_, err := cluster.client(t).WithAllocLimits(ctx, []nomad.Node{{ID: "node-1"}})
			r.NoError(err)

			// No allocation reaches the list, so no change of one is newer
			// than it.
			r.Equal(map[string]uint64{nomad.TopicAllocation: nomad.EveryIndex}, reads.Indexes())
		})
	}
}

// readCase is a read and the topics it notes.
type readCase struct {
	name   string
	topics []string
	bodies map[string]string
	call   func(context.Context, *nomad.Client) error
}

func jobReads() []readCase {
	object := `{}`

	return []readCase{
		{"jobs", []string{nomad.TopicJob}, nil, func(ctx context.Context, c *nomad.Client) error { return errOf(c.Jobs(ctx, "default")) }},
		{"pool jobs", []string{nomad.TopicJob}, nil, func(ctx context.Context, c *nomad.Client) error { return errOf(c.PoolJobs(ctx, "p")) }},
		{"job meta", []string{nomad.TopicJob}, map[string]string{"/v1/job/web": object}, func(ctx context.Context, c *nomad.Client) error { return errOf(c.JobMeta(ctx, "default", "web")) }},
		{"job versions", []string{nomad.TopicJob}, map[string]string{"/v1/job/web/versions": object}, func(ctx context.Context, c *nomad.Client) error { return errOf(c.JobVersions(ctx, "default", "web")) }},
		{"group scaling", []string{nomad.TopicJob}, map[string]string{"/v1/job/web": object, "/v1/job/web/scale": object}, func(ctx context.Context, c *nomad.Client) error {
			return errOf(c.GroupScaling(ctx, "default", "web", "g"))
		}},
		{"scaling policies", []string{nomad.TopicJob}, nil, func(ctx context.Context, c *nomad.Client) error { return errOf(c.ScalingPolicies(ctx, "default")) }},
		{"batch queue", []string{nomad.TopicJob}, map[string]string{"/v1/queue/jobs": object}, func(ctx context.Context, c *nomad.Client) error { return errOf(c.BatchQueueJobs(ctx, "p")) }},
	}
}

func allocReads() []readCase {
	object := `{}`

	return []readCase{
		{"allocations", []string{nomad.TopicAllocation}, nil, func(ctx context.Context, c *nomad.Client) error { return errOf(c.Allocations(ctx, "default", "")) }},
		{"job allocations", []string{nomad.TopicAllocation}, nil, func(ctx context.Context, c *nomad.Client) error { return errOf(c.Allocations(ctx, "default", "web")) }},
		{"node allocations", []string{nomad.TopicAllocation}, nil, func(ctx context.Context, c *nomad.Client) error { return errOf(c.NodeAllocations(ctx, "n1")) }},
		{"deployment allocations", []string{nomad.TopicAllocation}, nil, func(ctx context.Context, c *nomad.Client) error {
			return errOf(c.DeploymentAllocations(ctx, "default", "d1"))
		}},
		{"evaluation allocations", []string{nomad.TopicAllocation}, nil, func(ctx context.Context, c *nomad.Client) error {
			return errOf(c.EvaluationAllocations(ctx, "default", "e1"))
		}},
		{"allocation", []string{nomad.TopicAllocation}, map[string]string{"/v1/allocation/a1": object}, func(ctx context.Context, c *nomad.Client) error { return errOf(c.Allocation(ctx, "default", "a1")) }},
		{"alloc limits", []string{nomad.TopicAllocation}, map[string]string{
			"/v1/nodes":       `[{"ID": "node-1"}]`,
			"/v1/node/node-1": `{"NodeMaxAllocs": 2}`,
		}, func(ctx context.Context, c *nomad.Client) error {
			return errOf(c.WithAllocLimits(ctx, []nomad.Node{{ID: "node-1"}}))
		}},
		{"csi volume", []string{nomad.TopicCSIVolume, nomad.TopicCSIPlugin, nomad.TopicAllocation}, map[string]string{"/v1/volume/csi/v1": object}, func(ctx context.Context, c *nomad.Client) error {
			return errOf(c.Volume(ctx, "default", nomad.VolumeCSI, "v1"))
		}},
		{"host volume", []string{nomad.TopicHostVolume, nomad.TopicAllocation}, map[string]string{"/v1/volume/host/v1": object}, func(ctx context.Context, c *nomad.Client) error {
			return errOf(c.Volume(ctx, "default", nomad.VolumeHost, "v1"))
		}},
	}
}

func otherReads() []readCase {
	object := `{}`

	return []readCase{
		{"deployments", []string{nomad.TopicDeployment}, nil, func(ctx context.Context, c *nomad.Client) error { return errOf(c.Deployments(ctx, "default")) }},
		{"deployment", []string{nomad.TopicDeployment}, map[string]string{"/v1/deployment/d1": object}, func(ctx context.Context, c *nomad.Client) error { return errOf(c.Deployment(ctx, "default", "d1")) }},
		{"job deployments", []string{nomad.TopicDeployment}, nil, func(ctx context.Context, c *nomad.Client) error {
			return errOf(c.JobDeployments(ctx, "default", "web"))
		}},
		{"evaluations", []string{nomad.TopicEvaluation}, nil, func(ctx context.Context, c *nomad.Client) error { return errOf(c.Evaluations(ctx, "default")) }},
		{"newest evaluation", []string{nomad.TopicEvaluation}, nil, func(ctx context.Context, c *nomad.Client) error {
			return errOf(c.NewestEvaluation(ctx, "default", "web"))
		}},
		{"services", []string{nomad.TopicService}, nil, func(ctx context.Context, c *nomad.Client) error { return errOf(c.Services(ctx, "default")) }},
		{"service instances", []string{nomad.TopicService}, nil, func(ctx context.Context, c *nomad.Client) error {
			return errOf(c.ServiceInstances(ctx, "default", "db"))
		}},
		{"nodes", []string{nomad.TopicNode}, nil, func(ctx context.Context, c *nomad.Client) error { return errOf(c.Nodes(ctx)) }},
		{"pool nodes", []string{nomad.TopicNode}, nil, func(ctx context.Context, c *nomad.Client) error { return errOf(c.PoolNodes(ctx, "p")) }},
		{"node", []string{nomad.TopicNode}, map[string]string{"/v1/node/n1": object}, func(ctx context.Context, c *nomad.Client) error { return errOf(c.Node(ctx, "n1")) }},
		{"node pools", []string{nomad.TopicNodePool}, nil, func(ctx context.Context, c *nomad.Client) error { return errOf(c.NodePools(ctx)) }},
		{"plugins", []string{nomad.TopicCSIPlugin}, nil, func(ctx context.Context, c *nomad.Client) error { return errOf(c.Plugins(ctx)) }},
		{"plugin", []string{nomad.TopicCSIPlugin}, map[string]string{"/v1/plugin/csi/p1": object}, func(ctx context.Context, c *nomad.Client) error { return errOf(c.Plugin(ctx, "p1")) }},
		{"volumes", []string{nomad.TopicCSIVolume, nomad.TopicCSIPlugin}, nil, func(ctx context.Context, c *nomad.Client) error { return errOf(c.Volumes(ctx, "default")) }},
		{"variables", []string{nomad.TopicVariable}, nil, func(ctx context.Context, c *nomad.Client) error { return errOf(c.Variables(ctx, "default")) }},
		{"variable", []string{nomad.TopicVariable}, map[string]string{"/v1/var/a/b": object}, func(ctx context.Context, c *nomad.Client) error { return errOf(c.Variable(ctx, "default", "a/b")) }},
	}
}

func TestReads_EachReadOfAWatchedScreenNotesItsTopics(t *testing.T) {
	const at = 7

	tests := slices.Concat(jobReads(), allocReads(), otherReads())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := require.New(t)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-Nomad-Index", strconv.Itoa(at))
				w.Header().Set("Content-Type", "application/json")

				body, ok := tt.bodies[req.URL.Path]
				if !ok {
					body = `[]`
				}

				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()

			client, err := nomad.New(nomad.Config{Address: server.URL})
			r.NoError(err)

			ctx, reads := nomad.WithReads(context.Background())

			r.NoError(tt.call(ctx, client))

			want := map[string]uint64{}
			for _, topic := range tt.topics {
				want[topic] = at
			}

			r.Equal(want, reads.Indexes())
		})
	}
}
