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
	// it was planned. A nil to means the version before the one that runs.
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

// planDoneMsg says a plan of the job was submitted: it cannot be sent again.
type planDoneMsg struct{ jobID string }

// planPage is what the cluster would do with a job, before anything is
// sent: the file of an edit submitted, or the job gone back to a version.
type planPage struct {
	planState

	content textContent
}

// planOf is the page of a plan the cluster answered with, for what was
// planned.
func planOf(plan nomad.Plan, state planState) planPage {
	state.index = plan.Index
	state.failed = len(plan.Failures) > 0

	// A revert is planned to a version and from one: the one it goes back
	// to is known now, and the one the job has is what submitting checks.
	if state.revert {
		to := plan.To
		state.to, state.from = &to, plan.Version
	}

	return planPage{planState: state, content: paintedText(planText(plan, state.revert)).textContent}
}

// title says whose plan it is, and for a revert, to which version.
func (p planPage) title(env, int) string {
	if p.revert && p.to != nil {
		return sprintf("Revert (Job: %s, Version: %d)", p.jobID, *p.to)
	}

	return sprintf("Plan (Job: %s)", p.jobID)
}

func (planPage) titles() []string       { return nil }
func (planPage) topics() []string       { return nil }
func (planPage) fetch(env) tea.Cmd      { return nil }
func (planPage) rows(env) []tableRow    { return nil }
func (p planPage) text(env) textContent { return p.content }

// take puts a new plan of the same job in place of this one, not on top of
// it: escape still goes back to where the plan was asked from. After a plan
// is submitted, the screen goes back to where the edit started.
func (p planPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	switch msg := msg.(type) {
	case openMsg:
		again, ok := msg.page.(planPage)
		if !ok || again.jobID != p.jobID {
			return p, outcome{}, false
		}

		return again, outcome{reading: true}, true

	case planDoneMsg:
		if msg.jobID != p.jobID {
			return p, outcome{}, false
		}

		return p, outcome{now: []tea.Msg{backMsg{}}, reading: true}, true
	}

	return p, outcome{}, false
}

var planKeys = append([]pageKey[planPage]{
	// The key that sends a plan says what it sends.
	{press: "y", label: "Submit", do: submitPlan, writes: true, offered: planOfAnEdit},
	{press: "y", label: "Revert", do: submitPlan, writes: true, offered: planOfARevert},
	{press: "r", label: "Replan", do: replan},
}, textKeys[planPage]()...)

func (p planPage) keys(e env) []keyHint { return hintsOf(p, e, planKeys) }

func (p planPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, planKeys, k)
}

func planOfAnEdit(p planPage, _ env) bool { return !p.revert && !p.failed }

func planOfARevert(p planPage, _ env) bool { return p.revert && !p.failed }

// sends says there is something to send, and a key to send it with.
func (p planPage) sends(e env) bool { return !p.failed && !e.readOnly }

// button moves between the buttons at the foot of a plan, and presses the
// one the cursor is on.
func (p planPage) button(key string, e env) (page, outcome, bool) {
	switch key {
	case "left", "shift+tab", "h":
		p.choice = buttonCancel

	case "right", "tab", "l":
		if p.sends(e) {
			p.choice = buttonConfirm
		}

	case "enter":
		if p.sends(e) && p.choice == buttonConfirm {
			next, out := submitPlan(p, e)

			return next, out, true
		}

		return p, then(backMsg{}), true

	default:
		return p, outcome{}, false
	}

	return p, outcome{}, true
}

// bar is the question at the foot of a plan, with its buttons: cancel,
// and the one that sends it, or one that shows why nothing can be sent.
func (p planPage) bar(_ env, width int) []string {
	verb := planVerb(p.revert)

	question := fmt.Sprintf("%s %s?", verb, p.jobID)
	if p.revert && p.to != nil {
		question = fmt.Sprintf("%s %s to version %d?", verb, p.jobID, *p.to)
	}

	send := button(verb, p.choice == buttonConfirm)
	if p.failed {
		send = styleButtonOff.Render(fmt.Sprintf(" Can't %s: placement failed ", strings.ToLower(verb)))
	}

	buttons := button("Cancel", p.choice == buttonCancel) + "  " + send
	gap := max(width-ansi.StringWidth(question)-ansi.StringWidth(buttons), 1)

	return []string{
		styleMuted.Render(strings.Repeat("─", width)),
		truncate(styleText.Render(question)+strings.Repeat(" ", gap)+buttons, width),
	}
}

// planFor asks the cluster what submitting the file, or going back to the
// version, would do, and opens the plan.
func planFor(client jobsClient, state planState) tea.Cmd {
	return request(func(ctx context.Context) (nomad.Plan, error) {
		if state.revert {
			return client.PlanRevert(ctx, state.namespace, state.jobID, state.to)
		}

		return client.PlanJob(ctx, state.namespace, state.source, state.vars)
	}, func(plan nomad.Plan) tea.Msg { return openMsg{planOf(plan, state)} })
}

// submitPlan sends what was planned: the file at the index it was planned
// at, or the revert from the version it was planned from. The result shows
// whether or not the plan is still open; when the job changed since the
// plan, the plan stays open with its file, to be planned again.
func submitPlan(p planPage, e env) (planPage, outcome) {
	client, state := e.client, p.planState

	return p, outcome{cmd: func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		said := fmt.Sprintf("Job %s submitted.", state.jobID)

		var err error
		if state.revert && state.to != nil {
			said = fmt.Sprintf("Job %s reverted to version %d.", state.jobID, *state.to)
			err = client.RevertJobTo(ctx, state.namespace, state.jobID, *state.to, state.from)
		} else {
			err = client.SubmitJob(ctx, state.namespace, state.source, state.vars, state.index)
		}

		switch {
		case errors.Is(err, nomad.ErrJobChanged):
			return warnMsg(fmt.Sprintf("%s changed since this plan: r plans it again", state.jobID))

		case err != nil:
			return failMsg{err: err}
		}

		return tea.BatchMsg{
			func() tea.Msg { return planDoneMsg{jobID: state.jobID} },
			func() tea.Msg { return sayMsg(said) },
		}
	}}
}

// replan asks about the same file, or the same version, again.
func replan(p planPage, e env) (planPage, outcome) {
	return p, outcome{cmd: planFor(e.client, p.planState)}
}

// planText is a plan as a page: first the placement failures and the
// warnings, then what the scheduler would do to each group, then what would
// change, line by line.
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
