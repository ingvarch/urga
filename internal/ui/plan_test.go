package ui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// changedEnv is the plan of a change of an environment variable of web: the
// task is replaced, and one group is short of memory.
func changedEnv() nomad.Plan {
	return nomad.Plan{
		// A diff with nothing changed on the job itself starts with the line
		// that parts the job from its groups.
		Diff: "\nTask Group: web (edited)\n\n  Task: server (edited)\n    ~ Env[V]: 2 -> 3",
		Groups: []nomad.PlanGroup{
			{Name: "api"},
			{Name: "web", Destructive: 2, Canary: 1},
		},
		Failures: []nomad.PlacementFailure{
			{Group: "web", Unplaced: 1, NodesEvaluated: 1, NodesExhausted: 1, DimensionExhausted: map[string]int{"memory": 1}},
		},
		Warnings: "1 warning:\n\n* Group \"web\" has warnings",
		Index:    42,
	}
}

func TestPlanText(t *testing.T) {
	r := require.New(t)

	text := planText(changedEnv())

	r.Equal(strings.Join([]string{
		"Changes",
		"",
		"Task Group: web (edited)",
		"",
		"  Task: server (edited)",
		"    ~ Env[V]: 2 -> 3",
		"",
		"Scheduler",
		"",
		`  Task group "api": no change`,
		`  Task group "web": 2 create/destroy update, 1 canary`,
		"",
		"Placement failures",
		"",
		`Task group "web" failed to place 1 allocation:`,
		`  * Resources exhausted on 1 node`,
		`  * Dimension "memory" exhausted on 1 node`,
		"",
		"Warnings",
		"",
		"1 warning:",
		"",
		`* Group "web" has warnings`,
	}, "\n"), text)
}

func TestPlanText_NothingChanges(t *testing.T) {
	r := require.New(t)

	text := planText(nomad.Plan{Groups: []nomad.PlanGroup{{Name: "web"}}})

	r.Contains(text, "Nothing in the job changes.")
	r.NotContains(text, "Placement failures")
	r.NotContains(text, "Warnings")
}

// planned is the job list after web was edited and its plan came back.
func planned(t *testing.T, client *fakeClient) Model {
	t.Helper()

	m, editor := editModel(t, client)
	editor.replace = "job \"web\" {\n  type = \"batch\"\n}"

	m, cmd := m.update(key('e'))

	return follow(m, cmd, 6)
}

func TestPlan_ComesBeforeTheSubmit(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), spec: nomad.JobSource{Source: "job \"web\" {}"}, plan: changedEnv()}
	m := planned(t, client)

	// Saving the file asks the cluster what it would do, and does nothing
	// yet: a changed file can restart every allocation of the job.
	r.Equal(screenPlan, m.screen.kind)
	r.Equal("job \"web\" {\n  type = \"batch\"\n}", client.plannedSource)
	r.Equal("production", client.askedNamespace)
	r.Zero(client.submitted)

	out := plain(m.render())
	r.Contains(out, "Plan (Job: web)")
	r.Contains(out, "2 create/destroy update")
	r.Contains(out, "Submit")
}

func TestPlan_SubmitsAtItsIndex(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), spec: nomad.JobSource{Source: "job \"web\" {}"}, plan: changedEnv()}
	m := planned(t, client)

	m, cmd := m.update(key('y'))
	m = drain(m, cmd)

	// What was planned is what is sent, at the index it was planned at.
	r.Equal(1, client.submitted)
	r.Equal("job \"web\" {\n  type = \"batch\"\n}", client.submittedSource)
	r.Equal(uint64(42), client.submittedIndex)

	// The plan is spent: the screen goes back to the job list.
	r.Equal(screenJobs, m.screen.kind)
	r.Contains(plain(m.render()), "Job web submitted")
}

func TestPlan_WhenTheJobChangedSinceThePlan(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		jobs:      twoJobs(),
		spec:      nomad.JobSource{Source: "job \"web\" {}"},
		plan:      changedEnv(),
		actionErr: nomad.ErrJobChanged,
	}
	m := planned(t, client)

	m, cmd := m.update(key('y'))
	m = drain(m, cmd)

	// The plan on the screen is not what submitting would do any more.
	// The file is still there to be planned again.
	r.Equal(screenPlan, m.screen.kind)
	r.Contains(plain(m.render()), "web changed since this plan: r plans it again")
}

func TestPlan_PlansTheSameFileAgain(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), spec: nomad.JobSource{Source: "job \"web\" {}"}, plan: changedEnv()}
	m := planned(t, client)

	again := changedEnv()
	again.Groups = []nomad.PlanGroup{{Name: "web", InPlace: 3}}
	again.Index = 43
	client.plan = again

	m, cmd := m.update(key('r'))
	m = drain(m, cmd)

	r.Equal(2, client.planCalls)
	r.Equal("job \"web\" {\n  type = \"batch\"\n}", client.plannedSource)
	r.Equal(screenPlan, m.screen.kind)
	r.Contains(strings.Join(m.text.lines, "\n"), "3 in-place update")

	// Submitted after a new plan, it goes at the new index.
	m, cmd = m.update(key('y'))
	drain(m, cmd)
	r.Equal(uint64(43), client.submittedIndex)
}

func TestPlan_EscapeLeavesTheJobAsItIs(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), spec: nomad.JobSource{Source: "job \"web\" {}"}, plan: changedEnv()}
	m := planned(t, client)

	m, _ = m.update(escape())

	r.Equal(screenJobs, m.screen.kind)
	r.Zero(client.submitted)
}
