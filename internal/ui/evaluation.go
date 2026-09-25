package ui

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"maps"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// evaluationsPage is the evaluations of the namespace the session looks at.
type evaluationsPage struct {
	ofTheSession

	evaluations []nomad.Evaluation
}

var evaluationTitles = []string{"ID", "JobID", "Namespace", "Type", "TriggeredBy", "Status", "Age"}

func (evaluationsPage) title(e env, count int) string {
	return sprintf("Evaluations (%s) [%d]", namespaceLabel(e.namespace), count)
}

func (evaluationsPage) titles() []string { return evaluationTitles }
func (evaluationsPage) topics() []string { return []string{nomad.TopicEvaluation} }

func (evaluationsPage) fetch(e env) tea.Cmd {
	client, namespace := e.client, e.namespace

	return fetchList(func(ctx context.Context) ([]nomad.Evaluation, error) {
		return client.Evaluations(ctx, namespace)
	}, func(items []nomad.Evaluation) tea.Msg { return evaluationsMsg(items) })
}

func (p evaluationsPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	evaluations, ok := msg.(evaluationsMsg)
	if !ok {
		return p, outcome{}, false
	}

	p.evaluations = evaluations

	return p, outcome{}, true
}

func (p evaluationsPage) rows(env) []tableRow { return evaluationRows(p.evaluations) }

func evaluationRows(evals []nomad.Evaluation) []tableRow {
	rows := make([]tableRow, 0, len(evals))

	for _, e := range evals {
		row := tableRow{color: evaluationColor(e)}
		row.add(shortID(e.ID), e.JobID, e.Namespace, e.Type, e.TriggeredBy, e.Status)
		row.addAge(e.Created)

		rows = append(rows, row)
	}

	return rows
}

func evaluationColor(e nomad.Evaluation) color.Color {
	switch e.Status {
	case "pending", "blocked":
		return colorPending
	case "failed", "canceled":
		return colorDead
	}

	return nil
}

var evaluationsKeys = []pageKey[evaluationsPage]{{press: "enter", label: "Details", do: openEvaluation}}

func (p evaluationsPage) keys(e env) []keyHint { return hintsOf(p, e, evaluationsKeys) }

func (p evaluationsPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, evaluationsKeys, k)
}

// openEvaluation reads the evaluation under the cursor in full: how it
// ended, and why it placed nothing when it did not.
func openEvaluation(p evaluationsPage, e env) (evaluationsPage, outcome) {
	eval, ok := pickedFrom(e, p.evaluations)
	if !ok {
		return p, outcome{}
	}

	return p, outcome{cmd: describeEvaluation(e.client, eval.Namespace, eval.ID)}
}

// describeEvaluation reads one evaluation in full.
func describeEvaluation(client evaluationsClient, namespace, id string) tea.Cmd {
	return describe(fmt.Sprintf("Evaluation %s", shortID(id)), func(ctx context.Context) (string, error) {
		detail, err := client.Evaluation(ctx, namespace, id)
		if err != nil {
			return "", err
		}

		return evaluationText(detail), nil
	})
}

// jobWaits and groupWaits say the row under the cursor has allocations that
// wait for a place: there is a why to ask only then.
func jobWaits(p jobsPage, e env) bool {
	job, ok := p.picked(e)

	return ok && job.Queued > 0
}

func groupWaits(p taskGroupsPage, e env) bool {
	group, ok := p.picked(e)

	return ok && group.Queued > 0
}

// jobPlacement and groupPlacement say why what waits of the job is not
// placed. A group has no evaluation of its own: the one of its job holds
// every group.
func jobPlacement(p jobsPage, e env) (jobsPage, outcome) {
	job, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	return p, outcome{cmd: placementOf(e.client, job.Namespace, job.ID)}
}

func groupPlacement(p taskGroupsPage, e env) (taskGroupsPage, outcome) {
	if _, ok := p.picked(e); !ok {
		return p, outcome{}
	}

	return p, outcome{cmd: placementOf(e.client, p.namespace, p.jobID)}
}

// placementOf opens the newest evaluation of the job that failed to place
// something.
func placementOf(client evaluationsClient, namespace, jobID string) tea.Cmd {
	return describe(fmt.Sprintf("Placement (Job: %s)", jobID), func(ctx context.Context) (string, error) {
		eval, err := client.FailedPlacement(ctx, namespace, jobID)

		// Saying so is the page: the scheduler may not have got to the job
		// yet, or its evaluations were collected.
		if errors.Is(err, nomad.ErrNoFailures) {
			return fmt.Sprintf(
				"No evaluation of %s says why it waits.\n\n"+
					"The scheduler may not have got to it yet, or its evaluations were collected.", jobID), nil
		}

		if err != nil {
			return "", err
		}

		return evaluationText(eval), nil
	})
}

// evaluationText is an evaluation as a page: its fields, why each group it
// could not place was not placed, and the evaluations it is chained to.
func evaluationText(eval nomad.EvaluationDetail) string {
	status := eval.Status
	if eval.StatusDescription != "" {
		status += ": " + eval.StatusDescription
	}

	fields := [][2]string{
		{"Evaluation", eval.ID},
		{"Job", eval.JobID},
		{"Namespace", eval.Namespace},
		{"Type", eval.Type},
		{"Triggered by", eval.TriggeredBy},
		{"Status", status},
		{"Created", ageOf(eval.Created) + " ago"},
		{"Previous", eval.PreviousEval},
		{"Blocked by", eval.BlockedEval},
		{"Next", eval.NextEval},
	}

	lines := []string{}

	for _, field := range fields {
		// A field the evaluation does not have is left out, not drawn blank.
		if field[1] == "" || field[1] == "- ago" {
			continue
		}

		lines = append(lines, fmt.Sprintf("%-13s %s", field[0], field[1]))
	}

	if len(eval.Failures) > 0 {
		lines = append(lines, "", "Placement failures", "")
		lines = append(lines, placementLines(eval.Failures)...)
	}

	if len(eval.Related) > 0 {
		lines = append(lines, "", "Related evaluations", "")

		for _, related := range eval.Related {
			lines = append(lines, fmt.Sprintf("  %s  %s  %s", shortID(related.ID), related.Status, related.TriggeredBy))
		}
	}

	return strings.Join(lines, "\n")
}

// placementLines says why each group was not placed, in the words of the
// nomad command, which is where people have read them before.
func placementLines(failures []nomad.PlacementFailure) []string {
	lines := []string{}

	for i, failure := range failures {
		if i > 0 {
			lines = append(lines, "")
		}

		lines = append(lines, fmt.Sprintf("Task group %q failed to place %s:", failure.Group, plural(failure.Unplaced, "allocation")))

		for _, reason := range placementReasons(failure) {
			lines = append(lines, "  * "+reason)
		}
	}

	return lines
}

// placementReasons are the reasons one group was not placed: first the
// nodes that were never in the running, then the ones that ran out of room.
// A map is read in the order of its keys, so the lines stay where they are
// from one reading to the next.
func placementReasons(failure nomad.PlacementFailure) []string {
	reasons := []string{}

	if failure.NodesEvaluated == 0 {
		reasons = append(reasons, "No nodes were eligible for evaluation")
	}

	for _, dc := range slices.Sorted(maps.Keys(failure.NodesAvailable)) {
		if failure.NodesAvailable[dc] == 0 {
			reasons = append(reasons, fmt.Sprintf("No nodes are available in datacenter %q", dc))
		}
	}

	for _, class := range slices.Sorted(maps.Keys(failure.ClassFiltered)) {
		reasons = append(reasons, fmt.Sprintf("Class %q: %s excluded by filter", class, plural(failure.ClassFiltered[class], "node")))
	}

	for _, constraint := range slices.Sorted(maps.Keys(failure.ConstraintFiltered)) {
		reasons = append(reasons, fmt.Sprintf("Constraint %q: %s excluded by filter", constraint, plural(failure.ConstraintFiltered[constraint], "node")))
	}

	if failure.NodesExhausted > 0 {
		reasons = append(reasons, fmt.Sprintf("Resources exhausted on %s", plural(failure.NodesExhausted, "node")))
	}

	for _, class := range slices.Sorted(maps.Keys(failure.ClassExhausted)) {
		reasons = append(reasons, fmt.Sprintf("Class %q exhausted on %s", class, plural(failure.ClassExhausted[class], "node")))
	}

	for _, dimension := range slices.Sorted(maps.Keys(failure.DimensionExhausted)) {
		reasons = append(reasons, fmt.Sprintf("Dimension %q exhausted on %s", dimension, plural(failure.DimensionExhausted[dimension], "node")))
	}

	for _, quota := range failure.QuotaExhausted {
		reasons = append(reasons, fmt.Sprintf("Quota limit hit %q", quota))
	}

	return reasons
}

// plural is a count and what it counts: 1 node, 2 nodes.
func plural(count int, noun string) string {
	if count == 1 {
		return "1 " + noun
	}

	return fmt.Sprintf("%d %ss", count, noun)
}
