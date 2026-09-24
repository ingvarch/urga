package nomad_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// webPlan is what the cluster answers for a change of an environment variable
// of web: the task is replaced, one group is short of memory.
const webPlan = `{
	"JobModifyIndex": 42,
	"Diff": {"Type": "Edited", "ID": "web", "TaskGroups": [
		{"Type": "Edited", "Name": "web", "Tasks": [
			{"Type": "Edited", "Name": "server", "Fields": [
				{"Type": "Edited", "Name": "Env[V]", "Old": "2", "New": "3"}
			]}
		]}
	]},
	"Annotations": {"DesiredTGUpdates": {
		"web": {"DestructiveUpdate": 2, "Canary": 1},
		"api": {"InPlaceUpdate": 1, "Place": 1}
	}},
	"FailedTGAllocs": {"web": {"NodesEvaluated": 1, "NodesExhausted": 1, "DimensionExhausted": {"memory": 1}}},
	"Warnings": "1 warning:\n\n* Group \"web\" has warnings"
}`

func TestPlanJob(t *testing.T) {
	r := require.New(t)

	client, asked := jobServer(t, map[string]string{"/v1/job/web/plan": webPlan})

	plan, err := client.PlanJob(context.Background(), "production", `{"ID": "web", "Name": "web"}`, nomad.JobVariables{})
	r.NoError(err)

	// A plan is asked of the cluster in the namespace of the job, with the
	// diff: what would change is what the plan is read for.
	r.Len(*asked, 1)
	r.Equal("/v1/job/web/plan", (*asked)[0].path)
	r.Equal("production", (*asked)[0].namespace)
	r.Equal(true, (*asked)[0].body["Diff"])

	// The index the job had when it was planned: submitting at it fails if
	// the job changed in between.
	r.Equal(uint64(42), plan.Index)

	r.Contains(plan.Diff, "~ Env[V]: 2 -> 3")

	// What the scheduler would do to each group, in the order of their names.
	r.Equal([]nomad.PlanGroup{
		{Name: "api", Place: 1, InPlace: 1},
		{Name: "web", Destructive: 2, Canary: 1},
	}, plan.Groups)

	r.Len(plan.Failures, 1)
	r.Equal(map[string]int{"memory": 1}, plan.Failures[0].DimensionExhausted)

	r.Contains(plan.Warnings, "has warnings")
}

func TestPlanJob_HCLIsReadByTheClusterFirst(t *testing.T) {
	r := require.New(t)

	client, asked := jobServer(t, map[string]string{
		"/v1/jobs/parse":   `{"ID": "web", "Name": "web"}`,
		"/v1/job/web/plan": webPlan,
	})

	vars := nomad.JobVariables{Flags: map[string]string{"image": "nginx:1.27"}}

	_, err := client.PlanJob(context.Background(), "production", "job \"web\" {}", vars)
	r.NoError(err)

	// The file is planned the way it would be submitted: with its variables.
	r.Len(*asked, 2)
	r.Equal("/v1/jobs/parse", (*asked)[0].path)
	r.Contains((*asked)[0].body["Variables"], `image = "nginx:1.27"`)
	r.Equal("/v1/job/web/plan", (*asked)[1].path)
}

func TestSubmitJob_AtTheIndexOfItsPlan(t *testing.T) {
	r := require.New(t)

	client, asked := jobServer(t, map[string]string{"/v1/jobs": `{"EvalID": "eval-1"}`})

	r.NoError(client.SubmitJob(context.Background(), "production", `{"ID": "web"}`, nomad.JobVariables{}, 42))

	// What was planned is what is submitted, or nothing: a job that changed
	// since the plan is refused.
	body := (*asked)[0].body
	r.Equal(true, body["EnforceIndex"])
	r.Equal(float64(42), body["JobModifyIndex"])
}

func TestSubmitJob_WhenTheJobChangedSinceItsPlan(t *testing.T) {
	r := require.New(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// What the cluster answers when the index is not the job's any more.
		http.Error(w, "Enforcing job modify index 42: job exists with conflicting job modify index: 43",
			http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	r.NoError(err)

	err = client.SubmitJob(context.Background(), "production", `{"ID": "web"}`, nomad.JobVariables{}, 42)
	r.ErrorIs(err, nomad.ErrJobChanged)
}
