package nomad_test

import (
	"context"
	"encoding/json"
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

func TestJobVersionDiff_ReadsAsText(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, jobVersions)

	diff, err := client.JobVersionDiff(context.Background(), "production", "web", 3)
	r.NoError(err)

	// What changed, in the shape a diff reads in: the job, its groups and
	// the tasks under them.
	r.Contains(diff, "~ Priority: 50 -> 70")
	r.Contains(diff, "Task Group: bot")
	r.Contains(diff, "~ Count: 1 -> 3")
	r.Contains(diff, "Task: bot")
	r.Contains(diff, "+ Env[DEBUG]: true")
}

func TestJobVersionDiff_OfTheOldestVersion(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, jobVersions)

	_, err := client.JobVersionDiff(context.Background(), "production", "web", 1)

	// Nothing came before it, so there is nothing to compare it with.
	r.ErrorIs(err, nomad.ErrNoDiff)
}

func TestRevertJobTo_TakesTheVersionItIsGiven(t *testing.T) {
	r := require.New(t)

	client, sent, asked := metaServer(t, `{}`)

	err := client.RevertJobTo(context.Background(), "production", "web", 2)
	r.NoError(err)

	request := struct {
		JobID      string
		JobVersion uint64
	}{}
	r.NoError(json.Unmarshal(*sent, &request))

	// The version travels in the body of the request.
	r.Equal("web", request.JobID)
	r.Equal(uint64(2), request.JobVersion)

	// The namespace travels with it, the way it does with every call.
	r.Equal("/v1/job/web/revert", asked.URL.Path)
	r.Equal("production", asked.URL.Query().Get("namespace"))
}
