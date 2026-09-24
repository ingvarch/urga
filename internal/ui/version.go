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

var versionBindings = []binding{
	{press: "enter", label: "Diff", do: openVersionDiff},
	{press: "u", label: "Revert", do: revertToVersion, writes: true},
}

// openVersions opens what the job under the cursor was before.
func openVersions(m Model) (Model, tea.Cmd) {
	job, ok := selectedOf(m, screenJobs, m.jobs)
	if !ok {
		return m, nil
	}

	// What was read of another job is let go of: the screen holds nothing
	// until this one is answered for.
	m.versions = nil

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
func openVersionDiff(m Model) (Model, tea.Cmd) {
	version, ok := selectedOf(m, screenJobVersions, m.versions)
	if !ok {
		return m, nil
	}

	client, screen := m.client, m.screen

	return m, describeLines(fmt.Sprintf("%s version %d", screen.jobID, version.Version),
		func(ctx context.Context) ([]paintedLine, error) {
			diff, err := client.JobVersionDiff(ctx, screen.namespace, screen.jobID, version.Version)

			// The first version of a job changed nothing: there is nothing
			// before it to compare it with.
			if errors.Is(err, nomad.ErrNoDiff) {
				return plainLines(fmt.Sprintf(
					"Version %d of %s is the first one the cluster kept.\n\n"+
						"There is nothing before it to compare it with.", version.Version, screen.jobID)), nil
			}

			return diffLines(diff), err
		})
}

// revertToVersion puts the version under the cursor back in place, after its
// plan.
func revertToVersion(m Model) (Model, tea.Cmd) {
	version, ok := selectedOf(m, screenJobVersions, m.versions)
	if !ok {
		return m, nil
	}

	to := version.Version

	return m, planFor(m.client, planState{revert: true, to: &to, namespace: m.screen.namespace, jobID: m.screen.jobID})
}
