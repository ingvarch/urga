package nomad

import (
	"context"
	"fmt"

	"github.com/hashicorp/nomad/api"
)

// StopJob tells the cluster to stop a job. The job stays in the list, dead,
// until it is purged.
func (c *Client) StopJob(ctx context.Context, namespace, jobID string) error {
	_, _, err := c.api.Jobs().Deregister(jobID, false, c.write(ctx, namespace))

	return err
}

// StartJob submits a stopped job again.
func (c *Client) StartJob(ctx context.Context, namespace, jobID string) error {
	job, _, err := c.api.Jobs().Info(jobID, c.query(ctx, namespace))
	if err != nil {
		return err
	}

	job.Stop = boolPtr(false)

	_, _, err = c.api.Jobs().Register(job, c.write(ctx, namespace))

	return err
}

// RestartAllocation restarts every task of an allocation.
func (c *Client) RestartAllocation(ctx context.Context, namespace, allocID string) error {
	alloc, _, err := c.api.Allocations().Info(allocID, c.query(ctx, namespace))
	if err != nil {
		return err
	}

	return c.api.Allocations().Restart(alloc, "", c.query(ctx, namespace))
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

// RevertJob puts the version before the one that runs back in place.
func (c *Client) RevertJob(ctx context.Context, namespace, jobID string) error {
	job, _, err := c.api.Jobs().Info(jobID, c.query(ctx, namespace))
	if err != nil {
		return err
	}

	if job.Version == nil || *job.Version == 0 {
		return fmt.Errorf("%s has no earlier version to go back to", jobID)
	}

	_, _, err = c.api.Jobs().Revert(jobID, *job.Version-1, nil, c.write(ctx, namespace), "", "")

	return err
}

// ScaleJob sets how many allocations a task group runs.
func (c *Client) ScaleJob(ctx context.Context, namespace, jobID, group string, count int) error {
	_, _, err := c.api.Jobs().Scale(jobID, group, &count, "scaled from urga", false, nil, c.write(ctx, namespace))

	return err
}

func (c *Client) write(ctx context.Context, namespace string) *api.WriteOptions {
	return (&api.WriteOptions{Namespace: namespace}).WithContext(ctx)
}

func boolPtr(v bool) *bool { return &v }
