package nomad

import (
	"context"
	"maps"
	"slices"

	"github.com/hashicorp/nomad/api"
)

// EvaluationDetail is one scheduling decision in full: what it was about, how
// it ended, and why what it could not place was not placed.
type EvaluationDetail struct {
	Evaluation

	StatusDescription string

	// BlockedEval waits for room to place what this one could not;
	// PreviousEval and NextEval are the decisions before and after it.
	BlockedEval  string
	PreviousEval string
	NextEval     string

	// Related are the other decisions of the same chain.
	Related []Evaluation

	// Failures are the task groups it could not place, by name.
	Failures []PlacementFailure
}

// PlacementFailure is why the allocations of one task group were not placed:
// how many nodes were looked at, which were filtered out and by what, and
// which ran out of room.
type PlacementFailure struct {
	Group string

	// Unplaced is how many allocations of the group were not placed.
	Unplaced int

	NodesEvaluated int

	// NodesAvailable is how many nodes each datacenter of the job has.
	NodesAvailable map[string]int

	ClassFiltered      map[string]int
	ConstraintFiltered map[string]int

	NodesExhausted     int
	ClassExhausted     map[string]int
	DimensionExhausted map[string]int

	QuotaExhausted []string
}

// Evaluation is one scheduling decision, with why it placed nothing when it
// did not.
func (c *Client) Evaluation(ctx context.Context, namespace, evalID string) (EvaluationDetail, error) {
	eval, _, err := c.api.Evaluations().Info(evalID, c.query(ctx, namespace))
	if err != nil {
		return EvaluationDetail{}, err
	}

	return newEvaluationDetail(eval), nil
}

func newEvaluationDetail(eval *api.Evaluation) EvaluationDetail {
	detail := EvaluationDetail{
		Evaluation:        newEvaluation(eval),
		StatusDescription: eval.StatusDescription,
		BlockedEval:       eval.BlockedEval,
		PreviousEval:      eval.PreviousEval,
		NextEval:          eval.NextEval,
		Failures:          newPlacementFailures(eval.FailedTGAllocs),
	}

	for _, stub := range eval.RelatedEvals {
		if stub == nil {
			continue
		}

		detail.Related = append(detail.Related, Evaluation{
			ID:          stub.ID,
			JobID:       stub.JobID,
			Namespace:   stub.Namespace,
			Type:        stub.Type,
			TriggeredBy: stub.TriggeredBy,
			Status:      stub.Status,
			Created:     unixTime(stub.CreateTime),
		})
	}

	return detail
}

// newPlacementFailures puts the groups in the order of their names. A map
// hands them over in a different order every time.
func newPlacementFailures(metrics map[string]*api.AllocationMetric) []PlacementFailure {
	failures := make([]PlacementFailure, 0, len(metrics))

	for _, group := range slices.Sorted(maps.Keys(metrics)) {
		metric := metrics[group]
		if metric == nil {
			continue
		}

		failures = append(failures, PlacementFailure{
			Group: group,

			// The failures folded into this one are allocations too.
			Unplaced: metric.CoalescedFailures + 1,

			NodesEvaluated:     metric.NodesEvaluated,
			NodesAvailable:     metric.NodesAvailable,
			ClassFiltered:      metric.ClassFiltered,
			ConstraintFiltered: metric.ConstraintFiltered,
			NodesExhausted:     metric.NodesExhausted,
			ClassExhausted:     metric.ClassExhausted,
			DimensionExhausted: metric.DimensionExhausted,
			QuotaExhausted:     metric.QuotaExhausted,
		})
	}

	return failures
}
