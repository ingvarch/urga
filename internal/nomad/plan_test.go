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
		"api": {"InPlaceUpdate": 1, "Place": 1, "Ignore": 2}
	}},
	"FailedTGAllocs": {"web": {"NodesEvaluated": 1, "NodesExhausted": 1, "DimensionExhausted": {"memory": 1}}},
	"Warnings": "1 warning:\n\n* Group \"web\" has warnings"
}`

func TestPlanJob(t *testing.T) {
	r := require.New(t)

	client, asked := jobServer(t, map[string]string{"/v1/job/web/plan": webPlan})

	plan, err := client.PlanJob(context.Background(), "production", `{"ID": "web", "Name": "web"}`, nomad.JobVariables{})
	r.NoError(err)

	// The plan is requested in the namespace of the job, with the diff:
	// showing what would change is what a plan is for.
	r.Len(*asked, 1)
	r.Equal("/v1/job/web/plan", (*asked)[0].path)
	r.Equal("production", (*asked)[0].namespace)
	r.Equal(true, (*asked)[0].body["Diff"])

	// The index the job had when it was planned: submitting at it fails if
	// the job changed in between.
	r.Equal(uint64(42), plan.Index)

	r.Contains(plan.Diff, nomad.DiffLine{Kind: nomad.DiffDeleted, Indent: 3, Text: `V = "2"`})
	r.Contains(plan.Diff, nomad.DiffLine{Kind: nomad.DiffAdded, Indent: 3, Text: `V = "3"`})

	// What the scheduler would do to each group, in the order of their names.
	r.Equal([]nomad.PlanGroup{
		{Name: "api", Place: 1, InPlace: 1, Ignore: 2},
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

// webVersions are the versions of web, newest first: 5 runs, 4 was before it.
const webVersions = `{"Versions": [
	{"ID": "web", "Name": "web", "Version": 5, "Priority": 70},
	{"ID": "web", "Name": "web", "Version": 4, "Priority": 50},
	{"ID": "web", "Name": "web", "Version": 3, "Priority": 30}
], "Diffs": null}`

// plannedJob is the job a plan request asked about.
func plannedJob(t *testing.T, asked []sent) map[string]any {
	t.Helper()

	for _, req := range asked {
		if req.path == "/v1/job/web/plan" {
			job, ok := req.body["Job"].(map[string]any)
			require.True(t, ok)

			return job
		}
	}

	t.Fatal("no plan was asked for")

	return nil
}

func TestPlanRevert_ToTheVersionBefore(t *testing.T) {
	r := require.New(t)

	client, asked := jobServer(t, map[string]string{"/v1/job/web/versions": webVersions, "/v1/job/web/plan": webPlan})

	plan, err := client.PlanRevert(context.Background(), "production", "web", nil)
	r.NoError(err)

	r.Equal("/v1/job/web/versions", (*asked)[0].path)
	r.Equal("production", (*asked)[0].namespace)

	// Going back is submitting the version before the one that runs; its
	// plan says what that would change.
	r.Equal(float64(50), plannedJob(t, *asked)["Priority"])
	r.Equal(uint64(4), plan.To)
	r.Equal(uint64(5), plan.Version)
	r.Equal(uint64(42), plan.Index)
}

func TestPlanRevert_ToTheVersionGiven(t *testing.T) {
	r := require.New(t)

	client, asked := jobServer(t, map[string]string{"/v1/job/web/versions": webVersions, "/v1/job/web/plan": webPlan})

	to := uint64(3)
	plan, err := client.PlanRevert(context.Background(), "production", "web", &to)
	r.NoError(err)

	r.Equal(float64(30), plannedJob(t, *asked)["Priority"])
	r.Equal(uint64(3), plan.To)
}

func TestPlanRevert_AtTheFirstVersion(t *testing.T) {
	r := require.New(t)

	client, _ := jobServer(t, map[string]string{
		"/v1/job/web/versions": `{"Versions": [{"ID": "web", "Version": 0}]}`,
	})

	// There is no version before the first one, and an error that names this
	// is clearer than a cluster error.
	_, err := client.PlanRevert(context.Background(), "production", "web", nil)
	r.ErrorContains(err, "no earlier version")
}

func TestPlanRevert_ToAVersionThatIsGone(t *testing.T) {
	r := require.New(t)

	client, _ := jobServer(t, map[string]string{"/v1/job/web/versions": webVersions})

	to := uint64(1)
	_, err := client.PlanRevert(context.Background(), "production", "web", &to)
	r.ErrorContains(err, "no version 1")
}

func TestRevertJobTo_FromTheVersionItWasPlannedAt(t *testing.T) {
	r := require.New(t)

	client, asked := jobServer(t, map[string]string{"/v1/job/web/revert": `{"EvalID": "eval-1"}`})

	r.NoError(client.RevertJobTo(context.Background(), "production", "web", 4, 5))

	// The version goes in the body, with the one the job had when the revert
	// was planned: a job that changed since is not reverted.
	body := (*asked)[0].body
	r.Equal("web", body["JobID"])
	r.Equal(float64(4), body["JobVersion"])
	r.Equal(float64(5), body["EnforcePriorVersion"])
	r.Equal("production", (*asked)[0].namespace)
}

func TestRevertJobTo_WhenTheJobChangedSinceItsPlan(t *testing.T) {
	r := require.New(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// What the cluster answers when the job is not at that version any
		// more.
		http.Error(w, "Current job has version 6; enforcing version 5", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	r.NoError(err)

	r.ErrorIs(client.RevertJobTo(context.Background(), "production", "web", 4, 5), nomad.ErrJobChanged)
}
