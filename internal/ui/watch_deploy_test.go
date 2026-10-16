package ui

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// versionSeven is the version web runs.
func versionSeven() []nomad.JobVersion {
	return []nomad.JobVersion{{Version: 7, Current: true, JobModifyIndex: 70}, {Version: 6, JobModifyIndex: 60}}
}

// completeEval is the newest evaluation of web, which placed everything.
func completeEval() *nomad.EvaluationDetail {
	return &nomad.EvaluationDetail{Evaluation: nomad.Evaluation{
		ID: "e1a2b3c4-0000-0000-0000-000000000000", JobID: "web", Namespace: "production", Status: "complete",
	}}
}

// pendingEval is the newest evaluation of web, still at it.
func pendingEval() *nomad.EvaluationDetail {
	eval := completeEval()
	eval.Status = "pending"

	return eval
}

// onWatch is the watch of web, opened with w from the list of jobs.
func onWatch(t *testing.T, client *fakeClient, jobs ...nomad.Job) Model {
	t.Helper()

	if len(jobs) == 0 {
		jobs = []nomad.Job{twoJobs()[0]}
	}

	client.jobs, client.versions = jobs, versionSeven()

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(jobs))

	m, cmd := m.update(key('w'))

	return drain(m, cmd)
}

func TestWatch_AJobWithARunningDeployment(t *testing.T) {
	r := require.New(t)

	running := waitingCanary()
	client := &fakeClient{jobDeployments: []nomad.DeploymentDetail{running}, newest: completeEval(), deploymentAllocs: canaryAllocs()}

	m := onWatch(t, client)
	out := plain(m.render())

	r.Contains(out, "Deploy (Job: web, Version: 7) [2]")
	r.Contains(out, "Evaluation e1a2b3c4 complete")
	r.Contains(out, "Deployment running: Deployment is running but requires manual promotion")

	// The progress of each group, and the allocations of the deployment.
	r.Contains(out, "Unhealthy")
	r.Contains(out, "Canaries")
	r.Contains(fileRow(t, m, 0), "checking")

	// The deployment screen's keys, offered as there: the canary of web
	// waits under the cursor.
	r.True(offers(m, "p"))
	r.True(offers(m, "ctrl-p"))
	r.True(offers(m, "enter"))
	r.Equal("5d1a2b3c-0000-0000-0000-000000000000", client.deploymentID)
}

func TestWatch_PromoteTheCanaryFromTheWatch(t *testing.T) {
	r := require.New(t)

	running := waitingCanary()
	client := &fakeClient{jobDeployments: []nomad.DeploymentDetail{running}, deploymentAllocs: canaryAllocs()}

	m := onWatch(t, client)

	m, _ = m.update(key('p'))
	r.Contains(plain(m.render()), "Really promote the canaries of group web of web?")

	confirmed(t, m)

	r.Equal([]string{"web"}, client.promotedGroups)
	r.Equal("production", client.askedNamespace)
}

func TestWatch_NoDeploymentYet(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{newest: pendingEval(), allocs: []nomad.Alloc{
		{ID: "11111111-0000-0000-0000-000000000000", JobID: "web", Namespace: "production", TaskGroup: "web", Status: "pending", JobVersion: 7},
		{ID: "22222222-0000-0000-0000-000000000000", JobID: "web", Namespace: "production", TaskGroup: "web", Status: "running", JobVersion: 6},
	}}

	// The newest deployment is the one of the version before.
	older := waitingCanary()
	older.JobVersion = 6
	client.jobDeployments = []nomad.DeploymentDetail{older}

	m := onWatch(t, client)
	out := plain(m.render())

	r.Contains(out, "Waiting for the deployment of version 7.")
	r.NotContains(out, "Unhealthy")

	// The allocations of the job at that version, and the keys of an
	// allocation, none of a deployment.
	r.Contains(out, "[1]")
	r.Contains(fileRow(t, m, 0), "11111111")
	r.True(offers(m, "enter"))
	r.False(offers(m, "p"))
	r.False(offers(m, "ctrl-p"))
	r.False(offers(m, "f"))

	// The next read finds the deployment of version 7.
	running := waitingCanary()
	client.jobDeployments, client.deploymentAllocs = []nomad.DeploymentDetail{running}, canaryAllocs()

	m = drain(m, m.fetch())

	out = plain(m.render())
	r.NotContains(out, "Waiting for the deployment")
	r.Contains(out, "Deployment running:")

	// The allocation under the cursor is gone from the rows: the cursor
	// goes back on the first one.
	m, _ = m.update(key('k'))
	r.True(offers(m, "p"))
}

func TestWatch_ADeploymentThatEnded(t *testing.T) {
	for name, tc := range map[string]struct{ status, description, line string }{
		"successful": {"successful", "Deployment completed successfully", "Deployment successful."},
		"failed":     {"failed", "Failed due to progress deadline", "Deployment failed: Failed due to progress deadline."},
		"cancelled":  {"cancelled", "Cancelled because job is stopped", "Deployment cancelled: Cancelled because job is stopped."},
	} {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			done := waitingCanary()
			done.Status, done.StatusDescription = tc.status, tc.description

			client := &fakeClient{jobDeployments: []nomad.DeploymentDetail{done}, deploymentAllocs: canaryAllocs()}

			m := onWatch(t, client)

			r.Contains(plain(m.render()), tc.line)

			// Nothing is left to promote, fail, pause or mark; the allocations
			// keep their keys.
			for _, press := range []string{"p", "ctrl-p", "f", "ctrl-s", "h", "u"} {
				r.False(offers(m, press), press)
			}

			r.True(offers(m, "enter"))
		})
	}
}

func TestWatch_ABatchJobHasNoDeployment(t *testing.T) {
	for _, jobType := range []string{"batch", "sysbatch"} {
		t.Run(jobType, func(t *testing.T) { aBatchJobHasNoDeployment(t, jobType) })
	}
}

func aBatchJobHasNoDeployment(t *testing.T, jobType string) {
	r := require.New(t)

	job := nomad.Job{ID: "web", Name: "web", Namespace: "production", Type: jobType, Status: "running"}
	client := &fakeClient{allocs: []nomad.Alloc{
		{ID: "11111111-0000-0000-0000-000000000000", JobID: "web", Namespace: "production", TaskGroup: "web", Status: "running", JobVersion: 7},
	}}

	m := onWatch(t, client, job)
	out := plain(m.render())

	r.Contains(out, "web has no deployment for version 7: its allocations are below.")
	r.NotContains(out, "Waiting for the deployment")
	r.Contains(fileRow(t, m, 0), "11111111")
}

func TestWatch_AVersionThatMadeNoDeployment(t *testing.T) {
	// A stopped service job, or an update with max_parallel = 0: the newest
	// evaluation completed after the version was submitted and no deployment
	// came of it.
	line := "web has no deployment for version 7: its allocations are below."

	for name, tc := range map[string]struct {
		status string
		index  uint64
		waits  bool
	}{
		"completed after the version": {"complete", 70, false},
		// A submit that changes nothing makes an evaluation with no job
		// index; it is still made after the version.
		"after a submit of no change": {"complete", 75, false},
		"completed above it":          {"complete", 71, false},
		"older than the version":      {"complete", 69, true},
		"not complete":                {"pending", 70, true},
	} {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			eval := completeEval()
			eval.Status, eval.CreateIndex = tc.status, tc.index

			m := onWatch(t, &fakeClient{newest: eval})
			out := plain(m.render())

			if tc.waits {
				r.Contains(out, "Waiting for the deployment of version 7.")
				r.NotContains(out, line)

				return
			}

			r.Contains(out, line)
			r.NotContains(out, "Waiting for the deployment")
		})
	}
}

func TestWatch_ABlockedEvaluationSaysWhy(t *testing.T) {
	r := require.New(t)

	blocked := completeEval()
	blocked.Status = "blocked"
	blocked.Failures = []nomad.PlacementFailure{{Group: "web", Unplaced: 2, NodesEvaluated: 3, NodesExhausted: 3}}

	client := &fakeClient{newest: blocked}

	m := onWatch(t, client)
	out := plain(m.render())

	r.Contains(out, "Evaluation e1a2b3c4 blocked")
	r.Contains(out, `Task group "web" failed to place 2 allocations: Resources exhausted on 3 nodes`)
}

func TestWatch_EveryReadIsInTheNamespaceOfTheJob(t *testing.T) {
	r := require.New(t)

	running := waitingCanary()
	client := &fakeClient{jobDeployments: []nomad.DeploymentDetail{running}, newest: completeEval(), deploymentAllocs: canaryAllocs()}
	client.jobs, client.versions = twoJobs()[:1], versionSeven()

	m := newTestModel(client)
	m, _ = m.update(key('0'))
	r.Equal(nomad.AllNamespaces, m.namespace)

	m, _ = m.update(jobsMsg(client.jobs))
	m, cmd := m.update(key('w'))
	m = drain(m, cmd)

	r.Contains(plain(m.render()), "Deploy (Job: web, Version: 7)")

	r.Equal([]string{"production"}, client.lists["versions"])

	for _, read := range []string{"job deployments", "newest evaluation"} {
		r.Equal([]string{"production/web"}, client.lists[read], read)
	}

	r.Equal("production", client.deploymentNamespace)
	r.Equal("production", namespaceOf(m.screen.page, m.env()))
}

func TestWatch_AJobWithoutADeploymentReadsItsAllocationsInItsNamespace(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{newest: pendingEval()}
	client.jobs, client.versions = twoJobs()[:1], versionSeven()

	m := newTestModel(client)
	m, _ = m.update(key('0'))
	m, _ = m.update(jobsMsg(client.jobs))
	m, cmd := m.update(key('w'))
	m = drain(m, cmd)

	r.Contains(plain(m.render()), "Waiting for the deployment of version 7.")
	r.Equal([]string{"production/web"}, client.lists["allocations"])
}

func TestWatch_KeepsTheVersionItOpenedOn(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}

	m := onWatch(t, client)
	r.Contains(plain(m.render()), "Version: 7")

	// Version 8 is submitted while the watch is open.
	newer := waitingCanary()
	newer.JobVersion = 8
	client.versions, client.jobDeployments = []nomad.JobVersion{{Version: 8, Current: true}, {Version: 7}}, []nomad.DeploymentDetail{newer}

	m = drain(m, m.fetch())

	r.Contains(plain(m.render()), "Version: 7")
	r.NotContains(plain(m.render()), "Version: 8")
}

func TestWatch_DropsAnAnswerOfAnotherVersion(t *testing.T) {
	r := require.New(t)

	m := onWatch(t, &fakeClient{})
	r.Contains(plain(m.render()), "Version: 7")

	// A read asked before the watch knew its version can answer after it,
	// with the version a newer submit made.
	m, _ = m.update(deployWatchMsg{namespace: "production", jobID: "web", version: 8})

	r.Contains(plain(m.render()), "Version: 7")
	r.NotContains(plain(m.render()), "Version: 8")
}

func TestWatch_ANewerVersionCancelsTheDeploymentWatched(t *testing.T) {
	r := require.New(t)

	running := waitingCanary()
	client := &fakeClient{jobDeployments: []nomad.DeploymentDetail{running}, deploymentAllocs: canaryAllocs()}

	m := onWatch(t, client)
	r.Contains(plain(m.render()), "Deployment running:")

	// Version 8 is submitted: its deployment is the newest, and Nomad
	// cancelled the one of version 7.
	cancelled := waitingCanary()
	cancelled.Status, cancelled.StatusDescription = "cancelled", "Cancelled due to newer version of job"

	newer := waitingCanary()
	newer.ID, newer.JobVersion = "8e2f0000-0000-0000-0000-000000000000", 8

	client.jobDeployments = []nomad.DeploymentDetail{newer, cancelled}

	m = drain(m, m.fetch())
	out := plain(m.render())

	r.Contains(out, "Version: 7")
	r.Contains(out, "Deployment cancelled: Cancelled due to newer version of job.")
	r.NotContains(out, "Waiting for the deployment")
	r.Equal("5d1a2b3c-0000-0000-0000-000000000000", client.deploymentID)
}

func TestWatch_BeforeItsFirstReadItHasNoPanel(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	client.jobs, client.versions = twoJobs()[:1], versionSeven()

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(client.jobs))
	m, _ = m.update(key('w'))

	out := plain(m.render())
	r.Contains(out, "Deploy (Job: web) [0]")
	r.NotContains(out, "Waiting")
}

func TestWatch_AJobWithNoVersionsIsAnError(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}

	_, err := readDeploy(context.Background(), client, deployWatchPage{namespace: "production", jobID: "web"})

	r.EqualError(err, "web has no versions")
	r.Empty(client.lists["job deployments"])
	r.Empty(client.lists["newest evaluation"])
	r.Empty(client.lists["allocations"])
}

func TestWatch_WatchesTheJobItsEvaluationsItsDeploymentAndItsAllocations(t *testing.T) {
	r := require.New(t)

	m := onWatch(t, &fakeClient{})

	r.ElementsMatch([]string{nomad.TopicJob, nomad.TopicEvaluation, nomad.TopicDeployment, nomad.TopicAllocation}, m.screen.page.topics())
}

func TestWatch_NotOfferedOnAJobThatLaunchesOthers(t *testing.T) {
	r := require.New(t)

	jobs := []nomad.Job{
		{ID: "cron", Name: "cron", Namespace: "production", Type: "batch", Status: "running", Periodic: true},
		{ID: "mail", Name: "mail", Namespace: "production", Type: "batch", Status: "running", Parameterized: true},
		twoJobs()[0],
	}

	m := newTestModel(&fakeClient{jobs: jobs})
	m, _ = m.update(jobsMsg(jobs))

	r.False(offers(m, "w"))

	m, _ = m.update(key('j'))
	r.False(offers(m, "w"))

	m, _ = m.update(key('j'))
	r.True(offers(m, "w"))
}

func TestWatch_IsOfferedInReadOnlyMode(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{jobs: twoJobs()[:1], versions: versionSeven()})
	m.opts.ReadOnly = true
	m, _ = m.update(jobsMsg(twoJobs()[:1]))

	r.True(offers(m, "w"))

	m, cmd := m.update(key('w'))
	m = drain(m, cmd)

	r.Contains(plain(m.render()), "Deploy (Job: web, Version: 7)")
}

// schedulerRacing is a cluster whose scheduler writes the deployment while
// the evaluation is read: the deployment is there for any read of it after
// the evaluation was read.
type schedulerRacing struct {
	*fakeClient

	evalRead bool
}

func (c *schedulerRacing) NewestEvaluation(ctx context.Context, namespace, jobID string) (*nomad.EvaluationDetail, error) {
	c.evalRead = true

	return c.fakeClient.NewestEvaluation(ctx, namespace, jobID)
}

func (c *schedulerRacing) JobDeployments(ctx context.Context, namespace, jobID string) ([]nomad.DeploymentDetail, error) {
	if !c.evalRead {
		return nil, nil
	}

	return c.fakeClient.JobDeployments(ctx, namespace, jobID)
}

func TestWatch_ReadsTheEvaluationBeforeTheDeployments(t *testing.T) {
	r := require.New(t)

	eval := completeEval()
	eval.CreateIndex = 70

	jobs := []nomad.Job{twoJobs()[0]}
	client := &schedulerRacing{fakeClient: &fakeClient{
		jobs: jobs, versions: versionSeven(), newest: eval,
		jobDeployments: []nomad.DeploymentDetail{waitingCanary()}, deploymentAllocs: canaryAllocs(),
	}}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(jobs))
	m, cmd := m.update(key('w'))
	out := plain(drain(m, cmd).render())

	r.Contains(out, "Deployment running:")
	r.NotContains(out, "has no deployment")
}

func TestWatch_AServiceJobWithNoEvaluationHasNoDeploymentToWaitFor(t *testing.T) {
	r := require.New(t)

	// A stopped job whose evaluation and deployment were collected.
	out := plain(onWatch(t, &fakeClient{}).render())

	r.Contains(out, "web has no deployment for version 7: its allocations are below.")
	r.NotContains(out, "Waiting for the deployment")
}

func TestWatch_LogsAreOfTheAllocationsOfTheRows(t *testing.T) {
	r := require.New(t)

	alloc := func(id string, version uint64, task string) nomad.Alloc {
		return nomad.Alloc{ID: id, JobID: "web", Namespace: "production", TaskGroup: "web", Status: statusRunning,
			JobVersion: version, Tasks: []nomad.Task{{Name: task}}}
	}

	// Version 7 has no deployment yet: its evaluation is still pending.
	client := &fakeClient{newest: pendingEval(), allocs: []nomad.Alloc{
		alloc("11111111-0000-0000-0000-000000000000", 7, "new"),
		alloc("22222222-0000-0000-0000-000000000000", 6, "old"),
	}, logsByAlloc: map[string]*nomad.LogStream{"11111111-0000-0000-0000-000000000000": writing()}}

	m := onWatch(t, client)
	r.Contains(plain(m.render()), "Waiting for the deployment of version 7.")

	m, cmd := m.update(key('l'))
	m = playOut(m, cmd)

	r.IsType(jobLogsPage{}, m.screen.page)
	r.Equal("new", m.screen.page.(jobLogsPage).task)
}
