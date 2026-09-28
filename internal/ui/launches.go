package ui

import (
	"context"
	"fmt"
	"image/color"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// nextLaunchMsg is when a periodic job launches next, of the job it names by
// its mark.
type nextLaunchMsg struct {
	of string
	at time.Time
}

// fetchNextLaunch asks when a periodic job launches next.
func fetchNextLaunch(client jobsClient, job nomad.Job) tea.Cmd {
	return request(func(ctx context.Context) (time.Time, error) {
		return client.NextLaunch(ctx, job.Namespace, job.ID)
	}, func(at time.Time) tea.Msg { return nextLaunchMsg{of: jobMark(job), at: at} })
}

// nextIn is when the next launch is, as a title says it: nothing when none
// is due.
func nextIn(next time.Time) string {
	if left := time.Until(next); !next.IsZero() && left > 0 {
		return ", next in " + age(left)
	}

	return ""
}

// lastLaunches are the newest launch of each job that launched any, by the
// mark of that job.
func lastLaunches(jobs []nomad.Job) map[string]nomad.Job {
	last := map[string]nomad.Job{}

	for _, job := range jobs {
		if job.ParentID == "" {
			continue
		}

		parent := jobMark(nomad.Job{Namespace: job.Namespace, ID: job.ParentID})
		if newest, ok := last[parent]; !ok || job.SubmitTime.After(newest.SubmitTime) {
			last[parent] = job
		}
	}

	return last
}

// launcherColor is the color of a job, or of its last launch while it
// launches them: that is where it fails. A launch that runs or did its work
// leaves the color of the job.
func launcherColor(job nomad.Job, last map[string]nomad.Job) color.Color {
	launch, ok := last[jobMark(job)]
	if !ok || job.Status != statusRunning {
		return jobColor(job)
	}

	if c := jobColor(launch); c != nil && c != colorSpent {
		return c
	}

	return jobColor(job)
}

// launchesJobs says the job under the cursor is periodic or parameterized: it
// runs nothing itself, each launch is a job of its own.
func launchesJobs(p jobsPage, e env) bool {
	job, ok := p.picked(e)

	return ok && (job.Periodic || job.Parameterized)
}

// runsItself says the job under the cursor has allocations of its own, when
// there is one.
func runsItself(p jobsPage, e env) bool { return !launchesJobs(p, e) }

// openLaunches opens the jobs the job under the cursor launched.
func openLaunches(p jobsPage, e env) (jobsPage, outcome) {
	job, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg{jobsPage{parent: job}})
}

// runsOnSchedule says the job under the cursor is periodic. A parameterized
// job is not run this way: a dispatch gives it what it runs with.
func runsOnSchedule(p jobsPage, e env) bool {
	job, ok := p.picked(e)

	return ok && job.Periodic
}

// runNow launches the periodic job under the cursor out of its schedule,
// after the user confirms.
func runNow(p jobsPage, e env) (jobsPage, outcome) {
	job, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	client := e.client

	return p, then(askMsg{
		question: fmt.Sprintf("Really run the job %s now?", job.ID),
		apply: act(fmt.Sprintf("Job %s launched.", job.ID), func(ctx context.Context) error {
			return client.LaunchJob(ctx, job.Namespace, job.ID)
		}),
	})
}
