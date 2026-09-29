package nomad_test

import (
	"context"
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

	// The configuration as the cluster keeps it, with the index it was read
	// at: saving checks that nobody changed it since.
	r.Contains(spec, `"SchedulerAlgorithm": "spread"`)
	r.Contains(spec, `"ModifyIndex": 42`)
}

func TestSubmitScheduler(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{"/v1/operator/scheduler/configuration": `{"Updated": true}`})

	r.NoError(client.SubmitScheduler(context.Background(), `{"SchedulerAlgorithm": "binpack", "ModifyIndex": 42}`))

	sent := (*asked)[0]
	r.Equal("42", sent.query.Get("cas"))
	r.Equal("binpack", sent.body["SchedulerAlgorithm"])
}

func TestSubmitScheduler_ChangedMeanwhile(t *testing.T) {
	r := require.New(t)

	client, _ := clusterServer(t, map[string]string{"/v1/operator/scheduler/configuration": `{"Updated": false}`})

	err := client.SubmitScheduler(context.Background(), `{"SchedulerAlgorithm": "binpack", "ModifyIndex": 42}`)
	r.ErrorContains(err, "changed since")
}

func TestSubmitScheduler_NotJSON(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{})

	r.ErrorContains(client.SubmitScheduler(context.Background(), `{"SchedulerAlgorithm":`), "not valid JSON")
	r.Empty(*asked)
}
