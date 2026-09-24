package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// planState is what a plan screen was made of and where it was planned
// from: what submitting sends, and what planning again asks about.
type planState struct {
	namespace string
	jobID     string

	// source and vars are the job file of an edit.
	source string
	vars   nomad.JobVariables
	index  uint64

	// revert goes back to the version to, from the version the job had when
	// it was planned. No version to is the one before the version that runs.
	revert bool
	to     *uint64
	from   uint64
}

// Messages of a plan.
type (
	// planMsg is a plan the cluster answered with, for the file it carries.
	planMsg struct {
		plan  nomad.Plan
		state planState
	}

	// planDoneMsg is what came of submitting a plan, and what to say when
	// it went through.
	planDoneMsg struct {
		jobID string
		said  string
		err   error
	}
)

var planBindings = []binding{
	// The key that sends a plan says what it sends.
	{press: "y", label: "Submit", do: submitPlan, writes: true, offered: planOfAnEdit},
	{press: "y", label: "Revert", do: submitPlan, writes: true, offered: planOfARevert},
	{press: "r", label: "Replan", do: replan},
	{press: "w", label: "Toggle Wrap", do: wrapLines},
	{press: "ctrl+s", label: "Save", do: saveScreen},
}

func planOfAnEdit(m Model) bool { return !m.plan.revert }

func planOfARevert(m Model) bool { return m.plan.revert }

// planFor asks the cluster what submitting the file, or going back to the
// version, would do.
func planFor(client Client, state planState) tea.Cmd {
	return request(func(ctx context.Context) (planMsg, error) {
		if state.revert {
			plan, err := client.PlanRevert(ctx, state.namespace, state.jobID, state.to)

			return planMsg{plan: plan, state: state}, err
		}

		plan, err := client.PlanJob(ctx, state.namespace, state.source, state.vars)

		return planMsg{plan: plan, state: state}, err
	}, func(msg planMsg) tea.Msg { return msg })
}

// planTitle says whose plan it is, and for a revert, to which version.
func planTitle(m Model) string {
	if m.plan.revert && m.plan.to != nil {
		return sprintf("Revert (Job: %s, Version: %d)", m.screen.jobID, *m.plan.to)
	}

	return sprintf("Plan (Job: %s)", m.screen.jobID)
}

// showPlan puts a plan up, or in place of the one on the screen when it is
// the same job planned again.
func (m Model) showPlan(msg planMsg) Model {
	state := msg.state
	state.index = msg.plan.Index

	// A revert is planned to a version and from one: the one it goes back
	// to is known now, and the one the job has is what submitting checks.
	if state.revert {
		to := msg.plan.To
		state.to, state.from = &to, msg.plan.Version
	}

	if m.screen.kind == screenPlan && m.screen.jobID == state.jobID {
		m.plan = state
		planned := paintedText(planText(msg.plan))
		m.text.lines, m.text.paint = planned.lines, planned.paint
		m.layout()

		return m
	}

	m = m.stackText(screen{kind: screenPlan, namespace: state.namespace, jobID: state.jobID}, paintedText(planText(msg.plan)))
	m.plan = state

	return m
}

// submitPlan sends what was planned: the file at the index it was planned
// at, or the revert from the version it was planned from.
func submitPlan(m Model) (Model, tea.Cmd) {
	client, state := m.client, m.plan

	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		if state.revert && state.to != nil {
			err := client.RevertJobTo(ctx, state.namespace, state.jobID, *state.to, state.from)

			return planDoneMsg{jobID: state.jobID, said: fmt.Sprintf("Job %s reverted to version %d.", state.jobID, *state.to), err: err}
		}

		err := client.SubmitJob(ctx, state.namespace, state.source, state.vars, state.index)

		return planDoneMsg{jobID: state.jobID, said: fmt.Sprintf("Job %s submitted.", state.jobID), err: err}
	}
}

// replan asks about the same file, or the same version, again.
func replan(m Model) (Model, tea.Cmd) {
	return m, planFor(m.client, m.plan)
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

	return m.say(msg.said)
}

// planText is a plan as a page: what would change, as git diff shows it,
// what the scheduler would do to each group, what it could not place, and
// what the cluster warns of.
func planText(plan nomad.Plan) []paintedLine {
	lines := plainLines("Changes\n")

	if len(plan.Diff) == 0 {
		lines = append(lines, paintedLine{text: "Nothing in the job changes."})
	} else {
		lines = append(lines, diffLines(plan.Diff)...)
	}

	if len(plan.Groups) > 0 {
		lines = append(lines, plainLines("\nScheduler\n")...)

		for _, group := range plan.Groups {
			lines = append(lines, paintedLine{text: fmt.Sprintf("  Task group %q: %s", group.Name, groupUpdates(group))})
		}
	}

	if len(plan.Failures) > 0 {
		lines = append(lines, plainLines("\nPlacement failures\n")...)
		lines = append(lines, plainLines(strings.Join(placementLines(plan.Failures), "\n"))...)
	}

	if plan.Warnings != "" {
		lines = append(lines, plainLines("\nWarnings\n")...)
		lines = append(lines, plainLines(strings.TrimRight(plan.Warnings, "\n"))...)
	}

	return lines
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
