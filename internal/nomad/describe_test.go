package nomad_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestDescribeJob(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{"ID": "web", "Name": "web", "Datacenters": ["dc1"]}`)

	out, err := client.DescribeJob(context.Background(), "production", "web")
	r.NoError(err)

	r.Equal("/v1/job/web", asked.URL.Path)
	r.Equal("production", asked.URL.Query().Get("namespace"))

	// What the cluster knows about it, laid out to be read.
	r.Contains(out, "\"ID\": \"web\"")
	r.Contains(out, "\n")
}

func TestDescribeAllocation(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{"ID": "af1f37df", "TaskGroup": "frontend"}`)

	out, err := client.DescribeAllocation(context.Background(), "production", "af1f37df")
	r.NoError(err)

	r.Equal("/v1/allocation/af1f37df", asked.URL.Path)
	r.Contains(out, "frontend")
}

func TestDescribeDeployment(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{"ID": "dep-1", "Status": "running"}`)

	out, err := client.DescribeDeployment(context.Background(), "production", "dep-1")
	r.NoError(err)

	r.Equal("/v1/deployment/dep-1", asked.URL.Path)
	r.Contains(out, "running")
}

func TestJobSpec_FromWhatWasSubmitted(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{"Source": "job \"web\" {\n  type = \"service\"\n}", "Format": "hcl2"}`)

	out, err := client.JobSpec(context.Background(), "production", "web")
	r.NoError(err)

	// The file the job was submitted with, the way it was written.
	r.Equal("/v1/job/web/submission", asked.URL.Path)
	r.Equal("job \"web\" {\n  type = \"service\"\n}", out)
}

func TestJobSpec_WithoutASource(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `{"ID": "web", "Name": "web"}`)

	out, err := client.JobSpec(context.Background(), "production", "web")

	// A cluster that did not keep the source says so as an error. Handing a
	// sentence back as if it were the job file puts English prose in front
	// of an editor that submits what it is given.
	r.ErrorIs(err, nomad.ErrNoSource)
	r.Empty(out)
}

func TestJobSpec_WhenTheVersionCannotBeRead(t *testing.T) {
	r := require.New(t)

	var askedVersion string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/v1/job/web" {
			http.Error(w, "permission denied", http.StatusForbidden)

			return
		}

		askedVersion = req.URL.Query().Get("version")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Source": "job \"web\" { type = \"batch\" }", "Format": "hcl2"}`))
	}))
	defer server.Close()

	client, err := nomad.New(nomad.Config{Address: server.URL})
	r.NoError(err)

	_, err = client.JobSpec(context.Background(), "production", "web")

	// Not knowing which version runs is not the same as version zero. Asking
	// for the first version ever submitted puts an old file in the editor,
	// and saving it sends that old file back to the cluster as the new one.
	r.Error(err)
	r.Empty(askedVersion)
}
