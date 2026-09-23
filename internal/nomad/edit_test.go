package nomad_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubmitJob_JSON(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{"EvalID": "eval-1"}`)

	r.NoError(client.SubmitJob(context.Background(), "production", `{"ID": "web", "Name": "web"}`))

	// A file that is already JSON goes straight to the cluster.
	r.Equal(http.MethodPut, asked.Method)
	r.Equal("/v1/jobs", asked.URL.Path)
}

func TestSubmitJob_HCLGoesThroughTheCluster(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{"ID": "web", "Name": "web"}`)

	r.NoError(client.SubmitJob(context.Background(), "production", "job \"web\" {}"))

	// HCL is read by Nomad itself, urga does not parse job files.
	r.Equal("/v1/jobs", asked.URL.Path)
}

func TestSubmitJob_BrokenJSON(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `{}`)

	err := client.SubmitJob(context.Background(), "production", "{not json")

	r.ErrorContains(err, "not valid JSON")
}

func TestNamespaceSpec(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{"Name": "production", "Description": "live"}`)

	out, err := client.NamespaceSpec(context.Background(), "production")
	r.NoError(err)

	r.Equal("/v1/namespace/production", asked.URL.Path)
	r.Contains(out, "\"Description\": \"live\"")
}

func TestSubmitNamespace(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{}`)

	r.NoError(client.SubmitNamespace(context.Background(), `{"Name": "production"}`))

	// A namespace is written to the collection, not to its own path.
	r.Equal("/v1/namespace", asked.URL.Path)
}
