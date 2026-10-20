package nomad

import (
	"context"
	"errors"
	"maps"
	"slices"

	"github.com/hashicorp/nomad/api"
)

// EvaluationDetail is one scheduling decision in full: what it was about, how
// it ended, and why what it could not place was not placed.
type EvaluationDetail struct {
	Evaluation

	StatusDescription string

	// CreateIndex is the index of the cluster when the evaluation was made.
	CreateIndex uint64

	// BlockedEval waits for room to place what this one could not;
	// PreviousEval and NextEval are the decisions before and after it.
	BlockedEval  string
	PreviousEval string
	NextEval     string

	// Related are the other decisions of the same chain.
	Related []Evaluation

	// Failures are the task groups it could not place, by name.
	Failures []PlacementFailure

	// Groups are what it planned for each group, and Preempted the
	// allocations of other jobs it preempted. A cluster older than Nomad
	// 1.11 sends neither.
	Groups    []PlanGroup
	Preempted []Alloc
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
		CreateIndex:       eval.CreateIndex,
		BlockedEval:       eval.BlockedEval,
		PreviousEval:      eval.PreviousEval,
		NextEval:          eval.NextEval,
		Failures:          newPlacementFailures(eval.FailedTGAllocs),
	}

	if eval.PlanAnnotations != nil {
		detail.Groups = planGroups(eval.PlanAnnotations.DesiredTGUpdates)
		detail.Preempted = allocsOf(eval.PlanAnnotations.PreemptedAllocs)
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

			// The failures merged into this one are unplaced allocations too.
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

// ErrNoFailures says no evaluation of the job failed to place anything.
var ErrNoFailures = errors.New("no evaluation of the job failed to place anything")

// FailedPlacement is the newest evaluation of the job that failed to place
// something. The nomad command explains unplaced allocations with it too. A
// newer evaluation that placed everything else explains nothing about the
// allocations that still wait.
func (c *Client) FailedPlacement(ctx context.Context, namespace, jobID string) (EvaluationDetail, error) {
	evals, _, err := c.api.Jobs().Evaluations(jobID, c.query(ctx, namespace))
	if err != nil {
		return EvaluationDetail{}, err
	}

	newest := newestOf(evals, func(eval *api.Evaluation) bool { return len(eval.FailedTGAllocs) > 0 })

	if newest == nil {
		return EvaluationDetail{}, ErrNoFailures
	}

	return newEvaluationDetail(newest), nil
}

// NewestEvaluation is the newest evaluation of the job, nil when it has none.
func (c *Client) NewestEvaluation(ctx context.Context, namespace, jobID string) (*EvaluationDetail, error) {
	evals, _, err := c.api.Jobs().Evaluations(jobID, c.query(ctx, namespace))
	if err != nil {
		return nil, err
	}

	newest := newestOf(evals, func(*api.Evaluation) bool { return true })
	if newest == nil {
		return nil, nil
	}

	detail := newEvaluationDetail(newest)

	return &detail, nil
}

// newestOf is the evaluation with the highest create index among those that
// pass wanted, nil when none does.
func newestOf(evals []*api.Evaluation, wanted func(*api.Evaluation) bool) *api.Evaluation {
	var newest *api.Evaluation

	for _, eval := range evals {
		if eval == nil || !wanted(eval) {
			continue
		}

		if newest == nil || eval.CreateIndex > newest.CreateIndex {
			newest = eval
		}
	}

	return newest
}

// EvaluationAllocations are the allocations an evaluation placed or changed.
func (c *Client) EvaluationAllocations(ctx context.Context, namespace, evalID string) ([]Alloc, error) {
	stubs, _, err := c.api.Evaluations().Allocations(evalID, c.query(ctx, namespace))
	if err != nil {
		return nil, err
	}

	return allocsOf(stubs), nil
}
