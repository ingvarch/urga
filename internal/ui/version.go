package ui

import (
	"context"
	"fmt"
	"image/color"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// versionTitles are the columns of the versions of a job.
var versionTitles = []string{"Version", "State", "Tag", "Changes", "Age"}

var versionHints = []hint{
	{Key: "<enter>", Description: "What changed"},
	{Key: "<u>", Description: "Revert to it"},
}

// openVersions opens what the job under the cursor was before.
func (m Model) openVersions() (Model, tea.Cmd) {
	job, ok := selectedOf(m, screenJobs, m.jobs)
	if !ok {
		return m, nil
	}

	return m.push(screen{kind: screenJobVersions, namespace: job.Namespace, jobID: job.ID})
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
func (m Model) openVersionDiff() (Model, tea.Cmd) {
	version, ok := selectedOf(m, screenJobVersions, m.versions)
	if !ok {
		return m, nil
	}

	client, screen := m.client, m.screen

	return m, describe(fmt.Sprintf("%s version %d", screen.jobID, version.Version),
		func(ctx context.Context) (string, error) {
			return client.JobVersionDiff(ctx, screen.namespace, screen.jobID, version.Version)
		})
}

// revertToVersion puts the version under the cursor back in place.
func (m Model) revertToVersion() (Model, tea.Cmd) {
	version, ok := selectedOf(m, screenJobVersions, m.versions)
	if !ok {
		return m, nil
	}

	client, screen := m.client, m.screen

	return m.ask(
		fmt.Sprintf("Really revert %s to version %d?", screen.jobID, version.Version),
		act(fmt.Sprintf("Job %s reverted to version %d.", screen.jobID, version.Version), func(ctx context.Context) error {
			return client.RevertJobTo(ctx, screen.namespace, screen.jobID, version.Version)
		}),
	)
}
