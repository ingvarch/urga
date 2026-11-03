package nomad

import (
	"cmp"
	"context"
	"fmt"

	"github.com/hashicorp/nomad/api"
)

// How a parameterized job takes a payload.
const (
	PayloadOptional  = "optional"
	PayloadRequired  = "required"
	PayloadForbidden = "forbidden"
)

// DispatchForm is what a dispatch of a parameterized job may give it: the
// meta keys it needs and those it takes, and whether it takes a payload.
type DispatchForm struct {
	Required, Optional []string
	Payload            string

	// Priority is the one of the job, which a dispatch keeps unless it
	// gives one.
	Priority int
}

// DispatchForm reads what a dispatch of a parameterized job may give it.
func (c *Client) DispatchForm(ctx context.Context, namespace, jobID string) (DispatchForm, error) {
	job, _, err := c.api.Jobs().Info(jobID, c.query(ctx, namespace))
	if err != nil {
		return DispatchForm{}, err
	}

	config := job.ParameterizedJob
	if config == nil {
		return DispatchForm{}, fmt.Errorf("%s is not a parameterized job", jobID)
	}

	// The cluster leaves the default out.
	return DispatchForm{
		Required: config.MetaRequired,
		Optional: config.MetaOptional,
		Payload:  cmp.Or(config.Payload, PayloadOptional),
		Priority: valueOf(job.Priority),
	}, nil
}

// DispatchJob dispatches a parameterized job, and says the ID of the job the
// dispatch launched. A priority of 0 keeps the one of the job.
func (c *Client) DispatchJob(ctx context.Context, namespace, jobID string, meta map[string]string, payload []byte, priority int) (string, error) {
	answer, _, err := c.api.Jobs().DispatchOpts(
		&api.DispatchOptions{JobID: jobID, Meta: meta, Payload: payload, Priority: priority}, c.write(ctx, namespace))
	if err != nil {
		return "", err
	}

	return answer.DispatchedJobID, nil
}
