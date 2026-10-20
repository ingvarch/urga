package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// jobMetaPage is the meta of a job: a key and its value on each row.
type jobMetaPage struct {
	namespace, jobID string

	meta map[string]string
	read bool
}

func (p jobMetaPage) title(_ env, count int) string {
	return sprintf("Meta (Job: %s) [%d]", p.jobID, count)
}

func (jobMetaPage) titles() []string { return fieldTitles }
func (jobMetaPage) topics() []string { return []string{nomad.TopicJob} }

// where is the namespace of the job, which is where its stream watches.
func (p jobMetaPage) where(env) string { return p.namespace }

func (p jobMetaPage) fetch(e env) tea.Cmd {
	client, namespace, jobID := e.client, p.namespace, p.jobID

	return request(func(ctx context.Context) (map[string]string, error) {
		return client.JobMeta(ctx, namespace, jobID)
	}, func(meta map[string]string) tea.Msg {
		return jobMetaMsg{namespace: namespace, jobID: jobID, meta: meta}
	})
}

// take keeps the meta of the job the page is open on.
func (p jobMetaPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	meta, ok := msg.(jobMetaMsg)
	if !ok || meta.jobID != p.jobID || meta.namespace != p.namespace {
		return p, outcome{}, false
	}

	p.meta, p.read = meta.meta, true

	return p, outcome{}, true
}

func (p jobMetaPage) rows(env) []tableRow { return fieldRows(p.meta) }

// panel says that the job has no meta, once the cluster has answered.
func (p jobMetaPage) panel(_ env, width, room int) []string {
	if !p.read || len(p.meta) > 0 {
		return nil
	}

	head := []string{panelRow(styleMuted.Render(truncate(sprintf("The job %s has no meta.", p.jobID), width-tableIndent)))}

	return fitPanel(head, panelBlock{}, room)
}

var jobMetaKeys = []pageKey[jobMetaPage]{copyKey[jobMetaPage]()}

func (p jobMetaPage) keys(e env) []keyHint { return hintsOf(p, e, jobMetaKeys) }

func (p jobMetaPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, jobMetaKeys, k)
}

// openJobMeta opens the meta of the job under the cursor, in the namespace
// of the job.
func openJobMeta(p jobsPage, e env) (jobsPage, outcome) {
	job, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg{jobMetaPage{namespace: job.Namespace, jobID: job.ID}})
}
