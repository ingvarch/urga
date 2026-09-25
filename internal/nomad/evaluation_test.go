package nomad_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// blockedEvaluation is what the cluster returns for an evaluation that could
// not place everything: two groups, each short of something else.
const blockedEvaluation = `{
	"ID": "1ad88fd0-6b1c-da67-5e8f-482b5a15af1f",
	"Namespace": "production",
	"JobID": "web",
	"Type": "service",
	"TriggeredBy": "queued-allocs",
	"Status": "blocked",
	"StatusDescription": "created to place remaining allocations",
	"PreviousEval": "15d164c9-0000-0000-0000-000000000000",
	"CreateTime": 1758499200000000000,
	"FailedTGAllocs": {
		"web": {
			"NodesEvaluated": 3,
			"NodesAvailable": {"dc1": 3, "dc2": 0},
			"ConstraintFiltered": {"${attr.kernel.name} = linux": 1},
			"NodesExhausted": 2,
			"DimensionExhausted": {"memory": 2},
			"CoalescedFailures": 2
		},
		"api": {
			"NodesEvaluated": 0,
			"QuotaExhausted": ["cpu exhausted (500 needed > 400 limit)"]
		}
	},
	"RelatedEvals": [
		{"ID": "15d164c9-0000-0000-0000-000000000000", "JobID": "web", "Namespace": "production",
		 "TriggeredBy": "job-register", "Status": "complete"}
	]
}`

func TestEvaluation(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, blockedEvaluation)

	eval, err := client.Evaluation(context.Background(), "production", "1ad88fd0-6b1c-da67-5e8f-482b5a15af1f")
	r.NoError(err)

	r.Equal("/v1/evaluation/1ad88fd0-6b1c-da67-5e8f-482b5a15af1f", asked.URL.Path)
	r.Equal("production", asked.URL.Query().Get("namespace"))

	r.Equal("web", eval.JobID)
	r.Equal("blocked", eval.Status)
	r.Equal("created to place remaining allocations", eval.StatusDescription)
	r.Equal("queued-allocs", eval.TriggeredBy)
	r.Equal("15d164c9-0000-0000-0000-000000000000", eval.PreviousEval)
	r.False(eval.Created.IsZero())

	r.Len(eval.Related, 1)
	r.Equal("job-register", eval.Related[0].TriggeredBy)
}

func TestEvaluation_SaysWhyEachGroupWasNotPlaced(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, blockedEvaluation)

	eval, err := client.Evaluation(context.Background(), "production", "1ad88fd0")
	r.NoError(err)

	// The groups come in the order of their names: a map hands them over in
	// a different order every time.
	r.Len(eval.Failures, 2)
	r.Equal("api", eval.Failures[0].Group)
	r.Equal("web", eval.Failures[1].Group)

	web := eval.Failures[1]

	// The failures Nomad coalesced into this one are unplaced allocations too.
	r.Equal(3, web.Unplaced)
	r.Equal(3, web.NodesEvaluated)
	r.Equal(map[string]int{"dc1": 3, "dc2": 0}, web.NodesAvailable)
	r.Equal(map[string]int{"${attr.kernel.name} = linux": 1}, web.ConstraintFiltered)
	r.Equal(2, web.NodesExhausted)
	r.Equal(map[string]int{"memory": 2}, web.DimensionExhausted)

	r.Equal([]string{"cpu exhausted (500 needed > 400 limit)"}, eval.Failures[0].QuotaExhausted)
}

func TestFailedPlacement_TheNewestEvaluationWithFailures(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `[
		{"ID": "old", "JobID": "web", "Status": "complete", "CreateIndex": 5,
		 "FailedTGAllocs": {"web": {"NodesEvaluated": 1}}},
		{"ID": "blocked", "JobID": "web", "Status": "blocked", "CreateIndex": 20,
		 "FailedTGAllocs": {"web": {"NodesEvaluated": 3, "NodesExhausted": 3}}},
		{"ID": "placed", "JobID": "web", "Status": "complete", "CreateIndex": 30}
	]`)

	eval, err := client.FailedPlacement(context.Background(), "production", "web")
	r.NoError(err)

	r.Equal("/v1/job/web/evaluations", asked.URL.Path)
	r.Equal("production", asked.URL.Query().Get("namespace"))

	// The newest evaluation that failed to place something explains why the
	// job waits now; the nomad command picks the same one. A newer one that
	// placed everything else does not explain what still waits.
	r.Equal("blocked", eval.ID)
	r.Equal(3, eval.Failures[0].NodesExhausted)
}

func TestFailedPlacement_WhenNoneFailed(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `[{"ID": "placed", "JobID": "web", "Status": "complete", "CreateIndex": 30}]`)

	_, err := client.FailedPlacement(context.Background(), "production", "web")
	r.ErrorIs(err, nomad.ErrNoFailures)
}
