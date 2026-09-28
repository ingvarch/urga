package ui

import (
	"testing"

	"github.com/stretchr/testify/require"
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
