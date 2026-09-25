package ui

import (
	"context"
	"errors"
	"fmt"
	"image/color"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// versionTitles are the columns of the versions of a job.
var versionTitles = []string{"Version", "State", "Tag", "Changes", "Age"}

// versionsPage is what one job was before.
type versionsPage struct {
	namespace, jobID string

	versions []nomad.JobVersion
}

func (p versionsPage) title(_ env, count int) string {
	return sprintf("Versions (Job: %s) [%d]", p.jobID, count)
}

func (versionsPage) titles() []string { return versionTitles }
func (versionsPage) topics() []string { return nil }

// fetch reads the versions of the job the page is open on.
func (p versionsPage) fetch(e env) tea.Cmd {
	client, namespace, jobID := e.client, p.namespace, p.jobID

	return fetchList(func(ctx context.Context) ([]nomad.JobVersion, error) {
		return client.JobVersions(ctx, namespace, jobID)
	}, func(items []nomad.JobVersion) tea.Msg {
		return versionsMsg{jobID: jobID, versions: items}
	})
}

// take keeps the versions of the job the page is open on.
func (p versionsPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	versions, ok := msg.(versionsMsg)
	if !ok || versions.jobID != p.jobID {
		return p, outcome{}, false
	}

	p.versions = versions.versions

	return p, outcome{}, true
}

func (p versionsPage) rows(env) []tableRow { return versionRows(p.versions) }

// picked is the version under the cursor.
func (p versionsPage) picked(e env) (nomad.JobVersion, bool) { return pickedFrom(e, p.versions) }

var versionKeys = []pageKey[versionsPage]{
	{press: "enter", label: "Diff", do: openVersionDiff},
	{press: "u", label: "Revert", do: revertToVersion, writes: true},
}

func (p versionsPage) keys(e env) []keyHint { return hintsOf(p, e, versionKeys) }

func (p versionsPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, versionKeys, k)
}

// openVersions opens what the job under the cursor was before, on a page
// that holds nothing until this job is answered for.
func openVersions(p jobsPage, e env) (jobsPage, outcome) {
	job, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg(screen{
		kind: screenJobVersions,
		page: versionsPage{namespace: job.Namespace, jobID: job.ID},
	}))
}

func versionRows(versions []nomad.JobVersion) []tableRow {
	rows := make([]tableRow, 0, len(versions))

	for _, version := range versions {
		rows = append(rows, tableRow{
			cells: []string{
				fmt.Sprintf("%d", version.Version),
				versionState(version),
				version.Tag,
				changes(version.Changes),
				ageOf(version.Submitted),
			},
			ages:  moments{4: version.Submitted},
			color: versionColor(version),
		})
	}

	return rows
}

// versionState is what the cluster makes of a version: the one that runs,
// one it was happy with, or one it was not.
func versionState(version nomad.JobVersion) string {
	switch {
	case version.Current:
		return "current"
	case version.Stable:
		return "stable"
	}

	return ""
}

func versionColor(version nomad.JobVersion) color.Color {
	if version.Current {
		return colorTitle
	}

	return nil
}

// changes is how much a version changed from the one before it.
func changes(count int) string {
	switch count {
	case 0:
		return ""
	case 1:
		return "1 field"
	}

	return fmt.Sprintf("%d fields", count)
}

// openVersionDiff shows what the version under the cursor changed.
func openVersionDiff(p versionsPage, e env) (versionsPage, outcome) {
	version, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	client, namespace, jobID := e.client, p.namespace, p.jobID

	return p, outcome{cmd: describeLines(fmt.Sprintf("%s version %d", jobID, version.Version),
		func(ctx context.Context) ([]paintedLine, error) {
			diff, err := client.JobVersionDiff(ctx, namespace, jobID, version.Version)

			// The first version of a job changed nothing: there is nothing
			// before it to compare it with.
			if errors.Is(err, nomad.ErrNoDiff) {
				return plainLines(fmt.Sprintf(
					"Version %d of %s is the first one the cluster kept.\n\n"+
						"There is nothing before it to compare it with.", version.Version, jobID)), nil
			}

			return diffLines(diff), err
		})}
}

// revertToVersion puts the version under the cursor back in place, after its
// plan.
func revertToVersion(p versionsPage, e env) (versionsPage, outcome) {
	version, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	to := version.Version

	return p, outcome{cmd: planFor(e.client, planState{revert: true, to: &to, namespace: p.namespace, jobID: p.jobID})}
}
