package nomad

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"

	"github.com/hashicorp/nomad/api"
)

// ErrJobChanged says the job changed after it was planned: what the plan
// showed is no longer what submitting would do.
var ErrJobChanged = errors.New("the job changed since it was planned")

// Plan is what submitting a job would do, asked of the cluster without doing
// it.
type Plan struct {
	// Diff is what would change in the job, in the shape of a version diff.
	Diff string

	// Groups are what the scheduler would do to each group, by name.
	Groups []PlanGroup

	// Failures are the groups it could not place.
	Failures []PlacementFailure

	Warnings string

	// Index is the one the job had when it was planned. Submitting at it
	// fails when the job changed in between.
	Index uint64
}

// PlanGroup is what the scheduler would do to the allocations of a group.
type PlanGroup struct {
	Name string

	Place       int
	Stop        int
	InPlace     int
	Destructive int
	Canary      int
	Migrate     int
	Preemptions int
}

// PlanJob asks the cluster what submitting the job file would do.
func (c *Client) PlanJob(ctx context.Context, namespace, source string, vars JobVariables) (Plan, error) {
	job, _, err := c.jobOf(ctx, namespace, source, vars)
	if err != nil {
		return Plan{}, err
	}

	answer, _, err := c.api.Jobs().PlanOpts(job, &api.PlanOptions{Diff: true}, c.write(ctx, namespace))
	if err != nil {
		return Plan{}, err
	}

	return newPlan(answer), nil
}

func newPlan(answer *api.JobPlanResponse) Plan {
	plan := Plan{
		Failures: newPlacementFailures(answer.FailedTGAllocs),
		Warnings: answer.Warnings,
		Index:    answer.JobModifyIndex,
	}

	if answer.Diff != nil {
		plan.Diff = writeJobDiff(answer.Diff)
	}

	if answer.Annotations == nil {
		return plan
	}

	updates := answer.Annotations.DesiredTGUpdates

	for _, name := range slices.Sorted(maps.Keys(updates)) {
		update := updates[name]
		if update == nil {
			continue
		}

		plan.Groups = append(plan.Groups, PlanGroup{
			Name:        name,
			Place:       int(update.Place),
			Stop:        int(update.Stop),
			InPlace:     int(update.InPlaceUpdate),
			Destructive: int(update.DestructiveUpdate),
			Canary:      int(update.Canary),
			Migrate:     int(update.Migrate),
			Preemptions: int(update.Preemptions),
		})
	}

	return plan
}

// jobChanged says the cluster refused a job for an index that is no longer
// the one the job has.
func jobChanged(err error) bool {
	var answer api.UnexpectedResponseError

	return errors.As(err, &answer) && strings.Contains(answer.Body(), "conflicting job modify index")
}
