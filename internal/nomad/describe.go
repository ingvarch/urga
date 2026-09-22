package nomad

import (
	"context"
	"encoding/json"
	"fmt"
)

// DescribeJob is what the cluster knows about a job.
func (c *Client) DescribeJob(ctx context.Context, namespace, jobID string) (string, error) {
	job, _, err := c.api.Jobs().Info(jobID, c.query(ctx, namespace))
	if err != nil {
		return "", err
	}

	return asJSON(job)
}

// DescribeAllocation is what the cluster knows about an allocation.
func (c *Client) DescribeAllocation(ctx context.Context, namespace, allocID string) (string, error) {
	alloc, _, err := c.api.Allocations().Info(allocID, c.query(ctx, namespace))
	if err != nil {
		return "", err
	}

	return asJSON(alloc)
}

// DescribeDeployment is what the cluster knows about a deployment.
func (c *Client) DescribeDeployment(ctx context.Context, namespace, deploymentID string) (string, error) {
	deployment, _, err := c.api.Deployments().Info(deploymentID, c.query(ctx, namespace))
	if err != nil {
		return "", err
	}

	return asJSON(deployment)
}

// DescribeService is every registration of a service name.
func (c *Client) DescribeService(ctx context.Context, namespace, name string) (string, error) {
	registrations, _, err := c.api.Services().Get(name, c.query(ctx, namespace))
	if err != nil {
		return "", err
	}

	return asJSON(registrations)
}

// JobSpec is the file the job was submitted with. A cluster that did not keep
// it says so.
func (c *Client) JobSpec(ctx context.Context, namespace, jobID string) (string, error) {
	version := 0

	if job, _, err := c.api.Jobs().Info(jobID, c.query(ctx, namespace)); err == nil && job.Version != nil {
		version = int(*job.Version)
	}

	submission, _, err := c.api.Jobs().Submission(jobID, version, c.query(ctx, namespace))
	if err != nil {
		return "", err
	}

	if submission == nil || submission.Source == "" {
		return fmt.Sprintf("The cluster kept no source for %s.\n\nIt was submitted before submissions were stored, or through the API without one.", jobID), nil
	}

	return submission.Source, nil
}

func asJSON(v any) (string, error) {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}

	return string(out), nil
}
