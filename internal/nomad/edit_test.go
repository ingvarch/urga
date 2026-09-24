package nomad_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// sent is one request the cluster received.
type sent struct {
	path      string
	namespace string
	body      map[string]any
}

// jobServer answers each path with a body of its own and keeps every request
// in the order it came.
func jobServer(t *testing.T, answers map[string]string) (*nomad.Client, *[]sent) {
	t.Helper()

	var asked []sent

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(req.Body).Decode(&body)

		asked = append(asked, sent{path: req.URL.Path, namespace: req.URL.Query().Get("namespace"), body: body})

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(answers[req.URL.Path]))
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	require.NoError(t, err)

	return client, &asked
}

// submission is what a register request says the job was written as.
func submission(t *testing.T, register sent) map[string]any {
	t.Helper()

	require.Equal(t, "/v1/jobs", register.path)

	out, ok := register.body["Submission"].(map[string]any)
	require.True(t, ok, "the register request carries no submission")

	return out
}

func TestSubmitJob_JSON(t *testing.T) {
	r := require.New(t)

	client, asked := jobServer(t, map[string]string{"/v1/jobs": `{"EvalID": "eval-1"}`})

	source := `{"ID": "web", "Name": "web"}`
	r.NoError(client.SubmitJob(context.Background(), "production", source, nomad.JobVariables{}))

	// A file that is already JSON goes straight to the cluster, and is kept
	// with the version it makes: the next edit opens it again.
	r.Len(*asked, 1)

	kept := submission(t, (*asked)[0])
	r.Equal(source, kept["Source"])
	r.Equal("json", kept["Format"])
}

func TestSubmitJob_HCLGoesThroughTheCluster(t *testing.T) {
	r := require.New(t)

	client, asked := jobServer(t, map[string]string{
		"/v1/jobs/parse": `{"ID": "web", "Name": "web"}`,
		"/v1/jobs":       `{"EvalID": "eval-1"}`,
	})

	r.NoError(client.SubmitJob(context.Background(), "production", "job \"web\" {}", nomad.JobVariables{}))

	// HCL is read by Nomad itself, urga does not parse job files.
	r.Len(*asked, 2)
	r.Equal("/v1/jobs/parse", (*asked)[0].path)
	r.Equal("/v1/jobs", (*asked)[1].path)
}

func TestSubmitJob_ParsesInTheNamespace(t *testing.T) {
	r := require.New(t)

	client, asked := jobServer(t, map[string]string{
		"/v1/jobs/parse": `{"ID": "web", "Name": "web"}`,
		"/v1/jobs":       `{"EvalID": "eval-1"}`,
	})

	r.NoError(client.SubmitJob(context.Background(), "production", "job \"web\" {}", nomad.JobVariables{}))

	// A token may parse jobs in its own namespace only. Asking without one
	// asks in the default namespace, where it is refused.
	r.Equal("production", (*asked)[0].namespace)
}

func TestSubmitJob_HCLKeepsTheSourceAndVariables(t *testing.T) {
	r := require.New(t)

	client, asked := jobServer(t, map[string]string{
		"/v1/jobs/parse": `{"ID": "web", "Name": "web"}`,
		"/v1/jobs":       `{"EvalID": "eval-1"}`,
	})

	source := "variable \"image\" {}\njob \"web\" {}"
	vars := nomad.JobVariables{
		Flags: map[string]string{"image": "nginx:1.27"},
		File:  "count = 3\n",
	}

	r.NoError(client.SubmitJob(context.Background(), "production", source, vars))

	// The cluster reads variables only as a file, so the flags are written
	// into one next to the file the job was run with.
	parsed := (*asked)[0].body["Variables"]
	r.Contains(parsed, `image = "nginx:1.27"`)
	r.Contains(parsed, "count = 3")

	// The version keeps what it was made of, the way it was given.
	kept := submission(t, (*asked)[1])
	r.Equal(source, kept["Source"])
	r.Equal("hcl2", kept["Format"])
	r.Equal(map[string]any{"image": "nginx:1.27"}, kept["VariableFlags"])
	r.Equal("count = 3\n", kept["Variables"])
}

func TestSubmitJob_FlagValuesStayText(t *testing.T) {
	r := require.New(t)

	client, asked := jobServer(t, map[string]string{
		"/v1/jobs/parse": `{"ID": "web", "Name": "web"}`,
		"/v1/jobs":       `{"EvalID": "eval-1"}`,
	})

	vars := nomad.JobVariables{Flags: map[string]string{"greeting": "say \"hi\"\n${name} %{ok}\\"}}

	r.NoError(client.SubmitJob(context.Background(), "production", "job \"web\" {}", vars))

	// A flag was typed as text. Written into HCL as it is, a quote ends the
	// string early and ${ reads another variable instead of the characters.
	r.Equal(`greeting = "say \"hi\"\n$${name} %%{ok}\\"`+"\n", (*asked)[0].body["Variables"])
}

func TestSubmitJob_BrokenJSON(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `{}`)

	err := client.SubmitJob(context.Background(), "production", "{not json", nomad.JobVariables{})

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
