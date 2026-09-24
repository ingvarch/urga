package nomad

import (
	"context"
	"sort"
	"time"
)

// Active says the deployment is not over: it can still be promoted, paused
// or failed, and its deadlines still count.
func (d Deployment) Active() bool {
	switch d.Status {
	case "successful", "failed", "cancelled":
		return false
	}

	return true
}

// DeploymentDetail is a deployment with where each of its task groups is.
type DeploymentDetail struct {
	Deployment

	Groups []DeploymentGroup
}

// DeploymentGroup is how far a deployment has got with one task group.
type DeploymentGroup struct {
	Name string

	DesiredTotal int
	Placed       int
	Healthy      int
	Unhealthy    int

	DesiredCanaries int
	PlacedCanaries  int
	Promoted        bool
	AutoRevert      bool

	// ProgressDeadline is how long an allocation has to become healthy;
	// RequireProgressBy is when the deployment fails if none does.
	ProgressDeadline  time.Duration
	RequireProgressBy time.Time
}

// WaitsForPromotion says the group has canaries that have not been promoted.
func (g DeploymentGroup) WaitsForPromotion() bool {
	return g.DesiredCanaries > 0 && !g.Promoted
}

// Deployment is one deployment with its groups, in the order of their names.
func (c *Client) Deployment(ctx context.Context, namespace, deploymentID string) (DeploymentDetail, error) {
	d, _, err := c.api.Deployments().Info(deploymentID, c.query(ctx, namespace))
	if err != nil {
		return DeploymentDetail{}, err
	}

	detail := DeploymentDetail{Deployment: Deployment{
		ID:                d.ID,
		JobID:             d.JobID,
		Namespace:         d.Namespace,
		JobVersion:        d.JobVersion,
		Status:            d.Status,
		StatusDescription: d.StatusDescription,
	}}

	for name, state := range d.TaskGroups {
		if state == nil {
			continue
		}

		detail.Groups = append(detail.Groups, DeploymentGroup{
			Name:              name,
			DesiredTotal:      state.DesiredTotal,
			Placed:            state.PlacedAllocs,
			Healthy:           state.HealthyAllocs,
			Unhealthy:         state.UnhealthyAllocs,
			DesiredCanaries:   state.DesiredCanaries,
			PlacedCanaries:    len(state.PlacedCanaries),
			Promoted:          state.Promoted,
			AutoRevert:        state.AutoRevert,
			ProgressDeadline:  state.ProgressDeadline,
			RequireProgressBy: state.RequireProgressBy,
		})
	}

	sort.Slice(detail.Groups, func(i, j int) bool { return detail.Groups[i].Name < detail.Groups[j].Name })

	return detail, nil
}

// DeploymentAllocations are the allocations a deployment placed, with how
// it judged each of them.
func (c *Client) DeploymentAllocations(ctx context.Context, namespace, deploymentID string) ([]Alloc, error) {
	stubs, _, err := c.api.Deployments().Allocations(deploymentID, c.query(ctx, namespace))
	if err != nil {
		return nil, err
	}

	allocs := make([]Alloc, 0, len(stubs))
	for _, stub := range stubs {
		alloc := newAlloc(stub)

		// A stub does not say which deployment it is in; these are all in
		// this one.
		alloc.Health = healthOf(deploymentID, stub.DeploymentStatus)
		alloc.Canary = stub.DeploymentStatus != nil && stub.DeploymentStatus.Canary

		allocs = append(allocs, alloc)
	}

	return allocs, nil
}
