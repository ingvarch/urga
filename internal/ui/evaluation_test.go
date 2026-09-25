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

	// In the words of the nomad command, which people already know, with
	// the singular for one node or one allocation.
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

	// The evaluation is requested in its own namespace.
	r.Equal("production", client.askedNamespace)
	r.Equal("1ad88fd0-6b1c-da67-5e8f-482b5a15af1f", client.askedID)

	r.IsType(describePage{}, m.screen.page)

	r.Contains(plain(m.render()), "Evaluation 1ad88fd0")

	// The page is longer than the screen, so the test checks its text.
	page := strings.Join(m.text.lines, "\n")
	r.Contains(page, "blocked: created to place remaining allocations")
	r.Contains(page, "queued-allocs")
	r.Contains(page, `Dimension "memory" exhausted on 2 nodes`)
}

func TestEvaluation_OpensTheOneUnderTheCursorWhereItLives(t *testing.T) {
	r := require.New(t)

	other := nomad.Evaluation{ID: "e2222222-0000-0000-0000-000000000000", JobID: "billing", Namespace: "default", Status: "complete"}
	listed := []nomad.Evaluation{shortOfRoom().Evaluation, other}
	client := &fakeClient{evaluations: listed, evaluation: shortOfRoom()}

	// Every namespace is listed, so the one of the session is not the one
	// of the evaluation.
	m, _ := runLine(newTestModel(client), "evals")
	m, _ = m.update(key('0'))
	m, _ = m.update(evaluationsMsg(listed))
	m, _ = m.update(down())

	m, cmd := m.update(enter())
	drain(m, cmd)

	r.Equal("default", client.askedNamespace)
	r.Equal(other.ID, client.askedID)
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

	// A field the evaluation does not have gets no line.
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

// waiting are the jobs of the tests with one allocation of web that waits
// for a place.
func waiting() []nomad.Job {
	jobs := twoJobs()
	jobs[0].Queued = 1

	return jobs
}

// offers says the screen has the key in its header now.
func offers(m Model, press string) bool {
	for _, h := range m.hints() {
		if h.Key == "<"+press+">" {
			return true
		}
	}

	return false
}

func TestPlacement_FromAJobThatWaits(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: waiting(), evaluation: shortOfRoom()}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(waiting()))

	// A job with an allocation that waits for a place offers to show why.
	r.True(offers(m, "p"))

	m, cmd := m.update(key('p'))
	m = drain(m, cmd)

	r.Equal("production", client.askedNamespace)
	r.Equal("web", client.askedJobID)

	r.IsType(describePage{}, m.screen.page)
	r.Contains(plain(m.render()), "Placement (Job: web)")
	r.Contains(strings.Join(m.text.lines, "\n"), `Dimension "memory" exhausted on 2 nodes`)
}

func TestPlacement_NotOfferedWhenNothingWaits(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{jobs: twoJobs()})
	m, _ = m.update(jobsMsg(twoJobs()))

	// Every allocation of web is placed: there is nothing to explain.
	r.False(offers(m, "p"))
}

func TestPlacement_FromATaskGroupThatWaits(t *testing.T) {
	r := require.New(t)

	groups := twoGroups()
	groups[0].Queued = 2

	client := &fakeClient{jobs: twoJobs(), groups: groups, evaluation: shortOfRoom()}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(key('t'))
	m, _ = m.update(taskGroupsMsg(groups))

	r.True(offers(m, "p"))

	m, cmd := m.update(key('p'))
	m = drain(m, cmd)

	r.Equal("web", client.askedJobID)
	r.Contains(plain(m.render()), "Placement (Job: web)")
}

func TestPlacement_WhenNoEvaluationSaysWhy(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: waiting(), placementErr: nomad.ErrNoFailures}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(waiting()))

	m, cmd := m.update(key('p'))
	m = drain(m, cmd)

	// The page shows that message, and no error: the scheduler may not
	// have got to the job yet.
	r.IsType(describePage{}, m.screen.page)
	r.Contains(plain(m.render()), "No evaluation of web says why")
}

func TestEvaluations_TheListOfTheNamespaceAndItsKeys(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{evaluations: []nomad.Evaluation{shortOfRoom().Evaluation}}
	m := typeCommand(newTestModel(client), "evaluations")

	// Requested in the namespace of the session, which the title shows.
	r.Equal("production", client.askedNamespace)
	r.Contains(plain(m.render()), "Evaluations (production) [1]")

	header := fileRow(t, m, -1)
	for _, title := range []string{"ID", "JobID", "Namespace", "Type", "TriggeredBy", "Status", "Age"} {
		r.Contains(header, title)
	}

	row := fileRow(t, m, 0)
	for _, cell := range []string{"1ad88fd0", "web", "production", "service", "queued-allocs", "blocked", "3m"} {
		r.Contains(row, cell)
	}

	r.NotContains(row, "1ad88fd0-6b1c")

	// A blocked one waits.
	r.Equal(colorPending, m.list.table.rows[0].color)

	r.Equal([]hint{{Key: "<enter>", Description: "Details"}}, m.hints())
}

func TestEvaluations_AListThatAnswersLateIsDropped(t *testing.T) {
	r := require.New(t)

	listed := []nomad.Evaluation{shortOfRoom().Evaluation}
	client := &fakeClient{evaluations: listed, evaluation: shortOfRoom()}

	m, _ := runLine(newTestModel(client), "evals")
	m, _ = m.update(evaluationsMsg(listed))

	m, cmd := m.update(enter())
	m = drain(m, cmd)

	// The list answers while one of its evaluations is open.
	m, _ = m.update(evaluationsMsg{{ID: "b1111111-0000-0000-0000-000000000000", JobID: "billing", Namespace: "production"}})
	r.Contains(plain(m.render()), "Evaluation 1ad88fd0")

	// Back on the list, it shows the rows it had.
	m, _ = m.update(escape())

	out := plain(m.render())
	r.Contains(out, "Evaluations (production) [1]")
	r.Contains(fileRow(t, m, 0), "1ad88fd0")
	r.NotContains(out, "billing")
}

func TestEvaluations_OfTheRegionLeftAreLetGo(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{evaluations: []nomad.Evaluation{shortOfRoom().Evaluation}}
	m := regionalModel(t, client)
	m = typeCommand(m, "evaluations")
	r.Contains(fileRow(t, m, 0), "1ad88fd0")

	// The evaluations of eu must not be shown under the name of us before
	// us answers.
	client.evaluations = nil
	m, _ = runLine(m, "region us")

	out := plain(m.render())
	r.Contains(out, "Evaluations (production) [0]")
	r.NotContains(out, "1ad88fd0")
}
