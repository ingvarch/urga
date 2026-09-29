package nomad

import (
	"context"
	"time"

	"github.com/hashicorp/nomad/api"
)

// ScalingPolicy is one policy of the list: what it scales, of what type, and
// whether it is on.
type ScalingPolicy struct {
	ID, Namespace, Job, Group, Type string

	Enabled bool
}

// GroupPolicy is the policy that scales a task group: whether it is on, and
// the bounds it keeps the count in.
type GroupPolicy struct {
	ID       string
	Enabled  bool
	Min, Max int
}

// GroupScaling is how a task group is scaled: its count, the policy that
// scales it, if any, and what was done to its count, newest first.
type GroupScaling struct {
	Count, Running, Healthy int

	Policy *GroupPolicy
	Events []ScalingEvent
}

// ScalingEvent is one change of a count, or a report that left it: To is nil
// then.
type ScalingEvent struct {
	At      time.Time
	From    int
	To      *int
	Message string
	Error   bool
}

// ScalingPolicies lists the scaling policies of a namespace.
func (c *Client) ScalingPolicies(ctx context.Context, namespace string) ([]ScalingPolicy, error) {
	stubs, _, err := c.api.Scaling().ListPolicies(c.query(ctx, namespace))

	return listed(stubs, err, func(p *api.ScalingPolicyListStub) ScalingPolicy {
		return ScalingPolicy{
			ID: p.ID, Namespace: p.Target["Namespace"], Job: p.Target["Job"], Group: p.Target["Group"],
			Type: p.Type, Enabled: p.Enabled,
		}
	})
}

// DescribeScalingPolicy is a policy in full, as JSON: its bounds and the
// settings the autoscaler reads.
func (c *Client) DescribeScalingPolicy(ctx context.Context, namespace, id string) (string, error) {
	policy, _, err := c.api.Scaling().GetPolicy(id, c.query(ctx, namespace))
	if err != nil {
		return "", err
	}

	return asJSON(policy)
}

// GroupScaling reads how a task group of a job is scaled. The policy is part
// of the job, the counts and the events are in its scale status.
func (c *Client) GroupScaling(ctx context.Context, namespace, jobID, group string) (GroupScaling, error) {
	job, _, err := c.api.Jobs().Info(jobID, c.query(ctx, namespace))
	if err != nil {
		return GroupScaling{}, err
	}

	status, _, err := c.api.Jobs().ScaleStatus(jobID, c.query(ctx, namespace))
	if err != nil {
		return GroupScaling{}, err
	}

	var scaling GroupScaling

	for _, tg := range job.TaskGroups {
		if tg != nil && tg.Name != nil && *tg.Name == group {
			scaling.Policy = groupPolicy(tg.Scaling)
		}
	}

	counts := status.TaskGroups[group]
	scaling.Count, scaling.Running, scaling.Healthy = counts.Desired, counts.Running, counts.Healthy

	for _, e := range counts.Events {
		event := ScalingEvent{At: time.Unix(0, int64(e.Time)), From: int(e.PreviousCount), Message: e.Message, Error: e.Error}
		if e.Count != nil {
			to := int(*e.Count)
			event.To = &to
		}

		scaling.Events = append(scaling.Events, event)
	}

	return scaling, nil
}

// groupPolicy is the policy of a group as the job holds it, nil for none.
func groupPolicy(p *api.ScalingPolicy) *GroupPolicy {
	if p == nil {
		return nil
	}

	return &GroupPolicy{ID: p.ID, Enabled: valueOf(p.Enabled), Min: int(valueOf(p.Min)), Max: int(valueOf(p.Max))}
}
