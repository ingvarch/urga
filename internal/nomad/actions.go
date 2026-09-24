package nomad

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/hashicorp/nomad/api"
)

// StopJob tells the cluster to stop a job. The job stays in the list, dead,
// until it is purged.
func (c *Client) StopJob(ctx context.Context, namespace, jobID string) error {
	_, _, err := c.api.Jobs().Deregister(jobID, false, c.write(ctx, namespace))

	return err
}

// StartJob submits a stopped job again. The new version keeps the file the
// job was submitted with, and gets back the scaling policies that stopping
// turned off.
func (c *Client) StartJob(ctx context.Context, namespace, jobID string) error {
	job, _, err := c.api.Jobs().Info(jobID, c.query(ctx, namespace))
	if err != nil {
		return err
	}

	job.Stop = boolPtr(false)

	// A job registered through the API has no file, and starts without one.
	kept, err := c.submissionOf(ctx, namespace, jobID, versionOf(job))
	if err != nil && !errors.Is(err, ErrNoSource) {
		return err
	}

	if kept != nil {
		if err := c.restoreScaling(ctx, namespace, job, kept); err != nil {
			return err
		}
	}

	_, _, err = c.api.Jobs().RegisterOpts(job, &api.RegisterOptions{Submission: kept}, c.write(ctx, namespace))

	return err
}

// restoreScaling turns the scaling policies of a stopped job back to what its
// file says they are. Stopping turns every one of them off, and the stopped
// job does not remember which were on.
func (c *Client) restoreScaling(ctx context.Context, namespace string, job *api.Job, kept *api.JobSubmission) error {
	// Without a policy there is nothing to restore, and no reason to parse a
	// file the cluster may no longer read.
	scaled := slices.ContainsFunc(job.TaskGroups, func(group *api.TaskGroup) bool { return group.Scaling != nil })
	if !scaled {
		return nil
	}

	submitted, _, err := c.parseJob(ctx, namespace, kept.Source, JobVariables{Flags: kept.VariableFlags, File: kept.Variables})
	if err != nil {
		return fmt.Errorf("the scaling policies could not be turned back on: %w", err)
	}

	enabled := map[string]*bool{}
	for _, group := range submitted.TaskGroups {
		if group.Name != nil && group.Scaling != nil {
			enabled[*group.Name] = group.Scaling.Enabled
		}
	}

	for _, group := range job.TaskGroups {
		if group.Name == nil || group.Scaling == nil {
			continue
		}

		if on, ok := enabled[*group.Name]; ok {
			group.Scaling.Enabled = on
		}
	}

	return nil
}

// RestartAllocation restarts every task of an allocation.
func (c *Client) RestartAllocation(ctx context.Context, namespace, allocID string) error {
	alloc, _, err := c.api.Allocations().Info(allocID, c.query(ctx, namespace))
	if err != nil {
		return err
	}

	return c.api.Allocations().Restart(alloc, "", c.query(ctx, namespace))
}

// RestartTask restarts one task of an allocation, in place; the rest keep
// running. The client refuses a task that is not running.
func (c *Client) RestartTask(ctx context.Context, namespace, allocID, task string) error {
	return c.api.Allocations().Restart(&api.Allocation{ID: allocID}, task, c.query(ctx, namespace))
}

// SignalTask sends a signal, named the way the client names it (SIGHUP), to
// one task of an allocation.
func (c *Client) SignalTask(ctx context.Context, namespace, allocID, task, signal string) error {
	return c.api.Allocations().Signal(&api.Allocation{ID: allocID}, c.query(ctx, namespace), task, signal)
}

// StopAllocation stops an allocation. The scheduler places a new one when the
// job still asks for it.
func (c *Client) StopAllocation(ctx context.Context, namespace, allocID string) error {
	alloc, _, err := c.api.Allocations().Info(allocID, c.query(ctx, namespace))
	if err != nil {
		return err
	}

	_, err = c.api.Allocations().Stop(alloc, c.query(ctx, namespace))

	return err
}

// ScaleJob sets how many allocations a task group runs.
func (c *Client) ScaleJob(ctx context.Context, namespace, jobID, group string, count int) error {
	_, _, err := c.api.Jobs().Scale(jobID, group, &count, "scaled from urga", false, nil, c.write(ctx, namespace))

	return err
}

func (c *Client) write(ctx context.Context, namespace string) *api.WriteOptions {
	return (&api.WriteOptions{Namespace: namespace, Region: c.region}).WithContext(ctx)
}

func boolPtr(v bool) *bool { return &v }

// DrainNode starts or stops moving the work off a node. Stopping a drain
// makes the node take work again.
func (c *Client) DrainNode(ctx context.Context, nodeID string, drain bool) error {
	opts := &api.DrainOptions{MarkEligible: true}

	if drain {
		opts = &api.DrainOptions{DrainSpec: &api.DrainSpec{Deadline: drainDeadline}}
	}

	_, err := c.api.Nodes().UpdateDrainOpts(nodeID, opts, c.write(ctx, ""))

	return err
}

// drainDeadline is how long the allocations of a draining node are given to
// stop on their own.
const drainDeadline = time.Hour

// SetNodeEligible says whether a node may be given new work.
func (c *Client) SetNodeEligible(ctx context.Context, nodeID string, eligible bool) error {
	_, err := c.api.Nodes().ToggleEligibility(nodeID, eligible, c.write(ctx, ""))

	return err
}

// PromoteDeployment takes the canaries of a deployment into service.
func (c *Client) PromoteDeployment(ctx context.Context, namespace, deploymentID string) error {
	_, _, err := c.api.Deployments().PromoteAll(deploymentID, c.write(ctx, namespace))

	return err
}

// PromoteGroups takes the canaries of some task groups of a deployment into
// service; the other groups keep theirs.
func (c *Client) PromoteGroups(ctx context.Context, namespace, deploymentID string, groups []string) error {
	_, _, err := c.api.Deployments().PromoteGroups(deploymentID, groups, c.write(ctx, namespace))

	return err
}

// PauseDeployment stops a deployment where it is, or lets it go on.
func (c *Client) PauseDeployment(ctx context.Context, namespace, deploymentID string, pause bool) error {
	_, _, err := c.api.Deployments().Pause(deploymentID, pause, c.write(ctx, namespace))

	return err
}

// FailDeployment stops a deployment and rolls it back where the job says to.
func (c *Client) FailDeployment(ctx context.Context, namespace, deploymentID string) error {
	_, _, err := c.api.Deployments().Fail(deploymentID, c.write(ctx, namespace))

	return err
}
