package nomad

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/nomad/api"
)

// ErrNoDiff is a version with nothing before it to compare against.
var ErrNoDiff = errors.New("no version before this one")

// JobVersion is one version of a job as the cluster kept it.
type JobVersion struct {
	Version uint64

	// Current says this is the version that runs, Stable that the cluster
	// was happy with it.
	Current bool
	Stable  bool

	// Tag is the name a version was given, when it was given one.
	Tag string

	Submitted time.Time

	// Changes is how many fields differ from the version before this one.
	Changes int
}

// JobVersions lists what a job was, newest first, with how much each version
// changed from the one before it.
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
		if i < len(diffs) {
			version.Changes = countChanges(diffs[i])
		}

		out = append(out, version)
	}

	return out, nil
}

// JobVersionDiff is what one version changed, as text.
func (c *Client) JobVersionDiff(ctx context.Context, namespace, jobID string, version uint64) (string, error) {
	jobs, diffs, err := c.versions(ctx, namespace, jobID)
	if err != nil {
		return "", err
	}

	for i, job := range jobs {
		if job == nil || uintOf(job.Version) != version {
			continue
		}

		if i >= len(diffs) || diffs[i] == nil {
			return "", ErrNoDiff
		}

		return writeJobDiff(diffs[i]), nil
	}

	return "", ErrNoDiff
}

// RevertJobTo puts the version it is given back in place.
func (c *Client) RevertJobTo(ctx context.Context, namespace, jobID string, version uint64) error {
	_, _, err := c.api.Jobs().Revert(jobID, version, nil, c.write(ctx, namespace), "", "")

	return err
}

func (c *Client) versions(ctx context.Context, namespace, jobID string) ([]*api.Job, []*api.JobDiff, error) {
	jobs, diffs, _, err := c.api.Jobs().Versions(jobID, true, c.query(ctx, namespace))

	return jobs, diffs, err
}

// countChanges is how many fields a diff touches, however deep they sit.
func countChanges(diff *api.JobDiff) int {
	if diff == nil {
		return 0
	}

	count := len(diff.Fields)

	for _, object := range diff.Objects {
		count += countObject(object)
	}

	for _, group := range diff.TaskGroups {
		if group == nil {
			continue
		}

		count += len(group.Fields)

		for _, object := range group.Objects {
			count += countObject(object)
		}

		for _, task := range group.Tasks {
			if task == nil {
				continue
			}

			count += len(task.Fields)

			for _, object := range task.Objects {
				count += countObject(object)
			}
		}
	}

	return count
}

func countObject(object *api.ObjectDiff) int {
	if object == nil {
		return 0
	}

	count := len(object.Fields)
	for _, inner := range object.Objects {
		count += countObject(inner)
	}

	return count
}

// writeJobDiff puts a diff in the shape it reads in: the job, then its task
// groups, then the tasks under them.
func writeJobDiff(diff *api.JobDiff) string {
	lines := []string{}

	lines = append(lines, fieldLines(diff.Fields, 0)...)
	lines = append(lines, objectLines(diff.Objects, 0)...)

	for _, group := range diff.TaskGroups {
		if group == nil {
			continue
		}

		lines = append(lines, "", fmt.Sprintf("Task Group: %s (%s)", group.Name, strings.ToLower(group.Type)))
		lines = append(lines, fieldLines(group.Fields, 1)...)
		lines = append(lines, objectLines(group.Objects, 1)...)

		for _, task := range group.Tasks {
			if task == nil {
				continue
			}

			lines = append(lines, "", fmt.Sprintf("  Task: %s (%s)", task.Name, strings.ToLower(task.Type)))
			lines = append(lines, fieldLines(task.Fields, 2)...)
			lines = append(lines, objectLines(task.Objects, 2)...)
		}
	}

	return strings.Join(lines, "\n")
}

func objectLines(objects []*api.ObjectDiff, depth int) []string {
	lines := []string{}

	for _, object := range objects {
		if object == nil {
			continue
		}

		lines = append(lines, indentLine(fmt.Sprintf("%s:", object.Name), depth))
		lines = append(lines, fieldLines(object.Fields, depth+1)...)
		lines = append(lines, objectLines(object.Objects, depth+1)...)
	}

	return lines
}

// fieldLines read the way a diff reads: what was added, what went away, and
// what changed from one value to another.
func fieldLines(fields []*api.FieldDiff, depth int) []string {
	lines := make([]string, 0, len(fields))

	for _, field := range fields {
		if field == nil {
			continue
		}

		switch field.Type {
		case "Added":
			lines = append(lines, indentLine(fmt.Sprintf("+ %s: %s", field.Name, field.New), depth))
		case "Deleted":
			lines = append(lines, indentLine(fmt.Sprintf("- %s: %s", field.Name, field.Old), depth))
		default:
			lines = append(lines, indentLine(fmt.Sprintf("~ %s: %s -> %s", field.Name, field.Old, field.New), depth))
		}
	}

	return lines
}

func indentLine(line string, depth int) string {
	return strings.Repeat("  ", depth) + line
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
