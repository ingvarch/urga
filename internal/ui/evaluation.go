package ui

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

var evaluationBindings = []binding{{press: "enter", label: "Details", do: openEvaluation}}

// openEvaluation reads the evaluation under the cursor in full: how it
// ended, and why it placed nothing when it did not.
func openEvaluation(m Model) (Model, tea.Cmd) {
	eval, ok := selectedOf(m, screenEvaluations, m.evaluations)
	if !ok {
		return m, nil
	}

	client := m.client

	return m, describe(fmt.Sprintf("Evaluation %s", shortID(eval.ID)), func(ctx context.Context) (string, error) {
		detail, err := client.Evaluation(ctx, eval.Namespace, eval.ID)
		if err != nil {
			return "", err
		}

		return evaluationText(detail), nil
	})
}

// jobWaits and groupWaits say the row under the cursor has allocations that
// wait for a place: there is a why to ask only then.
func jobWaits(m Model) bool {
	job, ok := selectedOf(m, screenJobs, m.jobs)

	return ok && job.Queued > 0
}

func groupWaits(m Model) bool {
	group, ok := selectedOf(m, screenTaskGroups, m.groups)

	return ok && group.Queued > 0
}

// jobPlacement and groupPlacement say why what waits of the job is not
// placed. A group has no evaluation of its own: the one of its job holds
// every group.
func jobPlacement(m Model) (Model, tea.Cmd) {
	job, ok := selectedOf(m, screenJobs, m.jobs)
	if !ok {
		return m, nil
	}

	return m, placementOf(m.client, job.Namespace, job.ID)
}

func groupPlacement(m Model) (Model, tea.Cmd) {
	if _, ok := selectedOf(m, screenTaskGroups, m.groups); !ok {
		return m, nil
	}

	return m, placementOf(m.client, m.screen.namespace, m.screen.jobID)
}

// placementOf opens the newest evaluation of the job that failed to place
// something.
func placementOf(client Client, namespace, jobID string) tea.Cmd {
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
