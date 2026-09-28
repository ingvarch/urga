package ui

import (
	"context"
	"fmt"
	"image/color"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// jobTitles are the columns of the job list.
var jobTitles = []string{"ID", "Name", "Type", "Namespace", "Status", "Allocs", "Age"}

// jobsPage is the jobs of the namespace the session looks at, or the
// launches of one periodic or parameterized job.
type jobsPage struct {
	// parent is the job whose launches the page lists, and only the one job
	// it lists, found by a search. Without either, it lists the jobs of the
	// session.
	parent, only nomad.Job

	// next is when a periodic parent launches next, zero for never.
	next time.Time

	jobs []nomad.Job
}

func (p jobsPage) title(e env, count int) string {
	if p.ofParent() {
		return sprintf("Launches (Job: %s%s) [%d]", p.parent.ID, nextIn(p.next), count)
	}

	if p.alone() {
		return sprintf("Job %s (%s) [%d]", p.only.ID, p.only.Namespace, count)
	}

	return sprintf("Jobs (%s) [%d]", namespaceLabel(e.namespace), count)
}

// ofParent says the page lists the launches of a job; alone, that it lists
// one job.
func (p jobsPage) ofParent() bool { return p.parent.ID != "" }
func (p jobsPage) alone() bool    { return p.only.ID != "" }

// followsSession: the jobs of the session follow it; the launches of a job,
// and a job alone, stay where the job lives.
func (p jobsPage) followsSession() bool { return !p.ofParent() && !p.alone() }

// where is the namespace the jobs are asked in: the one of the job the page
// is about, or the session's.
func (p jobsPage) where(e env) string {
	switch {
	case p.ofParent():
		return p.parent.Namespace
	case p.alone():
		return p.only.Namespace
	}

	return e.namespace
}

func (jobsPage) titles() []string { return jobTitles }
func (jobsPage) topics() []string { return []string{nomad.TopicJob} }

// fetch reads the jobs, and when the page lists the launches of a periodic
// job, when it launches next.
func (p jobsPage) fetch(e env) tea.Cmd {
	client, namespace := e.client, p.where(e)

	jobs := fetchList(func(ctx context.Context) ([]nomad.Job, error) {
		return client.Jobs(ctx, namespace)
	}, func(items []nomad.Job) tea.Msg { return jobsMsg(items) })

	if !p.parent.Periodic {
		return jobs
	}

	return tea.Batch(jobs, fetchNextLaunch(client, p.parent))
}

func (p jobsPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	switch msg := msg.(type) {
	case jobsMsg:
		p.jobs = msg

		return p, outcome{}, true

	case nextLaunchMsg:
		// Only the next launch of this page's job is kept.
		if msg.of != jobMark(p.parent) {
			return p, outcome{}, false
		}

		p.next = msg.at

		return p, outcome{}, true
	}

	return p, outcome{}, false
}

// visible are the jobs of the datacenter the session is narrowed to that the
// page lists. The rows, the marks and the keys that find a row by its place
// read the same list, so a key finds the job on the screen.
func (p jobsPage) visible(e env) []nomad.Job { return keep(jobsIn(e.datacenter, p.jobs), p.lists) }

// lists says the page shows a job: a launch of its parent, the one job it is
// about, or, without either, a job that is not a launch. The job that
// launches them stands for them.
func (p jobsPage) lists(job nomad.Job) bool {
	switch {
	case p.ofParent():
		return job.ParentID == p.parent.ID && job.Namespace == p.parent.Namespace
	case p.alone():
		return job.ID == p.only.ID && job.Namespace == p.only.Namespace
	}

	return job.ParentID == ""
}

func (p jobsPage) rows(e env) []tableRow { return jobRows(p.visible(e), lastLaunches(p.jobs)) }

func (p jobsPage) ids(e env) []string { return names(p.visible(e), jobMark) }

// picked is the job under the cursor.
func (p jobsPage) picked(e env) (nomad.Job, bool) { return pickedFrom(e, p.visible(e)) }

var jobsKeys = []pageKey[jobsPage]{
	{press: "enter", label: "Allocations", do: openJobAllocations, offered: runsItself},
	{press: "enter", label: "Launches", do: openLaunches, offered: launchesJobs},
	markKey[jobsPage](),
	markAllKey[jobsPage](),
	{press: "t", label: "Task Groups", do: openJobGroups},
	{press: "d", label: "Describe", do: describeJob},
	{press: "h", label: "Job Spec", do: showJobSpec},
	{press: "ctrl+s", label: "Start/Stop", do: startStopJob, writes: true},
	{press: "r", label: "Run Now", do: runNow, writes: true, offered: runsOnSchedule},
	{press: "r", label: "Dispatch", do: dispatchJob, writes: true, offered: dispatchable},
	// The cluster evaluates the launches of a job, not the job that makes
	// them.
	{press: "ctrl+e", label: "Evaluate", do: evaluateJobs, writes: true, offered: runsItself},
	// The list of jobs reverts to the version before the one that runs;
	// the list of versions reverts to the one under the cursor.
	{press: "u", label: "Revert", do: revertJob, writes: true},
	{press: "v", label: "Versions", do: openVersions},
	{press: "l", label: "Logs", do: jobLogs, offered: runsItself},
	{press: "e", label: "Edit", do: editJob, writes: true},
	{press: "p", label: "Placement", do: jobPlacement, offered: jobWaits},
}

func (p jobsPage) keys(e env) []keyHint { return hintsOf(p, e, jobsKeys) }

func (p jobsPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, jobsKeys, k)
}

// namespaceLabel is the namespace as it reads in a title.
func namespaceLabel(namespace string) string {
	if namespace == "" || namespace == nomad.AllNamespaces {
		return "all"
	}

	return namespace
}

// jobRows is one row per job, in the order of the columns. last are the
// newest launches of the jobs that launch others.
func jobRows(jobs []nomad.Job, last map[string]nomad.Job) []tableRow {
	rows := make([]tableRow, 0, len(jobs))

	for _, job := range jobs {
		row := tableRow{color: launcherColor(job, last)}
		row.add(job.ID, job.Name, job.Type, job.Namespace, job.Status, fmt.Sprintf("%d/%d", job.Running, job.Desired))
		row.addAge(job.SubmitTime)

		rows = append(rows, row)
	}

	return rows
}

// jobColor shows the state of a job without reading the row: a service short
// of allocations gets the attention color, a dead one is red, a batch job
// that ended gets the spent color, not the dead one, unless a group of it
// failed.
func jobColor(job nomad.Job) color.Color {
	switch job.Status {
	case statusRunning:
		if job.Type == typeService && job.Running != job.Desired {
			return colorAttention
		}
	case statusPending:
		return colorPending
	case statusDead, statusFailed:
		if job.Type == typeBatch && !job.Failed {
			return colorSpent
		}

		return colorDead
	}

	return nil
}

const (
	statusRunning = "running"
	statusPending = "pending"
	statusDead    = "dead"
	statusFailed  = "failed"

	typeService = "service"
	typeBatch   = "batch"
)

// openJobAllocations drills into the job under the cursor: the allocations
// it runs.
func openJobAllocations(p jobsPage, e env) (jobsPage, outcome) {
	job, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg{allocationsPage{namespace: job.Namespace, jobID: job.ID}})
}
