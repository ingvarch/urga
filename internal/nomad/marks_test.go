package nomad_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTagVersion(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{"/v1/job/web/versions/golden/tag": `{}`})

	r.NoError(client.TagVersion(context.Background(), "production", "web", 3, "golden"))

	sent := (*asked)[0]
	r.Equal("production", sent.namespace)
	r.InDelta(3, sent.body["Version"], 0)
}

func TestUntagVersion(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{}`)

	r.NoError(client.UntagVersion(context.Background(), "production", "web", "golden"))

	r.Equal(http.MethodDelete, asked.Method)
	r.Equal("/v1/job/web/versions/golden/tag", asked.URL.Path)
	r.Equal("production", asked.URL.Query().Get("namespace"))
}

func TestSetAllocHealth(t *testing.T) {
	r := require.New(t)

	for healthy, field := range map[bool]string{true: "HealthyAllocationIDs", false: "UnhealthyAllocationIDs"} {
		client, asked := clusterServer(t, map[string]string{"/v1/deployment/allocation-health/dep-1": `{}`})

		r.NoError(client.SetAllocHealth(context.Background(), "production", "dep-1", []string{"a1", "a2"}, healthy))

		sent := (*asked)[0]
		r.Equal("production", sent.namespace)
		r.Equal([]any{"a1", "a2"}, sent.body[field], field)
	}
}

func TestReleaseLock(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{"/v1/var/nomad/jobs/web": `{"Path": "nomad/jobs/web"}`})

	r.NoError(client.ReleaseLock(context.Background(), "production", "nomad/jobs/web", "lock-1"))

	// The lock is released by its ID, which only its holder and urga know.
	sent := (*asked)[0]
	r.Equal("production", sent.namespace)
	r.True(sent.query.Has("lock-release"))
	r.Equal(map[string]any{"ID": "lock-1", "TTL": "", "LockDelay": ""}, sent.body["Lock"])
}
