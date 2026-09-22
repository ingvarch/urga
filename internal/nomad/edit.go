package nomad

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/nomad/api"
)

// SubmitJob sends a job file to the cluster. HCL and JSON are both taken,
// which is what the editor hands back.
func (c *Client) SubmitJob(ctx context.Context, namespace, source string) error {
	job, err := c.parseJob(source)
	if err != nil {
		return err
	}

	if namespace != "" && namespace != AllNamespaces {
		job.Namespace = &namespace
	}

	_, _, err = c.api.Jobs().Register(job, c.write(ctx, namespace))

	return err
}

// parseJob reads a job file the way the cluster does.
func (c *Client) parseJob(source string) (*api.Job, error) {
	if strings.HasPrefix(strings.TrimSpace(source), "{") {
		job := &api.Job{}
		if err := json.Unmarshal([]byte(source), job); err != nil {
			return nil, fmt.Errorf("the job is not valid JSON: %w", err)
		}

		return job, nil
	}

	job, err := c.api.Jobs().ParseHCL(source, true)
	if err != nil {
		return nil, fmt.Errorf("the job was not accepted: %w", err)
	}

	return job, nil
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
