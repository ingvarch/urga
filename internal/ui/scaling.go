package ui

import (
	"context"
	"fmt"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// scalingPoliciesMsg is the scaling policies of a namespace.
type scalingPoliciesMsg []nomad.ScalingPolicy

// scalingPoliciesPage is the scaling policies of the namespace the session
// looks at: what the autoscaler may do with the groups of its jobs.
type scalingPoliciesPage struct {
	ofTheSession

	policies []nomad.ScalingPolicy
}

var scalingPolicyTitles = []string{"Job", "Group", "Namespace", "Type", "Enabled"}

func (scalingPoliciesPage) title(e env, count int) string {
	return sprintf("Scaling Policies (%s) [%d]", namespaceLabel(e.namespace), count)
}

func (scalingPoliciesPage) titles() []string { return scalingPolicyTitles }

// topics: a policy is part of its job, and changes with it.
func (scalingPoliciesPage) topics() []string { return []string{nomad.TopicJob} }

func (scalingPoliciesPage) fetch(e env) tea.Cmd {
	client, namespace := e.client, e.namespace

	return fetchList(func(ctx context.Context) ([]nomad.ScalingPolicy, error) {
		return client.ScalingPolicies(ctx, namespace)
	}, func(items []nomad.ScalingPolicy) tea.Msg { return scalingPoliciesMsg(items) })
}

func (p scalingPoliciesPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	policies, ok := msg.(scalingPoliciesMsg)
	if !ok {
		return p, outcome{}, false
	}

	p.policies = policies

	return p, outcome{}, true
}

// rows are the policies; one that is off is muted.
func (p scalingPoliciesPage) rows(env) []tableRow {
	rows := make([]tableRow, 0, len(p.policies))

	for _, policy := range p.policies {
		row := tableRow{cells: []string{policy.Job, policy.Group, policy.Namespace, policy.Type, yesNo(policy.Enabled)}}
		if !policy.Enabled {
			row.color = colorMuted
		}

		rows = append(rows, row)
	}

	return rows
}

var scalingPoliciesKeys = []pageKey[scalingPoliciesPage]{
	{press: "enter", label: "History", do: openScalingHistory},
	{press: "d", label: "Describe", do: describeScalingPolicy},
}

func (p scalingPoliciesPage) keys(e env) []keyHint { return hintsOf(p, e, scalingPoliciesKeys) }

func (p scalingPoliciesPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, scalingPoliciesKeys, k)
}

// openScalingHistory opens what was done to the group of the policy under
// the cursor, in the namespace of the policy.
func openScalingHistory(p scalingPoliciesPage, e env) (scalingPoliciesPage, outcome) {
	policy, ok := pickedFrom(e, p.policies)
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg{scalingPage{namespace: policy.Namespace, jobID: policy.Job, group: policy.Group}})
}

func describeScalingPolicy(p scalingPoliciesPage, e env) (scalingPoliciesPage, outcome) {
	policy, ok := pickedFrom(e, p.policies)
	if !ok {
		return p, outcome{}
	}

	return p, outcome{cmd: describePolicy(e.client, policy.Namespace, policy.Job, policy.Group, policy.ID)}
}

// describePolicy shows a policy in full: its bounds and the settings the
// autoscaler reads.
func describePolicy(client scalingClient, namespace, jobID, group, id string) tea.Cmd {
	return describe("Scaling policy: "+jobID+"/"+group, func(ctx context.Context) (string, error) {
		return client.DescribeScalingPolicy(ctx, namespace, id)
	})
}

// scalingMsg is how a task group is scaled.
type scalingMsg nomad.GroupScaling

// scalingPage is what was done to the count of one task group, and the
// policy that keeps it, if any.
type scalingPage struct {
	namespace, jobID, group string

	// scaling is what the cluster answered, once read.
	scaling nomad.GroupScaling
	read    bool
}

var scalingTitles = []string{"Change", "Message", "Age"}

// title names the count of the group and the bounds of its policy, once the
// cluster answered.
func (p scalingPage) title(_ env, count int) string {
	if !p.read {
		return sprintf("Scaling (Job: %s, Group: %s) [%d]", p.jobID, p.group, count)
	}

	return sprintf("Scaling (Job: %s, Group: %s, %s) [%d]", p.jobID, p.group, p.countIn(), count)
}

// countIn is the count of the group, in the bounds of its policy: "3 in
// 1-5", with ", off" when the autoscaler leaves it.
func (p scalingPage) countIn() string {
	count, policy := strconv.Itoa(p.scaling.Count), p.scaling.Policy

	switch {
	case policy == nil:
		return count
	case !policy.Enabled:
		return count + " in " + bounds(*policy) + ", off"
	}

	return count + " in " + bounds(*policy)
}

func (scalingPage) titles() []string { return scalingTitles }

// topics: a scale writes a version of the job. A report that left the count
// shows on the next poll.
func (scalingPage) topics() []string { return []string{nomad.TopicJob} }

// where is the namespace of the job, whatever the session looks at.
func (p scalingPage) where(env) string { return p.namespace }

func (p scalingPage) fetch(e env) tea.Cmd {
	client, namespace, jobID, group := e.client, p.namespace, p.jobID, p.group

	return request(func(ctx context.Context) (nomad.GroupScaling, error) {
		return client.GroupScaling(ctx, namespace, jobID, group)
	}, func(scaling nomad.GroupScaling) tea.Msg { return scalingMsg(scaling) })
}

func (p scalingPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	scaling, ok := msg.(scalingMsg)
	if !ok {
		return p, outcome{}, false
	}

	p.scaling, p.read = nomad.GroupScaling(scaling), true

	return p, outcome{}, true
}

// rows are the events, newest first; one that failed is red.
func (p scalingPage) rows(env) []tableRow {
	rows := make([]tableRow, 0, len(p.scaling.Events))

	for _, event := range p.scaling.Events {
		row := tableRow{}
		if event.Error {
			row.color = colorDead
		}

		row.add(change(event), event.Message)
		row.addAge(event.At)

		rows = append(rows, row)
	}

	return rows
}

// change is what an event did to the count: a dash for a report that left
// it.
func change(event nomad.ScalingEvent) string {
	if event.To == nil {
		return "-"
	}

	return fmt.Sprintf("%d → %d", event.From, *event.To)
}

var scalingKeys = []pageKey[scalingPage]{
	{press: "d", label: "Describe", do: describeGroupPolicy, offered: func(p scalingPage, _ env) bool { return p.scaling.Policy != nil }},
}

func (p scalingPage) keys(e env) []keyHint { return hintsOf(p, e, scalingKeys) }

func (p scalingPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, scalingKeys, k)
}

func describeGroupPolicy(p scalingPage, e env) (scalingPage, outcome) {
	if p.scaling.Policy == nil {
		return p, outcome{}
	}

	return p, outcome{cmd: describePolicy(e.client, p.namespace, p.jobID, p.group, p.scaling.Policy.ID)}
}

// bounds is the count a policy keeps a group in: "1-5".
func bounds(policy nomad.GroupPolicy) string { return fmt.Sprintf("%d-%d", policy.Min, policy.Max) }

// policyCell is the policy of a group in a row of task groups: its bounds,
// "off" after them when the autoscaler leaves it, a dash for none.
func policyCell(policy *nomad.GroupPolicy) string {
	switch {
	case policy == nil:
		return "-"
	case !policy.Enabled:
		return bounds(*policy) + " off"
	}

	return bounds(*policy)
}
