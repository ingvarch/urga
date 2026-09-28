package nomad_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// fuzzyWeb is what the cluster finds for "web", in every shape of scope it
// answers with.
const fuzzyWeb = `{
	"Matches": {
		"jobs": [{"ID": "web", "Scope": ["default", "web"]}],
		"groups": [{"ID": "frontend", "Scope": ["default", "web"]}],
		"tasks": [{"ID": "server", "Scope": ["default", "web", "frontend"]}],
		"services": [{"ID": "web-http", "Scope": ["default", "web", "frontend"]}],
		"commands": [{"ID": "/bin/web", "Scope": ["default", "web", "frontend", "server"]}],
		"allocs": [{"ID": "web.frontend[0]", "Scope": ["staging", "bb19fb79-0d79-6a6d-97fa-319ec77d542e"]}],
		"nodes": [{"ID": "web-node-01", "Scope": ["03f874f0-dcec-5b67-2d83-791be144dcc4"]}],
		"node_pools": [{"ID": "web-pool", "Scope": ["web-pool"]}],
		"namespaces": [{"ID": "web-team"}],
		"vars": [{"ID": "nomad/jobs/web", "Scope": ["default", "nomad/jobs/web"]}],
		"plugins": [{"ID": "web-csi", "Scope": ["web-csi"]}],
		"host_volumes": [],
		"evals": []
	},
	"Truncations": {"jobs": false, "allocs": true, "evals": false}
}`

func TestFind_EveryKind(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{"/v1/search/fuzzy": fuzzyWeb})

	found, err := client.Find(context.Background(), "web")
	r.NoError(err)

	// Asked in every namespace, of every kind.
	r.Len(*asked, 1)
	r.Equal("*", (*asked)[0].namespace)
	r.Equal("web", (*asked)[0].body["Text"])
	r.Equal("all", (*asked)[0].body["Context"])

	r.Equal([]nomad.Match{
		{Kind: nomad.MatchJob, Name: "web", Namespace: "default", ID: "web"},
		{Kind: nomad.MatchGroup, Name: "frontend", Namespace: "default", JobID: "web", Group: "frontend"},
		{Kind: nomad.MatchTask, Name: "server", Namespace: "default", JobID: "web", Group: "frontend", Task: "server"},
		{Kind: nomad.MatchService, Name: "web-http", Namespace: "default", JobID: "web", Group: "frontend"},
		{Kind: nomad.MatchCommand, Name: "/bin/web", Namespace: "default", JobID: "web", Group: "frontend", Task: "server"},
		{Kind: nomad.MatchAlloc, Name: "web.frontend[0]", Namespace: "staging", ID: "bb19fb79-0d79-6a6d-97fa-319ec77d542e"},
		{Kind: nomad.MatchNode, Name: "web-node-01", ID: "03f874f0-dcec-5b67-2d83-791be144dcc4"},
		{Kind: nomad.MatchPool, Name: "web-pool", ID: "web-pool"},
		{Kind: nomad.MatchNamespace, Name: "web-team", ID: "web-team"},
		{Kind: nomad.MatchVariable, Name: "nomad/jobs/web", Namespace: "default", ID: "nomad/jobs/web"},
		{Kind: nomad.MatchPlugin, Name: "web-csi", ID: "web-csi"},
	}, found.Matches)

	// The kinds the cluster cut short, to say so.
	r.Equal([]string{nomad.MatchAlloc}, found.Truncated)
}

func TestFind_AnIDPrefixAsksForIDsToo(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{
		"/v1/search/fuzzy": `{"Matches": {"allocs": [{"ID": "web.frontend[0]", "Scope": ["staging", "bb19fb79-0d79-6a6d-97fa-319ec77d542e"]}]}}`,
		"/v1/search": `{
			"Matches": {
				"allocs": ["bb19fb79-0d79-6a6d-97fa-319ec77d542e", "bb1a0000-0000-0000-0000-000000000000"],
				"evals": ["bb1c0000-0000-0000-0000-000000000000"],
				"deployment": ["bb1d0000-0000-0000-0000-000000000000"],
				"nodes": ["bb1e0000-0000-0000-0000-000000000000"],
				"jobs": ["bb1-job"],
				"namespaces": ["bb1-team"]
			},
			"Truncations": {"evals": true}
		}`,
	})

	found, err := client.Find(context.Background(), "bb1")
	r.NoError(err)

	r.Len(*asked, 2)
	r.Equal("/v1/search", (*asked)[1].path)
	r.Equal("bb1", (*asked)[1].body["Prefix"])
	r.Equal("*", (*asked)[1].namespace)

	// An ID found both ways is shown once, with the namespace the name
	// search knows. Only what has an ID of its own is taken from the other:
	// names are found by the name search already.
	r.Equal([]nomad.Match{
		{Kind: nomad.MatchAlloc, Name: "web.frontend[0]", Namespace: "staging", ID: "bb19fb79-0d79-6a6d-97fa-319ec77d542e"},
		{Kind: nomad.MatchAlloc, Name: "bb1a0000-0000-0000-0000-000000000000", ID: "bb1a0000-0000-0000-0000-000000000000"},
		{Kind: nomad.MatchEval, Name: "bb1c0000-0000-0000-0000-000000000000", ID: "bb1c0000-0000-0000-0000-000000000000"},
		{Kind: nomad.MatchDeployment, Name: "bb1d0000-0000-0000-0000-000000000000", ID: "bb1d0000-0000-0000-0000-000000000000"},
		{Kind: nomad.MatchNode, Name: "bb1e0000-0000-0000-0000-000000000000", ID: "bb1e0000-0000-0000-0000-000000000000"},
	}, found.Matches)

	r.Equal([]string{nomad.MatchEval}, found.Truncated)
}

func TestFind_AWordIsNotAnID(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{"/v1/search/fuzzy": `{"Matches": {}}`})

	_, err := client.Find(context.Background(), "web")
	r.NoError(err)

	for _, req := range *asked {
		r.NotEqual("/v1/search", req.path)
	}
}

func TestFind_TheReasonOfTheCluster(t *testing.T) {
	r := require.New(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "fuzzy search query must be at least 2 characters, got 1", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	r.NoError(err)

	_, err = client.Find(context.Background(), "w")
	r.ErrorContains(err, "at least 2 characters")
}
