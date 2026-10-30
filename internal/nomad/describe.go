package nomad

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/hashicorp/nomad/api"
)

// ErrNoSource means the cluster does not have the file a job was submitted
// with: it was registered before submissions were kept, or through the API
// without one.
var ErrNoSource = errors.New("the cluster kept no source for this job")

// DescribeJob is what the cluster knows about a job.
func (c *Client) DescribeJob(ctx context.Context, namespace, jobID string) (string, error) {
	return c.describeRaw(ctx, namespace, "/v1/job/"+url.PathEscape(jobID))
}

// JobMeta is the meta of the job block, which is nil when the job has none.
func (c *Client) JobMeta(ctx context.Context, namespace, jobID string) (map[string]string, error) {
	job, _, err := c.api.Jobs().Info(jobID, c.query(ctx, namespace))
	if err != nil {
		return nil, err
	}

	return job.Meta, nil
}

// DescribeAllocation is what the cluster knows about an allocation.
func (c *Client) DescribeAllocation(ctx context.Context, namespace, allocID string) (string, error) {
	return c.describeRaw(ctx, namespace, "/v1/allocation/"+url.PathEscape(allocID))
}

// DescribeDeployment is what the cluster knows about a deployment.
func (c *Client) DescribeDeployment(ctx context.Context, namespace, deploymentID string) (string, error) {
	return c.describeRaw(ctx, namespace, "/v1/deployment/"+url.PathEscape(deploymentID))
}

// DescribeService is every registration of a service name.
func (c *Client) DescribeService(ctx context.Context, namespace, name string) (string, error) {
	return c.describeRaw(ctx, namespace, "/v1/service/"+url.PathEscape(name))
}

// describeRaw reads the exact body the cluster answered, and indents it for
// reading so that the keys and their order are the ones the cluster sent.
func (c *Client) describeRaw(ctx context.Context, namespace, endpoint string) (string, error) {
	var body json.RawMessage
	if _, err := c.api.Raw().Query(endpoint, &body, c.query(ctx, namespace)); err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := json.Indent(&buf, body, "", "  "); err != nil {
		return "", err
	}

	return buf.String(), nil
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

	// Index is the JobModifyIndex of the job as read, which check-and-set
	// compares.
	Index uint64
}

// JobVariables are the values of the variables of an HCL job: the -var flags
// and the variables file it was run with.
type JobVariables struct {
	Flags map[string]string
	File  string
}

// JobSpec is the file the job was submitted with, or ErrNoSource when the
// cluster did not keep it, with the job as JSON.
func (c *Client) JobSpec(ctx context.Context, namespace, jobID string) (JobSource, error) {
	// The running version is requested first: a submission is kept per
	// version, and version zero is the first file ever submitted, not the
	// current one. Guessing it here would open an old job in the editor.
	job, _, err := c.api.Jobs().Info(jobID, c.query(ctx, namespace))
	if err != nil {
		return JobSource{}, err
	}

	read := valueOf(job.JobModifyIndex)

	// Without a file the job is edited as JSON, from the same read as its
	// index.
	submission, err := c.submissionOf(ctx, namespace, jobID, versionOf(job))
	if errors.Is(err, ErrNoSource) {
		spec, jsonErr := asJSON(job)
		if jsonErr != nil {
			return JobSource{}, jsonErr
		}

		return JobSource{Source: spec, Format: FormatJSON, Index: read}, err
	}

	if err != nil {
		return JobSource{}, err
	}

	return JobSource{
		Source:    submission.Source,
		Format:    submission.Format,
		Variables: JobVariables{Flags: submission.VariableFlags, File: submission.Variables},
		Index:     read,
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

	// The job was read a moment ago, so a submission that is not found means
	// the job was registered without its file.
	if answered(err, http.StatusNotFound) {
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
