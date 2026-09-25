package ui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func threeVersions() []nomad.JobVersion {
	return []nomad.JobVersion{
		{Version: 3, Current: true, Stable: true, Tag: "golden", Submitted: time.Now().Add(-time.Hour), Changes: 3},
		{Version: 2, Submitted: time.Now().Add(-25 * time.Hour), Changes: 1},
		{Version: 1, Stable: true, Submitted: time.Now().Add(-49 * time.Hour)},
	}
}

// onVersions opens the versions of the job under the cursor.
func onVersions(t *testing.T) (Model, *fakeClient) {
	t.Helper()

	client := &fakeClient{jobs: twoJobs(), versions: threeVersions(), diff: []nomad.DiffLine{
		{Kind: nomad.DiffDeleted, Text: "priority = 50"},
		{Kind: nomad.DiffAdded, Text: "priority = 70"},
	}}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, cmd := m.update(key('v'))

	return drain(m, cmd), client
}

func TestVersions_ListWhatTheJobWas(t *testing.T) {
	r := require.New(t)

	m, client := onVersions(t)

	r.IsType(versionsPage{}, m.screen.page)
	r.Equal("web", client.askedJobID)

	out := plain(m.render())
	r.Contains(out, "Versions (Job: web) [3]")

	// What each version is: its number, whether it runs, whether the
	// cluster marked it stable, its tag and how much it changed.
	r.Contains(out, "golden")
	r.Contains(out, "current")
	r.Contains(out, "stable")
	r.Contains(out, "3 fields")
}

func TestVersions_OpenWhatAVersionChanged(t *testing.T) {
	r := require.New(t)

	m, client := onVersions(t)

	m, cmd := m.update(enter())
	m = drain(m, cmd)

	r.IsType(describePage{}, m.screen.page)
	r.Equal(uint64(3), client.askedVersion)
	// As a unified diff reads, in colour.
	out := m.render()
	r.Contains(plain(out), "- priority = 50")
	r.Contains(plain(out), "+ priority = 70")
	r.Contains(out, opening(styleAdded)+"+ priority = 70")
}

func TestVersions_RevertToTheOneUnderTheCursor(t *testing.T) {
	r := require.New(t)

	m, client := onVersions(t)

	client.plan = nomad.Plan{To: 2, Version: 3}

	// Down to version 2, which is not the one that runs.
	m, _ = m.update(key('j'))
	m, cmd := m.update(key('u'))
	m = drain(m, cmd)

	r.NotNil(client.plannedTo)
	r.Equal(uint64(2), *client.plannedTo)
	r.Contains(plain(m.render()), "Revert (Job: web, Version: 2)")

	m, cmd = m.update(key('y'))
	drain(m, cmd)

	r.Equal(uint64(2), client.revertedTo)
	r.Equal("web", client.askedJobID)
}

func TestVersions_TheJobScreenStillRevertsToThePrevious(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs()}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, cmd := m.update(key('u'))
	drain(m, cmd)

	// The key that was there keeps its meaning on the list of jobs: the
	// version before the one that runs.
	r.Nil(client.plannedTo)
	r.Equal("web", client.askedJobID)
}

func TestVersions_OfAnotherJobAreNotShownHere(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), versions: threeVersions()}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, cmd := m.update(key('v'))
	m = drain(m, cmd)

	m, _ = m.update(escape())
	m, _ = m.update(key('j'))
	m, _ = m.update(key('v'))

	// Until the cluster answers for this job, the screen holds nothing:
	// version numbers belong to one job, and the ones of another must not
	// show under its name.
	r.Contains(plain(m.render()), "Versions (Job: cron) [0]")

	// And a late answer for the job that was left is dropped.
	m, _ = m.update(versionsMsg{jobID: "web", versions: threeVersions()})
	r.Contains(plain(m.render()), "[0]")
}

func TestVersions_TheFirstVersionIsNotAnError(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), versions: threeVersions(), diffErr: nomad.ErrNoDiff}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))
	m, cmd := m.update(key('v'))
	m = drain(m, cmd)

	m, cmd = m.update(enter())
	m = drain(m, cmd)

	// Nothing came before the first version of a job. That is how jobs
	// begin, not something gone wrong.
	r.IsType(describePage{}, m.screen.page)
	r.NotEqual(flashErr, m.flash.level)
	r.Contains(plain(m.render()), "nothing before it")
}

func TestVersions_TheKeysOfAVersion(t *testing.T) {
	r := require.New(t)

	m, _ := onVersions(t)

	r.Equal([]hint{{Key: "<enter>", Description: "Diff"}, {Key: "<u>", Description: "Revert"}}, m.hints())
}

func TestVersions_AreAskedWhereTheJobLives(t *testing.T) {
	r := require.New(t)

	jobs := []nomad.Job{{ID: "web", Name: "web", Namespace: "default", Type: "service", Status: "running"}}
	client := &fakeClient{jobs: jobs, versions: threeVersions(), plan: nomad.Plan{To: 2, Version: 3}}

	m := New(client, Options{Namespace: nomad.AllNamespaces, Version: "v-test", PollEvery: time.Millisecond})
	m, _ = m.update(sizeMsg())
	m, _ = m.update(jobsMsg(jobs))

	m, cmd := m.update(key('v'))
	m = drain(m, cmd)

	r.Equal("default", client.askedNamespace)
	r.Contains(plain(m.render()), "Versions (Job: web) [3]")

	// What a version changed, and going back to it.
	client.askedNamespace = ""

	next, cmd := m.update(enter())
	drain(next, cmd)
	r.Equal("default", client.askedNamespace)

	client.askedNamespace = ""

	m, _ = m.update(key('j'))
	next, cmd = m.update(key('u'))
	drain(next, cmd)
	r.Equal("default", client.askedNamespace)
	r.Equal(uint64(2), *client.plannedTo)
}
