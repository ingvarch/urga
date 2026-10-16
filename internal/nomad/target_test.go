package nomad_test

import (
	"context"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestJobTarget_HCLNamesItsNamespace(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{
		"/v1/jobs/parse": `{"ID": "web", "Name": "web", "Namespace": "staging", "Type": "batch"}`,
	})

	target, err := client.JobTarget(context.Background(), "production", "job \"web\" {}")
	r.NoError(err)

	r.Equal(nomad.JobTarget{ID: "web", Namespace: "staging", Type: "batch"}, target)

	// The file is read as written: a canonical job has a namespace even when
	// the file names none.
	r.Len(*asked, 1)
	r.Equal(false, (*asked)[0].body["Canonicalize"])
}

func TestJobTarget_NoNamespaceInTheFileIsTheSessions(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{"/v1/jobs/parse": `{"ID": "web", "Name": "web"}`})

	target, err := client.JobTarget(context.Background(), "production", "job \"web\" {}")
	r.NoError(err)

	r.Equal("production", target.Namespace)
	r.Equal("production", (*asked)[0].namespace)
}

func TestJobTarget_EveryNamespaceGoesToDefault(t *testing.T) {
	for _, session := range []string{"", nomad.AllNamespaces} {
		r := require.New(t)

		client, asked := clusterServer(t, map[string]string{"/v1/jobs/parse": `{"ID": "web", "Name": "web"}`})

		target, err := client.JobTarget(context.Background(), session, "job \"web\" {}")
		r.NoError(err)

		r.Equal(nomad.DefaultNamespace, target.Namespace)
		r.Equal(nomad.DefaultNamespace, (*asked)[0].namespace)
	}
}

func TestJobTarget_ATypelessJobIsAService(t *testing.T) {
	r := require.New(t)

	client, _ := clusterServer(t, map[string]string{"/v1/jobs/parse": `{"ID": "web", "Name": "web"}`})

	target, err := client.JobTarget(context.Background(), "production", "job \"web\" {}")
	r.NoError(err)

	r.Equal("service", target.Type)
	r.False(target.Launches)
}

func TestJobTarget_AJobThatLaunchesOthers(t *testing.T) {
	for name, answer := range map[string]string{
		"periodic":      `{"ID": "cron", "Type": "batch", "Periodic": {"Enabled": true}}`,
		"parameterized": `{"ID": "cron", "Type": "batch", "ParameterizedJob": {}}`,
	} {
		t.Run(name, func(t *testing.T) {
			client, _ := clusterServer(t, map[string]string{"/v1/jobs/parse": answer})

			target, err := client.JobTarget(context.Background(), "production", "job \"cron\" {}")
			require.NoError(t, err)
			require.True(t, target.Launches)
		})
	}
}

func TestJobTarget_JSONIsReadWithoutTheCluster(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, nil)

	target, err := client.JobTarget(context.Background(), "production",
		`{"Job": {"ID": "web", "Namespace": "staging", "Type": "system"}}`)
	r.NoError(err)

	r.Equal(nomad.JobTarget{ID: "web", Namespace: "staging", Type: "system"}, target)
	r.Empty(*asked)

	target, err = client.JobTarget(context.Background(), "", `{"ID": "web"}`)
	r.NoError(err)
	r.Equal(nomad.DefaultNamespace, target.Namespace)
}

func TestJobTarget_AFileTheClusterDoesNotParse(t *testing.T) {
	r := require.New(t)

	client, _ := clusterServer(t, nil)

	_, err := client.JobTarget(context.Background(), "production", "job {")
	r.ErrorContains(err, "the job was not accepted")

	_, err = client.JobTarget(context.Background(), "production", "{not json")
	r.ErrorContains(err, "the job is not valid JSON")
}

func TestJobTemplate_IsAJobFile(t *testing.T) {
	r := require.New(t)

	file, diags := hclsyntax.ParseConfig([]byte(nomad.JobTemplate), "job.hcl", hcl.InitialPos)
	r.False(diags.HasErrors(), diags.Error())

	blocks := file.Body.(*hclsyntax.Body).Blocks
	r.Len(blocks, 1)
	r.Equal("job", blocks[0].Type)
	r.Equal([]string{"web"}, blocks[0].Labels)
	r.NotContains(nomad.JobTemplate, "#")
}
