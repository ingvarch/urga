package nomad

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/hashicorp/nomad/api"
	"github.com/zclconf/go-cty/cty"
)

// SubmitJob sends a job file to the cluster. HCL and JSON are both taken,
// which is what the editor hands back. The file is kept with the version it
// makes, and so are the values of its variables: the next edit opens the same
// file, and the job runs with the values it ran with before. It is submitted
// at the index of its plan: a job that changed since is refused rather than
// overwritten. The counts it opened with decide which counts the job keeps,
// see keepCounts.
func (c *Client) SubmitJob(ctx context.Context, namespace, source string, vars JobVariables, index uint64, opened map[string]int) error {
	job, kept, _, err := c.jobOf(ctx, namespace, source, vars, opened)
	if err != nil {
		return err
	}

	opts := &api.RegisterOptions{Submission: kept, EnforceIndex: true, ModifyIndex: index}

	_, _, err = c.api.Jobs().RegisterOpts(job, opts, c.write(ctx, namespace))
	if jobChanged(err) {
		return ErrJobChanged
	}

	return err
}

// jobOf reads a job file and puts the job in the namespace it is sent to,
// with the counts it keeps from the job that runs.
func (c *Client) jobOf(ctx context.Context, namespace, source string, vars JobVariables, opened map[string]int) (*api.Job, *api.JobSubmission, []KeptCount, error) {
	job, submission, err := c.parseJob(ctx, namespace, source, vars, true)
	if err != nil {
		return nil, nil, nil, err
	}

	if namespace != "" && namespace != AllNamespaces {
		job.Namespace = &namespace
	}

	kept, err := c.keepCounts(ctx, namespace, job, opened)

	return job, submission, kept, err
}

// KeptCount is a group that keeps the count its job runs with, though the
// file says another.
type KeptCount struct {
	Group string
	Kept  int
	File  int
}

// keepCounts sets the count of each group of an edited job. A group whose
// count is as in the file when it was opened keeps the count the job runs it
// with, which the autoscaler or a person may have changed since. A group
// whose count was edited, a new group, and a job that is not running take
// the count of the file. Nil opened counts keep nothing.
func (c *Client) keepCounts(ctx context.Context, namespace string, job *api.Job, opened map[string]int) ([]KeptCount, error) {
	if opened == nil {
		return nil, nil
	}

	running, _, err := c.api.Jobs().Info(valueOf(job.ID), c.query(ctx, namespace))
	if answered(err, http.StatusNotFound) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	var kept []KeptCount

	for _, group := range job.TaskGroups {
		name := valueOf(group.Name)
		file := countOf(group)

		was, wasOpened := opened[name]
		now := slices.IndexFunc(running.TaskGroups, func(g *api.TaskGroup) bool { return valueOf(g.Name) == name })

		if !wasOpened || was != file || now < 0 {
			continue
		}

		if count := countOf(running.TaskGroups[now]); count != file {
			group.Count = &count
			kept = append(kept, KeptCount{Group: name, Kept: count, File: file})
		}
	}

	return kept, nil
}

// countOf is the count of a group, which is 1 when the file leaves it out.
func countOf(group *api.TaskGroup) int {
	if group.Count == nil {
		return 1
	}

	return *group.Count
}

// JobCounts reads the count of each group of a job file, the way a plan of
// it would.
func (c *Client) JobCounts(ctx context.Context, namespace, source string, vars JobVariables) (map[string]int, error) {
	job, _, _, err := c.jobOf(ctx, namespace, source, vars, nil)
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int, len(job.TaskGroups))
	for _, group := range job.TaskGroups {
		counts[valueOf(group.Name)] = countOf(group)
	}

	return counts, nil
}

// parseJob reads a job file the way the cluster does, and returns what of it
// to keep with the version. A canonical job has every field the file leaves
// out filled in, its namespace too.
func (c *Client) parseJob(ctx context.Context, namespace, source string, vars JobVariables, canonicalize bool) (*api.Job, *api.JobSubmission, error) {
	if strings.HasPrefix(strings.TrimSpace(source), "{") {
		// The file holds the job or wraps it in a Job key. The cluster takes
		// both, and keeps the file as it was written.
		var either struct {
			Wrapped *api.Job `json:"Job"`
			api.Job
		}

		if err := json.Unmarshal([]byte(source), &either); err != nil {
			return nil, nil, fmt.Errorf("the job is not valid JSON: %w", err)
		}

		job := &either.Job
		if either.Wrapped != nil {
			job = either.Wrapped
		}

		// Variables are a thing of HCL, a JSON job has none.
		return job, &api.JobSubmission{Source: source, Format: FormatJSON}, nil
	}

	variables, err := vars.file()
	if err != nil {
		return nil, nil, err
	}

	request := &api.JobsParseRequest{JobHCL: source, Variables: variables, Canonicalize: canonicalize}

	// The client of the API parses without a namespace, which is the default
	// one, and a token may be allowed to parse jobs in its own namespace only.
	job := &api.Job{}
	if _, err := c.api.Raw().Write("/v1/jobs/parse", request, job, c.write(ctx, namespace)); err != nil {
		return nil, nil, fmt.Errorf("the job was not accepted: %w", err)
	}

	return job, &api.JobSubmission{
		Source:        source,
		Format:        formatHCL2,
		VariableFlags: vars.Flags,
		Variables:     vars.File,
	}, nil
}

// file is the variables as one variables file, the only form the cluster
// parses them in. A flag replaces the variable of its name in the file, as
// -var does on the command line, or is added to it. Its value is a string:
// the type of the variable converts it, the way it does for a value typed
// after -var.
func (v JobVariables) file() (string, error) {
	if len(v.Flags) == 0 {
		return v.File, nil
	}

	if strings.HasPrefix(strings.TrimSpace(v.File), "{") {
		return v.jsonFile()
	}

	file, diags := hclwrite.ParseConfig([]byte(v.File), "variables.hcl", hcl.InitialPos)
	if diags.HasErrors() {
		return "", fmt.Errorf("the variables file of the job does not parse: %w", diags)
	}

	for _, name := range slices.Sorted(maps.Keys(v.Flags)) {
		file.Body().SetAttributeValue(name, cty.StringVal(v.Flags[name]))
	}

	return string(file.Bytes()), nil
}

// jsonFile sets the flags in a variables file written as JSON.
func (v JobVariables) jsonFile() (string, error) {
	var values map[string]json.RawMessage
	if err := json.Unmarshal([]byte(v.File), &values); err != nil {
		return "", fmt.Errorf("the variables file of the job does not parse: %w", err)
	}

	for name, value := range v.Flags {
		text, err := json.Marshal(value)
		if err != nil {
			return "", err
		}

		values[name] = text
	}

	out, err := json.Marshal(values)
	if err != nil {
		return "", err
	}

	return string(out), nil
}

// NamespaceSpec is a namespace as a file, which is how it is edited.
func (c *Client) NamespaceSpec(ctx context.Context, name string) (string, error) {
	namespace, _, err := c.api.Namespaces().Info(name, c.query(ctx, ""))
	if err != nil {
		return "", err
	}

	return asJSON(namespace)
}

// SubmitNamespace sends a namespace file back to the cluster.
func (c *Client) SubmitNamespace(ctx context.Context, source string) error {
	namespace, err := namespaceOf(source)
	if err != nil {
		return err
	}

	_, err = c.api.Namespaces().Register(namespace, c.write(ctx, ""))

	return err
}

// CreateNamespace registers a namespace from its file when there is none of
// its name, and says its name: registering one that exists would overwrite
// it.
func (c *Client) CreateNamespace(ctx context.Context, source string) (string, error) {
	namespace, err := namespaceOf(source)
	if err != nil {
		return "", err
	}

	if namespace.Name == "" {
		return "", errors.New("a namespace needs a name")
	}

	_, _, err = c.api.Namespaces().Info(namespace.Name, c.query(ctx, ""))

	switch {
	case err == nil:
		return "", fmt.Errorf("namespace %s already exists", namespace.Name)
	case !answered(err, http.StatusNotFound):
		return "", err
	}

	_, err = c.api.Namespaces().Register(namespace, c.write(ctx, ""))

	return namespace.Name, err
}

// DeleteNamespace deletes a namespace. The cluster refuses one that still
// holds jobs, variables or volumes, and the default one.
func (c *Client) DeleteNamespace(ctx context.Context, name string) error {
	_, err := c.api.Namespaces().Delete(name, c.write(ctx, ""))

	return err
}

// namespaceOf reads a namespace file.
func namespaceOf(source string) (*api.Namespace, error) {
	namespace := &api.Namespace{}
	if err := json.Unmarshal([]byte(source), namespace); err != nil {
		return nil, fmt.Errorf("the namespace is not valid JSON: %w", err)
	}

	return namespace, nil
}

// JobTemplate is the file a new job starts from.
const JobTemplate = `job "web" {
  type = "service"

  group "web" {
    count = 1

    network {
      port "http" {
        to = 80
      }
    }

    task "server" {
      driver = "docker"

      config {
        image = "nginx:1.27"
        ports = ["http"]
      }

      resources {
        cpu    = 100
        memory = 128
      }
    }
  }
}
`

// JobTarget is where a job file sends its job: the job, the namespace it goes
// to, and what the job is.
type JobTarget struct {
	ID        string
	Namespace string
	Type      string

	// Launches says the job launches jobs of its own.
	Launches bool
}

// JobTarget reads a job file for where it goes. The namespace is the one the
// file names, or else the one of the session, and default when the session
// looks at all of them.
func (c *Client) JobTarget(ctx context.Context, namespace, source string) (JobTarget, error) {
	if namespace == "" || namespace == AllNamespaces {
		namespace = DefaultNamespace
	}

	job, _, err := c.parseJob(ctx, namespace, source, JobVariables{}, false)
	if err != nil {
		return JobTarget{}, err
	}

	target := JobTarget{
		ID:        valueOf(job.ID),
		Namespace: namespace,
		Type:      valueOf(job.Type),
		Launches:  job.IsPeriodic() || job.IsParameterized(),
	}

	if named := valueOf(job.Namespace); named != "" {
		target.Namespace = named
	}

	if target.Type == "" {
		target.Type = api.JobTypeService
	}

	return target, nil
}
