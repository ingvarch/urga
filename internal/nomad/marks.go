package nomad

import (
	"context"

	"github.com/hashicorp/nomad/api"
)

// TagVersion gives a version of a job a name, which can be reverted to by it.
func (c *Client) TagVersion(ctx context.Context, namespace, jobID string, version uint64, name string) error {
	_, err := c.api.Jobs().TagVersion(jobID, version, name, "", c.write(ctx, namespace))

	return err
}

// UntagVersion takes a name off the version of a job that carries it.
func (c *Client) UntagVersion(ctx context.Context, namespace, jobID, name string) error {
	_, err := c.api.Jobs().UntagVersion(jobID, name, c.write(ctx, namespace))

	return err
}

// SetAllocHealth marks allocations of a deployment healthy or unhealthy by
// hand, for a group that checks its health that way.
func (c *Client) SetAllocHealth(ctx context.Context, namespace, deploymentID string, allocIDs []string, healthy bool) error {
	var good, bad []string
	if healthy {
		good = allocIDs
	} else {
		bad = allocIDs
	}

	_, _, err := c.api.Deployments().SetAllocHealth(deploymentID, good, bad, c.write(ctx, namespace))

	return err
}

// ReleaseLock releases the lock held on a variable, whoever holds it.
func (c *Client) ReleaseLock(ctx context.Context, namespace, path, lockID string) error {
	_, _, err := c.api.Variables().ReleaseLock(&api.Variable{
		Namespace: namespace,
		Path:      path,
		Lock:      &api.VariableLock{ID: lockID},
	}, c.write(ctx, namespace))

	return err
}
