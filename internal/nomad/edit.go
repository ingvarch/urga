package nomad

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/hashicorp/nomad/api"
)

// SubmitJob sends a job file to the cluster. HCL and JSON are both taken,
// which is what the editor hands back. The file is kept with the version it
// makes, and so are the values of its variables: the next edit opens the same
// file, and the job runs with the values it ran with before. It is submitted
// at the index of its plan: a job that changed since is refused rather than
// overwritten.
func (c *Client) SubmitJob(ctx context.Context, namespace, source string, vars JobVariables, index uint64) error {
	job, kept, err := c.jobOf(ctx, namespace, source, vars)
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

// jobOf reads a job file and puts the job in the namespace it is sent to.
func (c *Client) jobOf(ctx context.Context, namespace, source string, vars JobVariables) (*api.Job, *api.JobSubmission, error) {
	job, kept, err := c.parseJob(ctx, namespace, source, vars)
	if err != nil {
		return nil, nil, err
	}

	if namespace != "" && namespace != AllNamespaces {
		job.Namespace = &namespace
	}

	return job, kept, nil
}

// parseJob reads a job file the way the cluster does, and returns what of it
// to keep with the version.
func (c *Client) parseJob(ctx context.Context, namespace, source string, vars JobVariables) (*api.Job, *api.JobSubmission, error) {
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

	request := &api.JobsParseRequest{JobHCL: source, Variables: vars.file(), Canonicalize: true}

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
// parses them in. A flag becomes a line of its own, its value a string: the
// type of the variable converts it, the way it does for a value typed after
// -var.
func (v JobVariables) file() string {
	var b strings.Builder

	for _, name := range slices.Sorted(maps.Keys(v.Flags)) {
		fmt.Fprintf(&b, "%s = %s\n", name, hclString(v.Flags[name]))
	}

	b.WriteString(v.File)

	return b.String()
}

// hclString quotes text as an HCL string. Beyond the escapes, ${ and %{ are
// doubled: in HCL they start a template, and a flag holds plain text.
func hclString(text string) string {
	quoted := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
		"\t", `\t`,
		"${", "$${",
		"%{", "%%{",
	).Replace(text)

	return `"` + quoted + `"`
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
	namespace := &api.Namespace{}
	if err := json.Unmarshal([]byte(source), namespace); err != nil {
		return fmt.Errorf("the namespace is not valid JSON: %w", err)
	}

	_, err := c.api.Namespaces().Register(namespace, c.write(ctx, ""))

	return err
}
