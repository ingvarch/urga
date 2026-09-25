package ui

import (
	"context"
	"fmt"
	"image/color"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// jobTitles are the columns of the job list.
var jobTitles = []string{"ID", "Name", "Type", "Namespace", "Status", "Allocs", "Age"}

// jobsPage is the jobs of the namespace the session looks at.
type jobsPage struct {
	jobs []nomad.Job
}

func (jobsPage) title(e env, count int) string {
	return sprintf("Jobs (%s) [%d]", namespaceLabel(e.namespace), count)
}

func (jobsPage) titles() []string { return jobTitles }
func (jobsPage) topics() []string { return []string{nomad.TopicJob} }

func (jobsPage) fetch(e env) tea.Cmd {
	client, namespace := e.client, e.namespace

	return fetchList(func(ctx context.Context) ([]nomad.Job, error) {
		return client.Jobs(ctx, namespace)
	}, func(items []nomad.Job) tea.Msg { return jobsMsg(items) })
}

func (p jobsPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	jobs, ok := msg.(jobsMsg)
	if !ok {
		return p, outcome{}, false
	}

	p.jobs = jobs

	return p, outcome{}, true
}

// visible are the jobs of the datacenter the session is narrowed to. The
// rows, the marks and the keys that find a row by its place read the same
// list, so a key finds the job on the screen.
func (p jobsPage) visible(e env) []nomad.Job { return jobsIn(e.datacenter, p.jobs) }

func (p jobsPage) rows(e env) []tableRow { return jobRows(p.visible(e)) }

func (p jobsPage) ids(e env) []string { return names(p.visible(e), jobMark) }

// picked is the job under the cursor.
func (p jobsPage) picked(e env) (nomad.Job, bool) { return pickedFrom(e, p.visible(e)) }

var jobsKeys = []pageKey[jobsPage]{
	{press: "enter", label: "Allocations", do: openJobAllocations},
	markKey[jobsPage](),
	markAllKey[jobsPage](),
	{press: "t", label: "Task Groups", do: openJobGroups},
	{press: "d", label: "Describe", do: describeJob},
	{press: "h", label: "Job Spec", do: showJobSpec},
	{press: "ctrl+s", label: "Start/Stop", do: startStopJob, writes: true},
	// The list of jobs reverts to the version before the one that runs;
	// the list of versions reverts to the one under the cursor.
	{press: "u", label: "Revert", do: revertJob, writes: true},
	{press: "v", label: "Versions", do: openVersions},
	{press: "l", label: "Logs", do: jobLogs},
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

// jobRows is one row per job, in the order of the columns.
func jobRows(jobs []nomad.Job) []tableRow {
	rows := make([]tableRow, 0, len(jobs))

	for _, job := range jobs {
		rows = append(rows, tableRow{
			cells: []string{
				job.ID,
				job.Name,
				job.Type,
				job.Namespace,
				job.Status,
				fmt.Sprintf("%d/%d", job.Running, job.Desired),
				ageOf(job.SubmitTime),
			},
			ages:  moments{6: job.SubmitTime},
			color: jobColor(job),
		})
	}

	return rows
}

// jobColor says what a job is up to without reading the row: a service short
// of allocations stands out, a dead one is red, a batch job that ended is
// spent rather than broken.
func jobColor(job nomad.Job) color.Color {
	switch job.Status {
	case statusRunning:
		if job.Type == typeService && job.Running != job.Desired {
			return colorAttention
		}
	case statusPending:
		return colorPending
	case statusDead, statusFailed:
		if job.Type == typeBatch {
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

	return p, then(openMsg(screen{
		kind:      screenAllocations,
		namespace: job.Namespace,
		page:      allocationsPage{namespace: job.Namespace, jobID: job.ID},
	}))
}
