package nomad_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

const jobVersions = `{
	"Versions": [
		{
			"ID": "web", "Name": "web", "Namespace": "production",
			"Version": 3, "Stable": true, "SubmitTime": 1758499200000000000,
			"VersionTag": {"Name": "golden", "Description": "the one that works"}
		},
		{"ID": "web", "Name": "web", "Version": 2, "Stable": false, "SubmitTime": 1758412800000000000},
		{"ID": "web", "Name": "web", "Version": 1, "Stable": true, "SubmitTime": 1758326400000000000}
	],
	"Diffs": [
		{
			"Type": "Edited", "ID": "web",
			"Fields": [{"Type": "Edited", "Name": "Priority", "Old": "50", "New": "70"}],
			"TaskGroups": [{
				"Type": "Edited", "Name": "bot",
				"Fields": [{"Type": "Edited", "Name": "Count", "Old": "1", "New": "3"}],
				"Tasks": [{
					"Type": "Edited", "Name": "bot",
					"Fields": [{"Type": "Added", "Name": "Env[DEBUG]", "New": "true"}]
				}]
			}]
		},
		{"Type": "Edited", "ID": "web", "Fields": [{"Type": "Deleted", "Name": "Region", "Old": "global"}]}
	]
}`

func TestJobVersions_Read(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, jobVersions)

	versions, err := client.JobVersions(context.Background(), "production", "web")
	r.NoError(err)
	r.Len(versions, 3)

	r.Equal("/v1/job/web/versions", asked.URL.Path)
	r.Equal("true", asked.URL.Query().Get("diffs"))
	r.Equal("production", asked.URL.Query().Get("namespace"))

	// The newest version is the one that runs, and the list says so.
	r.Equal(uint64(3), versions[0].Version)
	r.True(versions[0].Current)
	r.True(versions[0].Stable)
	r.Equal("golden", versions[0].Tag)
	r.False(versions[0].Submitted.IsZero())

	r.False(versions[1].Current)
	r.False(versions[1].Stable)
	r.Empty(versions[1].Tag)

	// How much each version changed from the one before it, which is what
	// the cluster answers with next to the versions themselves.
	r.Equal(3, versions[0].Changes)
	r.Equal(1, versions[1].Changes)

	// The oldest version has nothing before it to differ from.
	r.Zero(versions[2].Changes)
}

const objectVersions = `{
	"Versions": [
		{"ID": "web", "Name": "web", "Version": 2},
		{"ID": "web", "Name": "web", "Version": 1}
	],
	"Diffs": [{
		"Type": "Edited", "ID": "web",
		"Objects": [{
			"Type": "Edited", "Name": "Update",
			"Fields": [{"Type": "Edited", "Name": "MaxParallel", "Old": "1", "New": "2"}]
		}],
		"TaskGroups": [{
			"Type": "Edited", "Name": "bot",
			"Tasks": [{
				"Type": "Edited", "Name": "bot",
				"Objects": [{
					"Type": "Edited", "Name": "Resources",
					"Fields": [
						{"Type": "Edited", "Name": "CPU", "Old": "100", "New": "200"},
						{"Type": "Edited", "Name": "MemoryMB", "Old": "128", "New": "256"}
					],
					"Objects": [{
						"Type": "Added", "Name": "Network",
						"Fields": [{"Type": "Added", "Name": "MBits", "New": "10"}]
					}]
				}]
			}]
		}]
	}]
}`

func TestJobVersions_CountsFieldsInsideObjects(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, objectVersions)

	versions, err := client.JobVersions(context.Background(), "production", "web")
	r.NoError(err)

	// A field counts wherever it sits, however deep the objects around it.
	r.Equal(4, versions[0].Changes)
}

const templateVersions = `{
	"Versions": [
		{"ID": "web", "Name": "web", "Version": 2},
		{"ID": "web", "Name": "web", "Version": 1}
	],
	"Diffs": [{
		"Type": "Edited", "ID": "web",
		"TaskGroups": [{
			"Type": "Edited", "Name": "bot",
			"Tasks": [{
				"Type": "Edited", "Name": "bot",
				"Objects": [{
					"Type": "Edited", "Name": "Template",
					"Fields": [{
						"Type": "Edited", "Name": "EmbeddedTmpl",
						"Old": "services:\n- web\n- api",
						"New": "services:\n- web\n- worker"
					}]
				}]
			}]
		}]
	}]
}`

func TestJobVersions_CountsAValueOnManyLinesOnce(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, templateVersions)

	versions, err := client.JobVersions(context.Background(), "production", "web")
	r.NoError(err)

	// A template body is one field, even when its lines read like a diff.
	r.Equal(1, versions[0].Changes)
}

func TestJobVersionDiff_ReadsAsTheJobFile(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, jobVersions)

	diff, err := client.JobVersionDiff(context.Background(), "production", "web", 3)
	r.NoError(err)

	// What changed, in the shape the job file reads in: the job, its groups
	// and the tasks under them, each change as the line it was and the line
	// it is.
	r.Contains(diff, nomad.DiffLine{Kind: nomad.DiffDeleted, Text: "priority = 50"})
	r.Contains(diff, nomad.DiffLine{Kind: nomad.DiffAdded, Text: "priority = 70"})
	r.Contains(diff, nomad.DiffLine{Kind: nomad.DiffContext, Text: `group "bot" {`})
	r.Contains(diff, nomad.DiffLine{Kind: nomad.DiffAdded, Indent: 1, Text: "count = 3"})
	r.Contains(diff, nomad.DiffLine{Kind: nomad.DiffContext, Indent: 1, Text: `task "bot" {`})
	r.Contains(diff, nomad.DiffLine{Kind: nomad.DiffAdded, Indent: 3, Text: `DEBUG = "true"`})
}

func TestJobVersionDiff_OfTheOldestVersion(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, jobVersions)

	_, err := client.JobVersionDiff(context.Background(), "production", "web", 1)

	// Nothing came before it, so there is nothing to compare it with.
	r.ErrorIs(err, nomad.ErrNoDiff)
}
