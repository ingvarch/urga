package nomad_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestBatchQueueJobs_Fifo(t *testing.T) {
	r := require.New(t)

	var asked *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		asked = req
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"Type": "fifo",
			"Workloads": [
				{"JobID": "job-1", "Namespace": "default", "Position": 0, "Status": "placing", "CreatedAt": 1729500000000000000},
				{"JobID": "job-2", "Namespace": "default", "Position": 1, "Status": "queued", "CreatedAt": 1729500001000000000}
			]
		}`))
	}))
	defer srv.Close()

	client, err := nomad.New(nomad.Config{Address: srv.URL})
	r.NoError(err)

	workloads, err := client.BatchQueueJobs(context.Background(), "batch")
	r.NoError(err)
	r.Len(workloads, 2)

	r.Equal("/v1/queue/jobs", asked.URL.Path)
	r.Equal("batch", asked.URL.Query().Get("node_pool"))
	r.Equal("priority", asked.URL.Query().Get("sort"))

	r.Equal("job-1", workloads[0].JobID)
	r.Equal("default", workloads[0].Namespace)
	r.Equal("batch", workloads[0].Pool)
	r.Equal("fifo", workloads[0].QueueType)
	r.Equal(0, workloads[0].Position)
	r.Equal("placing", workloads[0].Status)
	r.Equal(time.Unix(0, 1729500000000000000), workloads[0].CreatedAt)

	r.Equal("job-2", workloads[1].JobID)
	r.Equal("default", workloads[1].Namespace)
	r.Equal("batch", workloads[1].Pool)
	r.Equal(1, workloads[1].Position)
	r.Equal("queued", workloads[1].Status)
}

func TestBatchQueueJobs_DynamicPriority(t *testing.T) {
	r := require.New(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"Type": "dynamic_priority",
			"Workloads": [
				{
					"JobID": "dyn-job",
					"Namespace": "prod",
					"Tenant": "team-a",
					"Position": 0,
					"Status": "placing",
					"AdjustedPriority": 105,
					"BasePriority": 50,
					"FairshareAdjustment": 10,
					"AgeAdjustment": 45,
					"CpuAdjustment": 5,
					"MemoryAdjustment": -5,
					"CreatedAt": 1729500000
				}
			]
		}`))
	}))
	defer srv.Close()

	client, err := nomad.New(nomad.Config{Address: srv.URL})
	r.NoError(err)

	workloads, err := client.BatchQueueJobs(context.Background(), "gpu")
	r.NoError(err)
	r.Len(workloads, 1)

	w := workloads[0]
	r.Equal("dyn-job", w.JobID)
	r.Equal("prod", w.Namespace)
	r.Equal("gpu", w.Pool)
	r.Equal("dynamic_priority", w.QueueType)
	r.Equal("team-a", w.Tenant)
	r.Equal(0, w.Position)
	r.Equal("placing", w.Status)
	r.Equal(105, w.AdjustedPriority)
	r.Equal(50, w.BasePriority)
	r.Equal(10, w.FairshareAdjustment)
	r.Equal(45, w.AgeAdjustment)
	r.Equal(5, w.CPUAdjustment)
	r.Equal(-5, w.MemoryAdjustment)
	r.Equal(time.Unix(1729500000, 0), w.CreatedAt)
}

func TestBatchQueueJobs_Pagination(t *testing.T) {
	r := require.New(t)

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Nomad-Index", "1")
		w.Header().Set("X-Nomad-LastContact", "0")
		if req.URL.Query().Get("next_token") == "" {
			w.Header().Set("X-Nomad-NextToken", "token-2")
			_, _ = w.Write([]byte(`{
				"Type": "fifo",
				"Workloads": [{"JobID": "job-1", "Namespace": "default", "Position": 0, "Status": "placing"}]
			}`))
			return
		}

		_, _ = w.Write([]byte(`{
			"Type": "fifo",
			"Workloads": [{"JobID": "job-2", "Namespace": "default", "Position": 1, "Status": "queued"}]
		}`))
	}))
	defer srv.Close()

	client, err := nomad.New(nomad.Config{Address: srv.URL})
	r.NoError(err)

	workloads, err := client.BatchQueueJobs(context.Background(), "batch")
	r.NoError(err)
	r.Len(workloads, 2)
	r.Equal(2, calls)
	r.Equal("job-1", workloads[0].JobID)
	r.Equal("job-2", workloads[1].JobID)
}

func TestBatchQueueJobs_Unsupported(t *testing.T) {
	r := require.New(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client, err := nomad.New(nomad.Config{Address: srv.URL})
	r.NoError(err)

	workloads, err := client.BatchQueueJobs(context.Background(), "batch")
	r.ErrorIs(err, nomad.ErrQueueUnsupported)
	r.Nil(workloads)
}

func TestBatchQueueJobs_Forbidden(t *testing.T) {
	r := require.New(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	client, err := nomad.New(nomad.Config{Address: srv.URL})
	r.NoError(err)

	workloads, err := client.BatchQueueJobs(context.Background(), "batch")
	r.NoError(err)
	r.Nil(workloads)
}

func TestBatchQueueStatus_DynamicPriority(t *testing.T) {
	r := require.New(t)

	var asked *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		asked = req
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"Type": "dynamic_priority",
			"Status": [
				{
					"TenantID": "team-a",
					"PercentageUsed": 75,
					"TenantFairshare": {"cpu": 600, "memory": 256},
					"TotalFairshare": {"cpu": 800, "memory": 512}
				}
			]
		}`))
	}))
	defer srv.Close()

	client, err := nomad.New(nomad.Config{Address: srv.URL})
	r.NoError(err)

	tenants, err := client.BatchQueueStatus(context.Background(), "gpu")
	r.NoError(err)
	r.Len(tenants, 1)

	r.Equal("/v1/queue/status", asked.URL.Path)
	r.Equal("gpu", asked.URL.Query().Get("node_pool"))

	tnt := tenants[0]
	r.Equal("team-a", tnt.ID)
	r.Equal(75, tnt.PercentageUsed)
	r.Equal(600.0, tnt.Fairshare["cpu"])
	r.Equal(256.0, tnt.Fairshare["memory"])
}

func TestBatchQueueStatus_UnsupportedAndForbidden(t *testing.T) {
	r := require.New(t)

	for _, status := range []int{http.StatusNotFound, http.StatusForbidden} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))

		client, err := nomad.New(nomad.Config{Address: srv.URL})
		r.NoError(err)

		tenants, err := client.BatchQueueStatus(context.Background(), "gpu")
		r.NoError(err)
		r.Nil(tenants)

		srv.Close()
	}
}

func TestBatchQueueStatus_Passthrough(t *testing.T) {
	r := require.New(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Type": "passthrough", "Status": null}`))
	}))
	defer srv.Close()

	client, err := nomad.New(nomad.Config{Address: srv.URL})
	r.NoError(err)

	tenants, err := client.BatchQueueStatus(context.Background(), "default")
	r.NoError(err)
	r.Nil(tenants)
}

func TestBatchQueueJobs_EmptyPool(t *testing.T) {
	r := require.New(t)

	client, err := nomad.New(nomad.Config{Address: "http://127.0.0.1:4646"})
	r.NoError(err)

	workloads, err := client.BatchQueueJobs(context.Background(), "")
	r.NoError(err)
	r.Nil(workloads)

	tenants, err := client.BatchQueueStatus(context.Background(), "")
	r.NoError(err)
	r.Nil(tenants)
}

func TestBatchQueueJobs_ServerError(t *testing.T) {
	r := require.New(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client, err := nomad.New(nomad.Config{Address: srv.URL})
	r.NoError(err)

	_, err = client.BatchQueueJobs(context.Background(), "batch")
	r.Error(err)

	_, err = client.BatchQueueStatus(context.Background(), "batch")
	r.Error(err)
}
