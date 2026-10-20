package ui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func onJobMeta(t *testing.T, client *fakeClient) Model {
	t.Helper()

	jobs := twoJobs()
	jobs[0].Namespace = "staging"
	client.jobs = jobs

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(jobs))

	m, cmd := m.update(key('m'))

	return drain(m, cmd)
}

func TestJobMeta_ShowsTheKeysAndValuesOfTheJob(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobMeta: map[string]string{"version": "3", "owner": "team-web"}}
	m := onJobMeta(t, client)

	r.IsType(jobMetaPage{}, m.screen.page)

	out := plain(m.render())
	r.Contains(out, "Meta (Job: web) [2]")
	r.Less(strings.Index(out, "owner"), strings.Index(out, "version"))
	r.Contains(out, "team-web")
	r.NotContains(out, "has no meta")

	// The job's own namespace, not the one the session has open.
	r.Equal("staging", client.askedNamespace)
	r.Equal("web", client.askedID)
	r.Equal("staging", client.watchedNamespace)
	r.Equal([]string{nomad.TopicJob}, client.watchedTopics)
}

func TestJobMeta_CopiesTheValueUnderTheCursor(t *testing.T) {
	r := require.New(t)

	m := onJobMeta(t, &fakeClient{jobMeta: map[string]string{"owner": "team-web"}})

	_, cmd := m.update(key('c'))
	r.Contains(clipboardOf(cmd), "team-web")
}

func TestJobMeta_SaysWhenTheJobHasNone(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs()}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, cmd := m.update(key('m'))
	r.NotContains(plain(m.render()), "has no meta")

	m = drain(m, cmd)
	r.Contains(plain(m.render()), "The job web has no meta.")
}

func TestJobMeta_DropsTheAnswerOfAnotherJob(t *testing.T) {
	r := require.New(t)

	m := onJobMeta(t, &fakeClient{jobMeta: map[string]string{"owner": "team-web"}})

	m, _ = m.update(jobMetaMsg{namespace: "staging", jobID: "cron", meta: map[string]string{"owner": "somebody else"}})
	r.NotContains(plain(m.render()), "somebody else")

	m, _ = m.update(jobMetaMsg{namespace: "production", jobID: "web", meta: map[string]string{"owner": "another namespace"}})
	r.NotContains(plain(m.render()), "another namespace")
}

func TestJobMeta_EscGoesBackToTheJobs(t *testing.T) {
	r := require.New(t)

	m := onJobMeta(t, &fakeClient{})

	m, _ = m.update(escape())
	r.IsType(jobsPage{}, m.screen.page)
}
