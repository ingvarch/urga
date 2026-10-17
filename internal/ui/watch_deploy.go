package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// deployWatchMsg is what one read of a watched deploy found.
type deployWatchMsg struct {
	namespace, jobID string
	version          uint64

	// versionIndex is the JobModifyIndex of the version.
	versionIndex uint64

	// eval is the newest evaluation of the job, deployment the deployment
	// of the version, both nil when there is none.
	eval       *nomad.EvaluationDetail
	deployment *nomad.DeploymentDetail

	// allocs are those of the deployment, or the job's at the version.
	allocs []nomad.Alloc
}

// watchClient is what the watch of a deploy reads.
type watchClient interface {
	jobsClient
	allocsClient
	deploymentsClient
	evaluationsClient
}

// deployWatchPage is the deploy of one version of a job as it goes: the
// newest evaluation and how the deployment stands, above the allocations of
// the deployment. Before the deployment exists the rows are the job's
// allocations at that version, and the keys those of an allocation.
type deployWatchPage struct {
	namespace, jobID, jobType string

	// version is the version watched. A page opened without one reads the
	// version the job runs on its first fetch; versioned says it has.
	version      uint64
	versionIndex uint64
	versioned    bool

	// eval is the newest evaluation of the job. read says the page has read
	// the job once: until then there is no panel.
	eval *nomad.EvaluationDetail
	read bool

	// deployment is the deployment screen the watch takes the place of: its
	// panel, rows and keys are the watch's once the deployment exists.
	deployment deploymentPage
}

func (p deployWatchPage) title(_ env, count int) string {
	if !p.versioned {
		return sprintf("Deploy (Job: %s) [%d]", p.jobID, count)
	}

	return sprintf("Deploy (Job: %s, Version: %d) [%d]", p.jobID, p.version, count)
}

func (p deployWatchPage) titles() []string { return p.deployment.titles() }

func (p deployWatchPage) where(env) string { return p.namespace }

func (deployWatchPage) topics() []string {
	return []string{nomad.TopicJob, nomad.TopicEvaluation, nomad.TopicDeployment, nomad.TopicAllocation}
}

func (p deployWatchPage) fetch(e env) tea.Cmd {
	client := e.client

	return request(func(ctx context.Context) (deployWatchMsg, error) {
		return readDeploy(ctx, client, p)
	}, func(msg deployWatchMsg) tea.Msg { return msg })
}

// readDeploy reads the job in its namespace: the version, when the page has
// none yet, the newest evaluation, the deployment of that version and the
// allocations.
func readDeploy(ctx context.Context, client watchClient, p deployWatchPage) (deployWatchMsg, error) {
	read := deployWatchMsg{namespace: p.namespace, jobID: p.jobID, version: p.version, versionIndex: p.versionIndex}

	if !p.versioned {
		versions, err := client.JobVersions(ctx, p.namespace, p.jobID)
		if err != nil {
			return read, err
		}

		if len(versions) == 0 {
			return read, fmt.Errorf("%s has no versions", p.jobID)
		}

		read.version, read.versionIndex = versions[0].Version, versions[0].JobModifyIndex
	}

	// The evaluation goes first: the scheduler writes the deployment before
	// it completes the evaluation, so an evaluation seen complete means the
	// deployment read after it is there.
	var err error
	if read.eval, err = client.NewestEvaluation(ctx, p.namespace, p.jobID); err != nil {
		return read, err
	}

	deployments, err := client.JobDeployments(ctx, p.namespace, p.jobID)
	if err != nil {
		return read, err
	}

	// The newest deployment of the job may be of a newer version.
	if i := slices.IndexFunc(deployments, func(d nomad.DeploymentDetail) bool { return d.JobVersion == read.version }); i >= 0 {
		read.deployment = &deployments[i]
	}

	read.allocs, err = deployAllocs(ctx, client, p.namespace, p.jobID, read)

	return read, err
}

// deployAllocs are the allocations the deployment placed, or without one
// those of the job at the version.
func deployAllocs(ctx context.Context, client watchClient, namespace, jobID string, read deployWatchMsg) ([]nomad.Alloc, error) {
	if read.deployment != nil {
		return client.DeploymentAllocations(ctx, namespace, read.deployment.ID)
	}

	allocs, err := client.Allocations(ctx, namespace, jobID)

	return keep(allocs, func(a nomad.Alloc) bool { return a.JobVersion == read.version }), err
}

func (p deployWatchPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	read, ok := msg.(deployWatchMsg)
	if !ok || read.namespace != p.namespace || read.jobID != p.jobID {
		return p, outcome{}, false
	}

	// A read asked before the version was known may bring a newer one.
	if p.versioned && read.version != p.version {
		return p, outcome{}, true
	}

	p.version, p.versionIndex, p.versioned, p.eval, p.read = read.version, read.versionIndex, true, read.eval, true
	p.deployment = deploymentPage{namespace: p.namespace, jobID: p.jobID, allocs: read.allocs, version: p.version, versioned: true}

	if read.deployment != nil {
		p.deployment.deployment, p.deployment.read = *read.deployment, true
		p.deployment.deploymentID = read.deployment.ID
	}

	return p, outcome{}, true
}

// panel is what the watch adds, then the groups of the deployment.
func (p deployWatchPage) panel(_ env, width, room int) []string {
	if !p.read {
		return nil
	}

	head := []string{}

	if p.eval != nil {
		head = append(head, panelRow(watchedEvaluation(*p.eval, width-tableIndent)))
	}

	head = append(head, panelRow(truncate(p.standing(), width-tableIndent)))

	return fitPanel(head, groupBlock(p.deployment.deployment.Groups, p.deployment.active(), width), room)
}

// standing is one line on how the deploy stands.
func (p deployWatchPage) standing() string {
	d := p.deployment.deployment

	switch {
	case !p.deployment.read && p.makesNoDeployment():
		return fmt.Sprintf("%s has no deployment for version %d: its allocations are below.", p.jobID, p.version)
	case !p.deployment.read:
		return fmt.Sprintf("Waiting for the deployment of version %d.", p.version)
	case d.Active():
		return fmt.Sprintf("Deployment %s: %s", d.Status, d.StatusDescription)
	case d.Status == "successful":
		return "Deployment successful."
	}

	return fmt.Sprintf("Deployment %s: %s.", d.Status, d.StatusDescription)
}

// makesNoDeployment says the version will not get a deployment: its job is
// of a type that has none, no evaluation is left of it (a submit always
// writes one, so they were collected), or an evaluation that saw the version
// completed and made none.
func (p deployWatchPage) makesNoDeployment() bool {
	if p.jobType == typeBatch || p.jobType == typeSysbatch {
		return true
	}

	return p.eval == nil || p.eval.Status == "complete" && p.eval.CreateIndex >= p.versionIndex
}

// watchedEvaluation is the newest evaluation: its status, and when it did not
// place everything, why, in the words of the evaluation screen.
func watchedEvaluation(eval nomad.EvaluationDetail, width int) string {
	line := fmt.Sprintf("Evaluation %s %s", shortID(eval.ID), eval.Status)

	failures := make([]string, 0, len(eval.Failures))
	for _, failure := range eval.Failures {
		failures = append(failures, failureHeading(failure)+" "+strings.Join(placementReasons(failure), ", "))
	}

	if len(failures) > 0 {
		line += ": " + strings.Join(failures, " ")
	}

	return truncate(line, width)
}

func (p deployWatchPage) rows(e env) []tableRow { return p.deployment.rows(e) }

func (p deployWatchPage) ids(e env) []string { return p.deployment.ids(e) }

func (p deployWatchPage) readings(e env) []rowRef { return p.deployment.readings(e) }

func (p deployWatchPage) reading(ctx context.Context, client Client, ref rowRef) (nomad.ResourceUse, error) {
	return p.deployment.reading(ctx, client, ref)
}

func (p deployWatchPage) keys(e env) []keyHint { return p.deployment.keys(e) }

func (p deployWatchPage) press(k string, e env) (page, outcome, bool) {
	next, out, ok := p.deployment.press(k, e)
	if ok {
		p.deployment = next.(deploymentPage)
	}

	return p, out, ok
}

// watchJob opens the watch of the deploy of the job under the cursor, at the
// version it runs.
func watchJob(p jobsPage, e env) (jobsPage, outcome) {
	job, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg{deployWatchPage{namespace: job.Namespace, jobID: job.ID, jobType: job.Type}})
}
