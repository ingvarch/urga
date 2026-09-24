package nomad

import (
	"context"
	"errors"
	"fmt"
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
	// Diff is what would change in the job, laid out as the job file.
	Diff []DiffLine

	// Groups are what the scheduler would do to each group, by name.
	Groups []PlanGroup

	// Failures are the groups it could not place.
	Failures []PlacementFailure

	Warnings string

	// Index is the one the job had when it was planned. Submitting at it
	// fails when the job changed in between.
	Index uint64

	// To is the version a revert goes back to, and Version the one the job
	// had when the revert was planned.
	To      uint64
	Version uint64
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

	plan.Diff = hclDiff(answer.Diff)

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

// jobChanged says the cluster refused a change planned on a job that has
// moved on since: a register at an index, or a revert from a version, that
// are no longer the job's.
func jobChanged(err error) bool {
	var answer api.UnexpectedResponseError
	if !errors.As(err, &answer) {
		return false
	}

	body := answer.Body()

	return strings.Contains(body, "conflicting job modify index") || strings.Contains(body, "; enforcing version")
}

// PlanRevert asks the cluster what going back to a version of the job would
// do, which is submitting that version again. No version given is the one
// before the version that runs.
func (c *Client) PlanRevert(ctx context.Context, namespace, jobID string, to *uint64) (Plan, error) {
	versions, _, err := c.versions(ctx, namespace, jobID)
	if err != nil {
		return Plan{}, err
	}

	// The versions come newest first: the first is the one that runs.
	if len(versions) == 0 {
		return Plan{}, fmt.Errorf("%s has no versions", jobID)
	}

	current := uintOf(versions[0].Version)

	if to == nil {
		if current == 0 {
			return Plan{}, fmt.Errorf("%s has no earlier version to go back to", jobID)
		}

		previous := current - 1
		to = &previous
	}

	i := slices.IndexFunc(versions, func(job *api.Job) bool { return uintOf(job.Version) == *to })
	if i < 0 {
		return Plan{}, fmt.Errorf("%s has no version %d", jobID, *to)
	}

	answer, _, err := c.api.Jobs().PlanOpts(versions[i], &api.PlanOptions{Diff: true}, c.write(ctx, namespace))
	if err != nil {
		return Plan{}, err
	}

	plan := newPlan(answer)
	plan.To, plan.Version = *to, current

	return plan, nil
}
