package nomad_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

const schedulerAnswer = `{"SchedulerConfig": {
	"SchedulerAlgorithm": "spread",
	"PreemptionConfig": {"SystemSchedulerEnabled": true, "SysBatchSchedulerEnabled": false, "BatchSchedulerEnabled": false, "ServiceSchedulerEnabled": true},
	"MemoryOversubscriptionEnabled": true,
	"RejectJobRegistration": false,
	"PauseEvalBroker": false,
	"CreateIndex": 5, "ModifyIndex": 42
}}`

func TestScheduler(t *testing.T) {
	r := require.New(t)

	client, _ := clusterServer(t, map[string]string{"/v1/operator/scheduler/configuration": schedulerAnswer})

	config, err := client.Scheduler(context.Background())
	r.NoError(err)

	r.Equal(nomad.SchedulerConfig{
		Algorithm:              "spread",
		PreemptSystem:          true,
		PreemptService:         true,
		MemoryOversubscription: true,
	}, config)
}

func TestSchedulerSpec(t *testing.T) {
	r := require.New(t)

	client, _ := clusterServer(t, map[string]string{"/v1/operator/scheduler/configuration": schedulerAnswer})

	spec, err := client.SchedulerSpec(context.Background())
	r.NoError(err)

	// The index travels beside the file: it is not a setting.
	r.Contains(spec.Source, `"SchedulerAlgorithm": "spread"`)
	r.NotContains(spec.Source, "ModifyIndex")
	r.NotContains(spec.Source, "CreateIndex")
	r.Equal(uint64(42), spec.Index)
}

func TestSubmitScheduler(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{"/v1/operator/scheduler/configuration": `{"Updated": true}`})

	// The index given wins over the one the file says.
	r.NoError(client.SubmitScheduler(context.Background(), `{"SchedulerAlgorithm": "binpack", "ModifyIndex": 7}`, 42))

	sent := (*asked)[0]
	r.Equal("42", sent.query.Get("cas"))
	r.Equal("binpack", sent.body["SchedulerAlgorithm"])
}

func TestSubmitScheduler_ChangedMeanwhile(t *testing.T) {
	r := require.New(t)

	client, _ := clusterServer(t, map[string]string{"/v1/operator/scheduler/configuration": `{"Updated": false, "SchedulerConfig": {"ModifyIndex": 77}}`})

	err := client.SubmitScheduler(context.Background(), `{"SchedulerAlgorithm": "binpack"}`, 42)
	r.ErrorIs(err, nomad.ErrSchedulerChanged)
	r.ErrorContains(err, "changed since")

	// The next save must be sent at the index the configuration has now.
	var changed *nomad.SchedulerConflict
	r.ErrorAs(err, &changed)
	r.Equal(uint64(77), changed.Index)
}

func TestSubmitScheduler_AConflictItCannotReadAgain(t *testing.T) {
	r := require.New(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPut {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Updated": false}`))

			return
		}

		http.Error(w, "no leader", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	r.NoError(err)

	// The save was refused either way, and there is no index to retry at.
	err = client.SubmitScheduler(context.Background(), `{"SchedulerAlgorithm": "binpack"}`, 42)
	r.ErrorIs(err, nomad.ErrSchedulerChanged)
	r.ErrorContains(err, "no leader")

	var conflict *nomad.SchedulerConflict
	r.False(errors.As(err, &conflict), "%v", err)
}

func TestSubmitScheduler_NotJSON(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{})

	r.ErrorContains(client.SubmitScheduler(context.Background(), `{"SchedulerAlgorithm":`, 42), "not valid JSON")
	r.Empty(*asked)
}
