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

// How a job file is written, as the cluster names it.
const (
	FormatJSON = "json"
	formatHCL2 = "hcl2"
)

// JobSource is what a job was submitted with: the file, how it is written,
// and the values its variables were given.
type JobSource struct {
	Source string

	// Format is hcl2 or FormatJSON.
	Format string

	Variables JobVariables
}

// JobVariables are the values of the variables of an HCL job: the -var flags
// and the variables file it was run with.
type JobVariables struct {
	Flags map[string]string
	File  string
}

// JobSpec is the file the job was submitted with. A cluster that did not keep
// it says so.
func (c *Client) JobSpec(ctx context.Context, namespace, jobID string) (JobSource, error) {
	// Which version runs is asked first: a submission is kept per version,
	// and version zero is the first file ever submitted, not the current
	// one. Guessing it here would put an old job in front of the editor.
	job, _, err := c.api.Jobs().Info(jobID, c.query(ctx, namespace))
	if err != nil {
		return JobSource{}, err
	}

	submission, err := c.submissionOf(ctx, namespace, jobID, versionOf(job))
	if err != nil {
		return JobSource{}, err
	}

	return JobSource{
		Source:    submission.Source,
		Format:    submission.Format,
		Variables: JobVariables{Flags: submission.VariableFlags, File: submission.Variables},
	}, nil
}

// versionOf is the version of the job that runs.
func versionOf(job *api.Job) int {
	if job.Version == nil {
		return 0
	}

	return int(*job.Version)
}

// submissionOf is the file a version of the job was submitted with.
func (c *Client) submissionOf(ctx context.Context, namespace, jobID string, version int) (*api.JobSubmission, error) {
	submission, _, err := c.api.Jobs().Submission(jobID, version, c.query(ctx, namespace))

	// The job was read a moment ago, so a submission that is not found is a
	// job that was registered without its file.
	var answer api.UnexpectedResponseError
	if errors.As(err, &answer) && answer.StatusCode() == http.StatusNotFound {
		return nil, ErrNoSource
	}

	if err != nil {
		return nil, err
	}

	if submission == nil || submission.Source == "" {
		return nil, ErrNoSource
	}

	return submission, nil
}

func asJSON(v any) (string, error) {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}

	return string(out), nil
}
