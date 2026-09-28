package ui

import (
	"errors"
	"fmt"
	"image/color"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// periodicJobs are a service, a periodic job with two launches, the newest
// one last, and a parameterized job with one.
func periodicJobs() []nomad.Job {
	return []nomad.Job{
		{ID: "web", Name: "web", Namespace: "production", Type: "service", Status: "running", Running: 1, Desired: 1},
		{ID: "backup", Name: "backup", Namespace: "production", Type: "batch", Status: "running", Periodic: true},
		launchOf("backup", "backup/periodic-1758495600", "dead", -2*time.Hour),
		launchOf("backup", "backup/periodic-1758499200", "dead", -time.Hour),
		{ID: "report", Name: "report", Namespace: "production", Type: "batch", Status: "running", Parameterized: true},
		launchOf("report", "report/dispatch-1758499200-3f1c", "running", -time.Minute),
	}
}

// launchOf is a job a periodic or parameterized job launched some time ago.
func launchOf(parent, id, status string, ago time.Duration) nomad.Job {
	return nomad.Job{
		ID: id, Name: id, Namespace: "production", Type: "batch", Status: status,
		ParentID: parent, SubmitTime: time.Now().Add(ago),
	}
}

func TestJobs_LaunchesAreNotListed(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{jobs: periodicJobs()})
	m, _ = m.update(jobsMsg(periodicJobs()))

	// The jobs that launch them stand for their launches.
	r.Equal("Jobs (production) [3]", m.title())

	out := plain(m.render())
	r.Contains(out, "backup")
	r.Contains(out, "report")
	r.NotContains(out, "periodic-")
	r.NotContains(out, "dispatch-")
}

// onBackup is the job list with the cursor on the periodic job.
func onBackup(t *testing.T, client *fakeClient) Model {
	t.Helper()

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(client.jobs))
	m, _ = m.update(key('j'))

	return m
}

func TestJobs_EnterOpensTheLaunchesOfAPeriodicJob(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: periodicJobs(), changes: newChanges()}
	m := onBackup(t, client)

	// The job runs nothing itself: enter opens what it launched.
	r.Contains(m.hints(), hint{Key: "<enter>", Description: "Launches"})
	r.NotContains(m.hints(), hint{Key: "<enter>", Description: "Allocations"})

	m, cmd := m.update(enter())
	m = playOut(m, cmd)

	r.Equal("Launches (Job: backup) [2]", m.title())

	out := plain(m.render())
	r.Contains(out, "backup/periodic-1758495600")
	r.Contains(out, "backup/periodic-1758499200")
	r.NotContains(out, "dispatch-")

	// Back on the list, on the same job.
	m, cmd = m.update(escape())
	m = playOut(m, cmd)
	r.Equal("Jobs (production) [3]", m.title())
}

func TestJobs_EnterOpensTheLaunchesOfAParameterizedJob(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: periodicJobs(), changes: newChanges()}
	m := onBackup(t, client)
	m, _ = m.update(key('j'))

	m, cmd := m.update(enter())
	m = playOut(m, cmd)

	r.Equal("Launches (Job: report) [1]", m.title())
	r.Contains(plain(m.render()), "report/dispatch-1758499200-3f1c")
}

func TestLaunches_OnlyThoseOfTheJobInItsNamespace(t *testing.T) {
	r := require.New(t)

	// A job of the same name in another namespace is another job.
	elsewhere := launchOf("backup", "backup/periodic-1758402000", "dead", -3*time.Hour)
	elsewhere.Namespace = "staging"

	client := &fakeClient{jobs: append(periodicJobs(), elsewhere), changes: newChanges()}
	m := onBackup(t, client)

	m, cmd := m.update(enter())
	m = playOut(m, cmd)

	r.Equal("Launches (Job: backup) [2]", m.title())
	r.NotContains(plain(m.render()), "periodic-1758402000")
}

func TestLaunches_StayInTheNamespaceOfTheJob(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: periodicJobs(), changes: newChanges()}
	m := onBackup(t, client)
	m.namespaceOrder = []string{"production", "staging"}

	m, cmd := m.update(enter())
	m = playOut(m, cmd)

	// The job is in production: the launches are asked for there, whatever
	// the session looks at.
	m, cmd = m.update(key('2'))
	m = playOut(m, cmd)

	r.Equal("staging", m.namespace)
	r.Equal("Launches (Job: backup) [2]", m.title())
	r.Equal("production", client.askedNamespace)
	r.Equal("production", client.watchedNamespace)
}

func TestLaunches_EnterOpensTheAllocationsOfALaunch(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: periodicJobs(), allocs: twoAllocs(), changes: newChanges()}
	m := onBackup(t, client)

	m, cmd := m.update(enter())
	m = playOut(m, cmd)

	// A launch is a job like any other.
	r.Contains(m.hints(), hint{Key: "<enter>", Description: "Allocations"})

	m, cmd = m.update(enter())
	m = playOut(m, cmd)

	r.IsType(allocationsPage{}, m.screen.page)
	r.Equal("backup/periodic-1758495600", client.askedJobID)
}

func TestJobs_AJobThatLaunchesHasNoLogs(t *testing.T) {
	r := require.New(t)

	m := onBackup(t, &fakeClient{jobs: periodicJobs()})

	// It has no allocations to read them from; its launches have.
	r.NotContains(m.hints(), hint{Key: "<l>", Description: "Logs"})
}

// colorOf is the color of the row of a job on the job list.
func colorOf(t *testing.T, jobs []nomad.Job, id string) color.Color {
	t.Helper()

	for _, row := range (jobsPage{jobs: jobs}).rows(env{}) {
		if row.cells[0] == id {
			return row.color
		}
	}

	t.Fatalf("no row for %s", id)

	return nil
}

// withLaunches is a running periodic job and its launches, oldest first.
func withLaunches(launches ...nomad.Job) []nomad.Job {
	jobs := []nomad.Job{{ID: "backup", Namespace: "production", Type: "batch", Status: "running", Periodic: true}}

	for i, launch := range launches {
		launch.ID = fmt.Sprintf("backup/periodic-%d", i)
		launch.ParentID, launch.Namespace, launch.Type = "backup", "production", "batch"
		launch.SubmitTime = time.Now().Add(time.Duration(i-len(launches)) * time.Hour)
		jobs = append(jobs, launch)
	}

	return jobs
}

func TestJobs_AJobThatLaunchesHasTheColorOfItsLastLaunch(t *testing.T) {
	r := require.New(t)

	failed := nomad.Job{Status: "dead", Failed: true}
	done := nomad.Job{Status: "dead"}

	r.Equal(colorDead, colorOf(t, withLaunches(done, failed), "backup"))
	r.Equal(colorPending, colorOf(t, withLaunches(failed, nomad.Job{Status: "pending"}), "backup"))

	// A launch that runs, or one that did its work, leaves the job as it is:
	// an older launch that failed is over.
	r.Nil(colorOf(t, withLaunches(failed, nomad.Job{Status: "running"}), "backup"))
	r.Nil(colorOf(t, withLaunches(failed, done), "backup"))
	r.Nil(colorOf(t, withLaunches(), "backup"))
}

func TestJobs_ANewerLaunchIsTheLastOneInAnyOrder(t *testing.T) {
	r := require.New(t)

	jobs := withLaunches(nomad.Job{Status: "dead"}, nomad.Job{Status: "dead", Failed: true})
	jobs[1], jobs[2] = jobs[2], jobs[1]

	r.Equal(colorDead, colorOf(t, jobs, "backup"))
}

func TestJobs_AStoppedJobKeepsItsColorWhateverItLaunched(t *testing.T) {
	r := require.New(t)

	jobs := withLaunches(nomad.Job{Status: "dead", Failed: true})
	jobs[0].Status = "dead"

	// It launches nothing any more.
	r.Equal(colorSpent, colorOf(t, jobs, "backup"))
}

func TestJobs_AFailedLaunchIsInTrouble(t *testing.T) {
	r := require.New(t)

	jobs := append(twoJobs(), withLaunches(nomad.Job{Status: "dead", Failed: true})...)

	m := newTestModel(&fakeClient{jobs: jobs})
	m, _ = m.update(jobsMsg(jobs))

	// The rows that need attention keep the job whose last launch failed.
	m, _ = m.update(key('!'))
	r.Equal("Jobs (production) [1]", m.title())
	r.Contains(plain(m.render()), "backup")
}

func TestJobs_RunAPeriodicJobNow(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: periodicJobs(), changes: newChanges()}
	m := onBackup(t, client)

	r.Contains(m.hints(), hint{Key: "<r>", Description: "Run Now"})

	m, _ = m.update(key('r'))
	r.Contains(plain(m.render()), "Really run the job backup now?")

	m, cmd := m.update(key('y'))
	m = playOut(m, cmd)

	r.Equal([]string{"LaunchJob"}, client.writes)
	r.Equal("production", client.askedNamespace)
	r.Equal("backup", client.askedID)
	r.Contains(plain(m.render()), "Job backup launched.")
}

func TestJobs_RunNowIsForAPeriodicJobOnly(t *testing.T) {
	r := require.New(t)

	runNow := hint{Key: "<r>", Description: "Run Now"}

	// web is a service.
	m := newTestModel(&fakeClient{jobs: periodicJobs()})
	m, _ = m.update(jobsMsg(periodicJobs()))
	r.NotContains(m.hints(), runNow)

	// report runs when it is dispatched, with what the dispatch gives it.
	m, _ = m.update(key('j'))
	m, _ = m.update(key('j'))
	r.NotContains(m.hints(), runNow)
}

func TestJobs_ALaunchIsNotRunNow(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: periodicJobs(), changes: newChanges()}
	m := onBackup(t, client)

	m, cmd := m.update(enter())
	m = playOut(m, cmd)

	r.NotContains(m.hints(), hint{Key: "<r>", Description: "Run Now"})
}

// onLaunches are the launches of the job the cursor is on.
func onLaunches(t *testing.T, client *fakeClient, down int) Model {
	t.Helper()

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(client.jobs))

	for range down {
		m, _ = m.update(key('j'))
	}

	m, cmd := m.update(enter())

	return playOut(m, cmd)
}

func TestLaunches_TheTitleSaysWhenTheNextOneIs(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: periodicJobs(), next: time.Now().Add(4*time.Hour + 30*time.Minute), changes: newChanges()}
	m := onLaunches(t, client, 1)

	r.Equal("Launches (Job: backup, next in 4h) [2]", m.title())
	r.Equal("production/backup", client.nextOf)
}

func TestLaunches_NoNextOneToSay(t *testing.T) {
	r := require.New(t)

	// A periodic job that is stopped or turned off launches nothing.
	client := &fakeClient{jobs: periodicJobs(), changes: newChanges()}
	r.Equal("Launches (Job: backup) [2]", onLaunches(t, client, 1).title())

	// A parameterized job has no schedule to ask about.
	client = &fakeClient{jobs: periodicJobs(), next: time.Now().Add(time.Hour), changes: newChanges()}
	r.Equal("Launches (Job: report) [1]", onLaunches(t, client, 2).title())
	r.Empty(client.nextOf)
}

func TestLaunches_ShowWhenTheNextOneIsNotKnown(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: periodicJobs(), nextErr: errors.New("failed parsing cron expression"), changes: newChanges()}
	m := onLaunches(t, client, 1)

	// The schedule is broken; the launches that were made are still there.
	r.Equal("Launches (Job: backup) [2]", m.title())

	out := plain(m.render())
	r.Contains(out, "backup/periodic-1758499200")
	r.Contains(out, "failed parsing cron expression")
}

func TestLaunches_TheNextLaunchOfAnotherJobIsNotTaken(t *testing.T) {
	r := require.New(t)

	m := onLaunches(t, &fakeClient{jobs: periodicJobs(), changes: newChanges()}, 1)

	m, _ = m.update(nextLaunchMsg{of: "staging/backup", at: time.Now().Add(time.Hour)})
	r.Equal("Launches (Job: backup) [2]", m.title())
}
