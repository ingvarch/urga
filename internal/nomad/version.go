package nomad

import (
	"context"
	"errors"
	"time"

	"github.com/hashicorp/nomad/api"
)

var (
	// ErrNoDiff is a version with nothing before it to compare against.
	ErrNoDiff = errors.New("no version before this one")

	// ErrNoVersion is a version the cluster does not have.
	ErrNoVersion = errors.New("no such version")
)

// JobVersion is one version of a job as the cluster kept it.
type JobVersion struct {
	Version uint64

	// Current says this is the version that runs, Stable that the cluster
	// marked it stable, as it does after a successful deployment.
	Current bool
	Stable  bool

	// Tag is the name a version was given, when it was given one.
	Tag string

	Submitted time.Time

	// Changes is how many fields differ from the version before this one.
	Changes int
}

// JobVersions lists the versions of a job, newest first, with how much each
// one changed from the one before it.
func (c *Client) JobVersions(ctx context.Context, namespace, jobID string) ([]JobVersion, error) {
	jobs, diffs, err := c.versions(ctx, namespace, jobID)
	if err != nil {
		return nil, err
	}

	out := make([]JobVersion, 0, len(jobs))

	for i, job := range jobs {
		if job == nil {
			continue
		}

		version := JobVersion{
			Version: uintOf(job.Version),
			Stable:  boolOf(job.Stable),
			Current: i == 0,
		}

		if job.VersionTag != nil {
			version.Tag = job.VersionTag.Name
		}

		if job.SubmitTime != nil {
			version.Submitted = unixTime(*job.SubmitTime)
		}

		// The cluster answers with one diff per step between versions, so
		// the oldest one has none.
		if i < len(diffs) && diffs[i] != nil {
			version.Changes = countChanges(diffs[i])
		}

		out = append(out, version)
	}

	return out, nil
}

// JobVersionDiff is what one version changed, laid out as the job file.
func (c *Client) JobVersionDiff(ctx context.Context, namespace, jobID string, version uint64) ([]DiffLine, error) {
	jobs, diffs, err := c.versions(ctx, namespace, jobID)
	if err != nil {
		return nil, err
	}

	for i, job := range jobs {
		if job == nil || uintOf(job.Version) != version {
			continue
		}

		if i >= len(diffs) || diffs[i] == nil {
			return nil, ErrNoDiff
		}

		return hclDiff(diffs[i]), nil
	}

	return nil, ErrNoVersion
}

// RevertJobTo puts the version it is given back in place, if the job is
// still at the version from which the revert was planned.
func (c *Client) RevertJobTo(ctx context.Context, namespace, jobID string, version, from uint64) error {
	_, _, err := c.api.Jobs().Revert(jobID, version, &from, c.write(ctx, namespace), "", "")
	if jobChanged(err) {
		return ErrJobChanged
	}

	return err
}

func (c *Client) versions(ctx context.Context, namespace, jobID string) ([]*api.Job, []*api.JobDiff, error) {
	jobs, diffs, _, err := c.api.Jobs().Versions(jobID, true, c.query(ctx, namespace))

	return jobs, diffs, err
}

// countChanges is how many fields a diff changes, counted on the diff tree:
// in the text, a value of many lines takes as many lines, and those lines
// can look like changes of their own.
func countChanges(diff *api.JobDiff) int {
	count := countFields(diff.Fields, diff.Objects)

	for _, group := range diff.TaskGroups {
		if group == nil {
			continue
		}

		count += countFields(group.Fields, group.Objects)

		for _, task := range group.Tasks {
			if task == nil {
				continue
			}

			count += countFields(task.Fields, task.Objects)
		}
	}

	return count
}

func countFields(fields []*api.FieldDiff, objects []*api.ObjectDiff) int {
	count := 0

	for _, field := range fields {
		if field != nil && changed(field.Type) {
			count++
		}
	}

	for _, object := range objects {
		if object != nil {
			count += countFields(object.Fields, object.Objects)
		}
	}

	return count
}

func uintOf(value *uint64) uint64 {
	if value == nil {
		return 0
	}

	return *value
}

func boolOf(value *bool) bool {
	return value != nil && *value
}
