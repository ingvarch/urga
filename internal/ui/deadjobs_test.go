package ui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestFault_ReadsAsItsState(t *testing.T) {
	r := require.New(t)

	words := map[fault]string{
		faultDead:         "dead",
		faultFailedOrLost: "failed or lost",
		faultRestarting:   "restarting",
		faultOOMKilled:    "OOM killed",
		faultDown:         "down",
		faultDisconnected: "disconnected",
		faultDraining:     "draining",
		faultIneligible:   "ineligible",
		faultBlocked:      "blocked",
		faultFailed:       "failed",
		faultPaused:       "paused",
		faultRunning:      "running",
	}

	for f, word := range words {
		r.Equal(word, f.String())
	}

	r.Empty(noFault.String())
}

// deadJobsFixture is a job of every kind the dead rule has to tell apart.
func deadJobsFixture() []nomad.Job {
	failedLaunch := launchOf("nightly", "nightly/periodic-1", "dead", -time.Hour)
	failedLaunch.Failed = true

	return []nomad.Job{
		{ID: "crashed", Namespace: "production", Type: "service", Status: "dead"},
		{ID: "stopped", Namespace: "production", Type: "service", Status: "dead", Stopped: true},
		{ID: "ended", Namespace: "production", Type: "batch", Status: "dead"},
		{ID: "broken", Namespace: "production", Type: "batch", Status: "dead", Failed: true},
		{ID: "nightly", Namespace: "production", Type: "batch", Status: "running", Periodic: true},
		failedLaunch,
		{ID: "web", Namespace: "production", Type: "service", Status: "running", Running: 1, Desired: 1},
	}
}

func TestDeadJobs_KeepsWhatTheListPaintsRedAndNobodyStopped(t *testing.T) {
	r := require.New(t)

	page := jobsPage{jobs: deadJobsFixture(), fault: faultDead}

	r.Equal([]string{"production/crashed", "production/broken", "production/nightly"}, page.ids(env{}))
}

func TestDeadJobs_TitleSaysWhatTheListIsNarrowedTo(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{jobs: deadJobsFixture()})
	m, _ = m.update(openMsg{jobsPage{jobs: deadJobsFixture(), fault: faultDead}})

	r.Equal("Jobs (production, dead) [3]", m.title())
}

func TestDeadJobs_KeysActOnTheRowsThatAreLeft(t *testing.T) {
	r := require.New(t)

	jobs := deadJobsFixture()
	// The first job read is not dead: the first row of the page is the first dead one.
	jobs = append([]nomad.Job{{ID: "alive", Namespace: "production", Type: "service", Status: "running", Running: 1, Desired: 1}}, jobs...)

	client := &fakeClient{jobs: jobs, changes: newChanges()}
	m := newTestModel(client)
	m, _ = m.update(openMsg{jobsPage{jobs: jobs, fault: faultDead}})

	m, cmd := m.update(enter())
	m = playOut(m, cmd)

	r.IsType(allocationsPage{}, m.screen.page)
	r.Equal("crashed", client.askedJobID)
}

func TestDeadJobs_FollowTheSession(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: deadJobsFixture(), changes: newChanges()}
	m := newTestModel(client)
	m.namespaceOrder = []string{"production", "staging"}
	m, _ = m.update(openMsg{jobsPage{jobs: client.jobs, fault: faultDead}})

	m, cmd := m.update(key('2'))
	m = playOut(m, cmd)

	r.Equal("staging", m.namespace)
	r.Equal("staging", client.askedNamespace)
	r.Equal(faultDead, m.screen.page.(jobsPage).fault)
}
