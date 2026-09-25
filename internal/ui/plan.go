package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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

	// failed says the cluster has no room for what was planned: there is
	// nothing to send.
	failed bool

	// choice is the button at the foot of the plan the cursor is on. It
	// starts on cancel: enter out of habit then sends nothing.
	choice int
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

func planOfAnEdit(m Model) bool { return !m.plan.revert && !m.plan.failed }

func planOfARevert(m Model) bool { return m.plan.revert && !m.plan.failed }

// sendKey is the key that sends the plan, when there is one to send.
func (m Model) sendKey() (binding, bool) {
	return m.binding("y")
}

// planButtonKey moves between the buttons at the foot of a plan, and presses
// the one the cursor is on.
func (m Model) planButtonKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.screen.kind != screenPlan {
		return m, nil, false
	}

	switch msg.String() {
	case "left", "shift+tab", "h":
		m.plan.choice = buttonCancel

	case "right", "tab", "l":
		if _, ok := m.sendKey(); ok {
			m.plan.choice = buttonConfirm
		}

	case "enter":
		if send, ok := m.sendKey(); ok && m.plan.choice == buttonConfirm {
			next, cmd := send.do(m)

			return next, cmd, true
		}

		next, cmd := m.back()

		return next, cmd, true

	default:
		return m, nil, false
	}

	return m, nil, true
}

// planBar is the question at the foot of a plan, with its buttons: cancel,
// and the one that sends it, or says why nothing can be sent.
func (m Model) planBar(width int) string {
	verb := planVerb(m.plan.revert)

	question := fmt.Sprintf("%s %s?", verb, m.screen.jobID)
	if m.plan.revert && m.plan.to != nil {
		question = fmt.Sprintf("%s %s to version %d?", verb, m.screen.jobID, *m.plan.to)
	}

	send := button(verb, m.plan.choice == buttonConfirm)
	if m.plan.failed {
		send = styleButtonOff.Render(fmt.Sprintf(" Can't %s: placement failed ", strings.ToLower(verb)))
	}

	buttons := button("Cancel", m.plan.choice == buttonCancel) + "  " + send
	gap := max(width-ansi.StringWidth(question)-ansi.StringWidth(buttons), 1)

	return styleMuted.Render(strings.Repeat("─", width)) + "\n" +
		truncate(styleText.Render(question)+strings.Repeat(" ", gap)+buttons, width)
}

// barRows is how many rows of the box the question at the foot of a plan
// takes.
func (m Model) barRows() int {
	if m.screen.kind == screenPlan {
		return 2
	}

	return 0
}

// planFor asks the cluster what submitting the file, or going back to the
// version, would do.
func planFor(client jobsClient, state planState) tea.Cmd {
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
	state.failed = len(msg.plan.Failures) > 0

	// A revert is planned to a version and from one: the one it goes back
	// to is known now, and the one the job has is what submitting checks.
	if state.revert {
		to := msg.plan.To
		state.to, state.from = &to, msg.plan.Version
	}

	if m.screen.kind == screenPlan && m.screen.jobID == state.jobID {
		m.plan = state
		planned := paintedText(planText(msg.plan, state.revert))
		m.text.lines, m.text.paint = planned.lines, planned.paint
		m.layout()

		return m
	}

	m = m.stackText(screen{kind: screenPlan, namespace: state.namespace, jobID: state.jobID}, paintedText(planText(msg.plan, state.revert)))
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

// planText is a plan as a page: first what keeps it from the cluster and
// what the cluster warns of, then what the scheduler would do to each group,
// then what would change, line by line.
func planText(plan nomad.Plan, revert bool) []paintedLine {
	lines := []paintedLine{}

	if len(plan.Failures) > 0 {
		lines = append(lines, paintedLine{text: "Placement failures", style: &styleError}, paintedLine{})
		lines = append(lines, plainLines(strings.Join(placementLines(plan.Failures), "\n"))...)
		lines = append(lines, paintedLine{})
	}

	if plan.Warnings != "" {
		lines = append(lines, paintedLine{text: "Warnings", style: &styleWarn}, paintedLine{})
		lines = append(lines, plainLines(strings.TrimRight(plan.Warnings, "\n"))...)
		lines = append(lines, paintedLine{})
	}

	lines = append(lines, paintedLine{text: fmt.Sprintf("What will happen when you %s this job:", strings.ToLower(planVerb(revert)))}, paintedLine{})

	if len(plan.Groups) == 0 {
		lines = append(lines, paintedLine{text: "  No changes detected", style: &styleMuted})
	}

	for i, group := range plan.Groups {
		if i > 0 {
			lines = append(lines, paintedLine{})
		}

		lines = append(lines, paintedLine{text: fmt.Sprintf("  Task group %q", group.Name)})
		lines = append(lines, groupUpdates(group)...)
	}

	lines = append(lines, paintedLine{}, paintedLine{text: "Changes"}, paintedLine{})

	if len(plan.Diff) == 0 {
		return append(lines, paintedLine{text: "Nothing in the job changes."})
	}

	return append(lines, diffLines(plan.Diff)...)
}

// planVerb is what sending a plan does to the job.
func planVerb(revert bool) string {
	if revert {
		return "Revert"
	}

	return "Submit"
}

// groupUpdates say what the scheduler would do to the allocations of a
// group, a line for each kind of change in its colour.
func groupUpdates(group nomad.PlanGroup) []paintedLine {
	updates := []struct {
		count int
		what  string
		style *lipgloss.Style
	}{
		{group.Place, plural(group.Place, "new allocation") + " will be created", &styleAdded},
		{group.Stop, plural(group.Stop, "allocation") + " will be stopped", &styleDeleted},
		{group.Migrate, plural(group.Migrate, "allocation") + " will be migrated to " + otherNodes(group.Migrate), &styleTitle},
		{group.InPlace, plural(group.InPlace, "allocation") + " will be updated in-place (no restart)", &stylePending},
		{group.Destructive, plural(group.Destructive, "allocation") + " will be recreated (destructive update)", &styleDestructive},
		{group.Canary, plural(group.Canary, "canary allocation") + " will be deployed", &styleCanary},
		{group.Preemptions, preempted(group.Preemptions), &styleDeleted},
	}

	lines := []paintedLine{}

	for _, update := range updates {
		if update.count > 0 {
			lines = append(lines, paintedLine{text: "    ● " + update.what, style: update.style})
		}
	}

	switch {
	case len(lines) == 0 && group.Ignore > 0:
		return []paintedLine{{text: "    No changes - " + plural(group.Ignore, "allocation") + " will remain unchanged", style: &styleMuted}}

	case len(lines) == 0:
		return []paintedLine{{text: "    No allocations affected", style: &styleMuted}}

	case group.Ignore > 0:
		lines = append(lines, paintedLine{text: "    ● " + plural(group.Ignore, "allocation") + " unchanged", style: &styleMuted})
	}

	return lines
}

// otherNodes is where migrated allocations go.
func otherNodes(count int) string {
	if count == 1 {
		return "another node"
	}

	return "other nodes"
}

// preempted says how many allocations of other jobs make room.
func preempted(count int) string {
	if count == 1 {
		return "1 allocation of another job will be preempted"
	}

	return fmt.Sprintf("%d allocations of other jobs will be preempted", count)
}
