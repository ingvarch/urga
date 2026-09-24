package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// planState is the job file a plan screen was made of and the index it was
// planned at: what submitting sends, and what planning again asks about.
type planState struct {
	namespace string
	jobID     string
	source    string
	vars      nomad.JobVariables
	index     uint64
}

// Messages of a plan.
type (
	// planMsg is a plan the cluster answered with, for the file it carries.
	planMsg struct {
		plan  nomad.Plan
		state planState
	}

	// planDoneMsg is what came of submitting a plan.
	planDoneMsg struct {
		jobID string
		err   error
	}
)

var planBindings = []binding{
	{press: "y", label: "Submit", do: submitPlan, writes: true},
	{press: "r", label: "Replan", do: replan},
	{press: "w", label: "Toggle Wrap", do: wrapLines},
	{press: "ctrl+s", label: "Save", do: saveScreen},
}

// planOf asks the cluster what submitting the file would do.
func planOf(client Client, state planState) tea.Cmd {
	return request(func(ctx context.Context) (planMsg, error) {
		plan, err := client.PlanJob(ctx, state.namespace, state.source, state.vars)

		return planMsg{plan: plan, state: state}, err
	}, func(msg planMsg) tea.Msg { return msg })
}

// showPlan puts a plan up, or in place of the one on the screen when it is
// the same job planned again.
func (m Model) showPlan(msg planMsg) Model {
	state := msg.state
	state.index = msg.plan.Index

	if m.screen.kind == screenPlan && m.screen.jobID == state.jobID {
		m.plan = state
		m.text.lines = strings.Split(planText(msg.plan), "\n")
		m.layout()

		return m
	}

	m = m.stackText(screen{kind: screenPlan, namespace: state.namespace, jobID: state.jobID}, planText(msg.plan))
	m.plan = state

	return m
}

// submitPlan sends what was planned, at the index it was planned at.
func submitPlan(m Model) (Model, tea.Cmd) {
	client, state := m.client, m.plan

	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		err := client.SubmitJob(ctx, state.namespace, state.source, state.vars, state.index)

		return planDoneMsg{jobID: state.jobID, err: err}
	}
}

// replan asks about the same file again.
func replan(m Model) (Model, tea.Cmd) {
	return m, planOf(m.client, m.plan)
}

// finishPlan says what came of a submit. A plan that went through is spent,
// and the screen goes back to where the edit started; one the job changed
// under stays, with its file, to be planned again.
func (m Model) finishPlan(msg planDoneMsg) Model {
	switch {
	case errors.Is(msg.err, nomad.ErrJobChanged):
		return m.warn(fmt.Sprintf("%s changed since this plan: r plans it again", msg.jobID))

	case msg.err != nil:
		return m.fail(msg.err)
	}

	if m.screen.kind == screenPlan {
		m, _ = m.back()
	}

	return m.say(fmt.Sprintf("Job %s submitted.", msg.jobID))
}

// planText is a plan as a page: what would change, what the scheduler would
// do to each group, what it could not place, and what the cluster warns of.
func planText(plan nomad.Plan) string {
	lines := []string{"Changes", ""}

	// A diff parts the job from its groups with an empty line, which leads
	// it when nothing changed on the job itself.
	diff := strings.Trim(plan.Diff, "\n")

	if diff == "" {
		lines = append(lines, "Nothing in the job changes.")
	} else {
		lines = append(lines, strings.Split(diff, "\n")...)
	}

	if len(plan.Groups) > 0 {
		lines = append(lines, "", "Scheduler", "")

		for _, group := range plan.Groups {
			lines = append(lines, fmt.Sprintf("  Task group %q: %s", group.Name, groupUpdates(group)))
		}
	}

	if len(plan.Failures) > 0 {
		lines = append(lines, "", "Placement failures", "")
		lines = append(lines, placementLines(plan.Failures)...)
	}

	if plan.Warnings != "" {
		lines = append(lines, "", "Warnings", "")
		lines = append(lines, strings.Split(strings.TrimRight(plan.Warnings, "\n"), "\n")...)
	}

	return strings.Join(lines, "\n")
}

// groupUpdates says what the scheduler would do to a group, in the words of
// the nomad command.
func groupUpdates(group nomad.PlanGroup) string {
	counts := []struct {
		count int
		what  string
	}{
		{group.Place, "create"},
		{group.Stop, "destroy"},
		{group.InPlace, "in-place update"},
		{group.Destructive, "create/destroy update"},
		{group.Canary, "canary"},
		{group.Migrate, "migrate"},
		{group.Preemptions, "preemption"},
	}

	parts := []string{}

	for _, c := range counts {
		if c.count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", c.count, c.what))
		}
	}

	if len(parts) == 0 {
		return "no change"
	}

	return strings.Join(parts, ", ")
}
