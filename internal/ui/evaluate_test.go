package ui

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestJobs_EvaluateAJob(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs()}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	r.Contains(m.hints(), hint{Key: "<ctrl-e>", Description: "Evaluate"})

	m, _ = m.update(ctrlKey('e'))
	r.Contains(plain(m.render()), "Really evaluate the job web?")

	m, cmd := m.update(key('y'))
	m = playOut(m, cmd)

	r.Equal([]string{"EvaluateJob"}, client.writes)
	r.Equal([]string{"web"}, client.evaluated)
	r.Equal("production", client.askedNamespace)
	r.Contains(plain(m.render()), "Evaluated the job web.")
}

func TestJobs_EvaluateTheMarkedJobs(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs()}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(ctrlKey('a'))

	m, _ = m.update(ctrlKey('e'))
	r.Contains(plain(m.render()), "Really evaluate 2 jobs?")

	m, cmd := m.update(key('y'))
	playOut(m, cmd)

	r.ElementsMatch([]string{"web", "cron"}, client.evaluated)
}

func TestJobs_AJobThatLaunchesIsNotEvaluated(t *testing.T) {
	r := require.New(t)

	// The cluster evaluates its launches, not the job that makes them.
	m := onBackup(t, &fakeClient{jobs: periodicJobs()})
	r.NotContains(m.hints(), hint{Key: "<ctrl-e>", Description: "Evaluate"})
}

func TestJobs_RescheduleTheFailedAllocationsOfAJob(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs()}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	r.Contains(m.hints(), hint{Key: "<ctrl-r>", Description: "Reschedule Failed"})

	m, _ = m.update(ctrlKey('r'))
	r.Contains(plain(m.render()), "Really evaluate the job web and place its failed allocations again now?")

	m, cmd := m.update(key('y'))
	m = playOut(m, cmd)

	r.Equal([]string{"RescheduleFailed"}, client.writes)
	r.Equal([]string{"production/web"}, client.rescheduled)
	r.Contains(plain(m.render()), "Rescheduled the failed allocations of the job web.")
}

func TestJobs_RescheduleTheFailedAllocationsOfTheMarkedJobs(t *testing.T) {
	r := require.New(t)

	jobs := twoJobs()
	jobs[1].Namespace = "staging"

	client := &fakeClient{jobs: jobs}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(jobs))
	m, _ = m.update(ctrlKey('a'))

	m, _ = m.update(ctrlKey('r'))
	r.Contains(plain(m.render()), "Really evaluate 2 jobs and place their failed allocations again now?")

	m, cmd := m.update(key('y'))
	playOut(m, cmd)

	r.ElementsMatch([]string{"production/web", "staging/cron"}, client.rescheduled)
}

func TestJobs_ASystemJobHasNoFailedAllocationsToReschedule(t *testing.T) {
	r := require.New(t)

	// Nomad reschedules no allocation of a system or sysbatch job: the key
	// would only evaluate it, as ctrl+e does.
	for _, kind := range []string{"system", "sysbatch"} {
		jobs := []nomad.Job{{ID: "agent", Name: "agent", Namespace: "production", Type: kind, Status: "running"}}

		m := newTestModel(&fakeClient{jobs: jobs})
		m, _ = m.update(jobsMsg(jobs))

		r.NotContains(m.hints(), hint{Key: "<ctrl-r>", Description: "Reschedule Failed"}, kind)
		r.Contains(m.hints(), hint{Key: "<ctrl-e>", Description: "Evaluate"}, kind)
	}
}

func TestJobs_AJobThatLaunchesHasNoFailedAllocationsToReschedule(t *testing.T) {
	r := require.New(t)

	m := onBackup(t, &fakeClient{jobs: periodicJobs()})
	r.NotContains(m.hints(), hint{Key: "<ctrl-r>", Description: "Reschedule Failed"})
}
