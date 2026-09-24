package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// shortOfRoom is an evaluation that placed nothing: one group has nowhere to
// go at all, the other ran out of memory where it could go.
func shortOfRoom() nomad.EvaluationDetail {
	return nomad.EvaluationDetail{
		Evaluation: nomad.Evaluation{
			ID:          "1ad88fd0-6b1c-da67-5e8f-482b5a15af1f",
			JobID:       "web",
			Namespace:   "production",
			Type:        "service",
			TriggeredBy: "queued-allocs",
			Status:      "blocked",
			Created:     time.Now().Add(-3 * time.Minute),
		},
		StatusDescription: "created to place remaining allocations",
		PreviousEval:      "15d164c9-0000-0000-0000-000000000000",
		Failures: []nomad.PlacementFailure{
			{
				Group:          "api",
				Unplaced:       1,
				QuotaExhausted: []string{"cpu exhausted (500 needed > 400 limit)"},
			},
			{
				Group:              "web",
				Unplaced:           3,
				NodesEvaluated:     3,
				NodesAvailable:     map[string]int{"dc1": 3, "dc2": 0},
				ConstraintFiltered: map[string]int{"${attr.kernel.name} = linux": 1},
				NodesExhausted:     2,
				DimensionExhausted: map[string]int{"memory": 2},
			},
		},
		Related: []nomad.Evaluation{
			{ID: "15d164c9-0000-0000-0000-000000000000", Status: "complete", TriggeredBy: "job-register"},
		},
	}
}

func TestPlacementLines(t *testing.T) {
	r := require.New(t)

	// In the words of the nomad command, which is where people have read
	// them before, with one node said as one node.
	r.Equal([]string{
		`Task group "api" failed to place 1 allocation:`,
		`  * No nodes were eligible for evaluation`,
		`  * Quota limit hit "cpu exhausted (500 needed > 400 limit)"`,
		``,
		`Task group "web" failed to place 3 allocations:`,
		`  * No nodes are available in datacenter "dc2"`,
		`  * Constraint "${attr.kernel.name} = linux": 1 node excluded by filter`,
		`  * Resources exhausted on 2 nodes`,
		`  * Dimension "memory" exhausted on 2 nodes`,
	}, placementLines(shortOfRoom().Failures))
}

func TestEvaluation_OpensFromTheList(t *testing.T) {
	r := require.New(t)

	listed := []nomad.Evaluation{shortOfRoom().Evaluation}
	client := &fakeClient{evaluations: listed, evaluation: shortOfRoom()}

	m := newTestModel(client)
	m, _ = m.update(key(':'))
	m = typeIn(m, "evals")
	m, _ = m.update(enter())
	m, _ = m.update(evaluationsMsg(listed))

	m, cmd := m.update(enter())
	m = drain(m, cmd)

	// The evaluation is asked for where it lives.
	r.Equal("production", client.askedNamespace)
	r.Equal("1ad88fd0-6b1c-da67-5e8f-482b5a15af1f", client.askedID)

	r.Equal(screenDescribe, m.screen.kind)

	r.Contains(plain(m.render()), "Evaluation 1ad88fd0")

	// The page runs past the screen; what is on it is what the text holds.
	page := strings.Join(m.text.lines, "\n")
	r.Contains(page, "blocked: created to place remaining allocations")
	r.Contains(page, "queued-allocs")
	r.Contains(page, `Dimension "memory" exhausted on 2 nodes`)
}

func TestEvaluationText(t *testing.T) {
	r := require.New(t)

	text := evaluationText(shortOfRoom())

	r.Contains(text, "Evaluation    1ad88fd0-6b1c-da67-5e8f-482b5a15af1f")
	r.Contains(text, "Triggered by  queued-allocs")
	r.Contains(text, "Previous      15d164c9-0000-0000-0000-000000000000")
	r.Contains(text, "Placement failures")
	r.Contains(text, "Related evaluations")
	r.Contains(text, "15d164c9  complete  job-register")

	// A field the evaluation does not have is not a line of blanks.
	r.NotContains(text, "Blocked by")
	r.NotContains(text, "Next  ")
}

func TestEvaluationText_WhenEverythingWasPlaced(t *testing.T) {
	r := require.New(t)

	placed := nomad.EvaluationDetail{Evaluation: nomad.Evaluation{ID: "e1", Status: "complete"}}

	text := evaluationText(placed)
	r.NotContains(text, "Placement failures")
	r.True(strings.HasPrefix(text, "Evaluation    e1"))
}
