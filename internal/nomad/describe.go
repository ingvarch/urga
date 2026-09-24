package nomad

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/hashicorp/nomad/api"
)

// ErrNoSource says the cluster does not have the file a job was submitted
// with: it was registered before submissions were kept, or through the API
// without one.
var ErrNoSource = errors.New("the cluster kept no source for this job")

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
	// Which version runs is asked first: a submission is kept per version,
	// and version zero is the first file ever submitted, not the current
	// one. Guessing it here would put an old job in front of the editor.
	job, _, err := c.api.Jobs().Info(jobID, c.query(ctx, namespace))
	if err != nil {
		return "", err
	}

	version := 0
	if job.Version != nil {
		version = int(*job.Version)
	}

	submission, _, err := c.api.Jobs().Submission(jobID, version, c.query(ctx, namespace))

	// The job was read a moment ago, so a submission that is not found is a
	// job that was registered without its file.
	var answer api.UnexpectedResponseError
	if errors.As(err, &answer) && answer.StatusCode() == http.StatusNotFound {
		return "", ErrNoSource
	}

	if err != nil {
		return "", err
	}

	if submission == nil || submission.Source == "" {
		return "", ErrNoSource
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
