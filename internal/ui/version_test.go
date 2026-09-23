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

	client := &fakeClient{jobs: twoJobs(), versions: threeVersions(), diff: "~ Priority: 50 -> 70"}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, cmd := m.update(key('v'))

	return drain(m, cmd), client
}

func TestVersions_ListWhatTheJobWas(t *testing.T) {
	r := require.New(t)

	m, client := onVersions(t)

	r.Equal(screenJobVersions, m.screen.kind)
	r.Equal("web", client.askedJobID)

	out := plain(m.render())
	r.Contains(out, "Versions (Job: web) [3]")

	// What each version is: its number, whether it runs, whether the
	// cluster was happy with it, what it was called and how much it changed.
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

	r.Equal(screenDescribe, m.screen.kind)
	r.Equal(uint64(3), client.askedVersion)
	r.Contains(plain(m.render()), "~ Priority: 50 -> 70")
}

func TestVersions_RevertToTheOneUnderTheCursor(t *testing.T) {
	r := require.New(t)

	m, client := onVersions(t)

	// Down to version 2, which is not the one that runs.
	m, _ = m.update(key('j'))
	m, _ = m.update(key('u'))

	r.Contains(plain(m.render()), "revert web to version 2")

	_, cmd := answerYes(m)
	drain(m, cmd)

	r.Equal(uint64(2), client.revertedTo)
	r.Equal("web", client.askedJobID)
}

func TestVersions_TheJobScreenStillRevertsToThePrevious(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs()}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, _ = m.update(key('u'))

	// The key that was there keeps its meaning on the list of jobs.
	r.Contains(plain(m.render()), "revert")
	r.Equal(screenJobs, m.screen.kind)
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
	// stand under its name.
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
	r.Equal(screenDescribe, m.screen.kind)
	r.NotEqual(flashErr, m.flash.level)
	r.Contains(plain(m.render()), "nothing before it")
}
