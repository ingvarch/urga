package nomad_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/nomad/api"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// storedVariable is a variable as the cluster answers it.
func storedVariable(t *testing.T, items map[string]string) string {
	t.Helper()

	body, err := json.Marshal(api.Variable{Namespace: "production", Path: "nomad/jobs/web", ModifyIndex: 769, Items: items})
	require.NoError(t, err)

	return string(body)
}

// sentItems are the items a save sent to the cluster.
func sentItems(t *testing.T, sent []byte) map[string]string {
	t.Helper()

	var v api.Variable
	require.NoError(t, json.Unmarshal(sent, &v))

	return v.Items
}

func TestVariableSpec_WritesTheItemsAsTOML(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, storedVariable(t, map[string]string{
		"DB_HOST": "10.0.0.5",
		"CERT":    "-----BEGIN CERTIFICATE-----\nMIIBfake\n-----END CERTIFICATE-----\n",
		"LIMITS":  `{"cpu": 500}`,
		"CONFIG":  "{\n  \"port\": 8080\n}\n",
	}))

	spec, err := client.VariableSpec(context.Background(), "production", "nomad/jobs/web")
	r.NoError(err)

	r.Equal("/v1/var/nomad/jobs/web", asked.URL.Path)
	r.Equal("production", asked.URL.Query().Get("namespace"))
	r.Equal(uint64(769), spec.Index)

	// By key, a value of several lines as the lines it has, and quotes in a
	// value as they are.
	r.Equal(`# Variable nomad/jobs/web in namespace production.
CERT = """
-----BEGIN CERTIFICATE-----
MIIBfake
-----END CERTIFICATE-----
"""
CONFIG = """
{
  "port": 8080
}
"""
DB_HOST = "10.0.0.5"
LIMITS = '{"cpu": 500}'
`, spec.Source)
}

func TestSubmitVariable_SavesWithCheckAndSet(t *testing.T) {
	r := require.New(t)

	client, sent, asked := metaServer(t, `{}`)

	err := client.SubmitVariable(context.Background(), "production", "nomad/jobs/web", "DB_HOST = \"10.0.0.6\"\n", 769)
	r.NoError(err)

	r.Equal(http.MethodPut, asked.Method)
	r.Equal("/v1/var/nomad/jobs/web", asked.URL.Path)
	r.Equal("production", asked.URL.Query().Get("namespace"))
	r.Equal("769", asked.URL.Query().Get("cas"))
	r.Equal(map[string]string{"DB_HOST": "10.0.0.6"}, sentItems(t, *sent))
}

func TestVariableFile_ReadsBackWhatItWrote(t *testing.T) {
	r := require.New(t)

	items := map[string]string{
		"plain":          "10.0.0.5",
		"empty":          "",
		"quotes":         `say "hi"`,
		"json":           `{"a": "b", "c": ""}`,
		"apostrophe":     `it's "quoted"`,
		"quote and bell": "\"\x07",
		"quote and tab":  "a\t\"b\"",
		"backslash":      `C:\temp\new`,
		"tab":            "a\tb",
		"control":        "bell\x07",
		"unicode":        "Grüße ✓",
		"lines":          "one\ntwo\n",
		"no final line":  "one\ntwo",
		"blank lines":    "\n\none\n\n",
		"crlf":           "one\r\ntwo\r\n",
		"quote runs":     "a\n\"\"\"b\"\"\"\n\"",
		"ends in quote":  "a\nb\"",
		"ends in two":    "a\nb\"\"",
		"a.b":            "dotted key",
		"ключ":           "not a bare key",
	}

	client, sent, _ := metaServer(t, storedVariable(t, items))

	spec, err := client.VariableSpec(context.Background(), "production", "nomad/jobs/web")
	r.NoError(err)

	r.NoError(client.SubmitVariable(context.Background(), "production", "nomad/jobs/web", spec.Source, spec.Index))
	r.Equal(items, sentItems(t, *sent))
}

func TestSubmitVariable_RefusesAValueThatIsNotText(t *testing.T) {
	r := require.New(t)

	client, _, asked := metaServer(t, `{}`)

	err := client.SubmitVariable(context.Background(), "production", "nomad/jobs/web", "PORT = 5432\n", 769)
	r.ErrorContains(err, "PORT")
	r.ErrorContains(err, "text in quotes")

	// Nothing reached the cluster.
	r.Empty(asked.Method)
}

func TestSubmitVariable_RefusesAFileThatIsNotTOML(t *testing.T) {
	r := require.New(t)

	client, _, asked := metaServer(t, `{}`)

	err := client.SubmitVariable(context.Background(), "production", "nomad/jobs/web", "DB_HOST = \n", 769)
	r.ErrorContains(err, "not valid TOML")
	r.Empty(asked.Method)
}

// conflicted answers a save with a conflict, the way the cluster does, and
// a read with what the variable is now: nothing when now is empty.
func conflicted(t *testing.T, now string) *nomad.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case req.Method == http.MethodPut:
			// Only a variable changed since carries itself; one deleted or
			// locked comes back empty.
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"Namespace": "production", "Path": "nomad/jobs/web", "ModifyIndex": 0, "Items": null}`))
		case now == "":
			http.Error(w, "variable not found", http.StatusNotFound)
		default:
			_, _ = w.Write([]byte(now))
		}
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	require.NoError(t, err)

	return client
}

// conflictOf saves over a variable read at 769 and is what the cluster
// said about it.
func conflictOf(t *testing.T, client *nomad.Client) *nomad.VariableConflict {
	t.Helper()

	err := client.SubmitVariable(context.Background(), "production", "nomad/jobs/web", "DB_HOST = \"10.0.0.6\"\n", 769)

	var conflict *nomad.VariableConflict
	require.True(t, errors.As(err, &conflict), "%v", err)

	return conflict
}

func TestSubmitVariable_SaysTheVariableChanged(t *testing.T) {
	r := require.New(t)

	client := conflicted(t, `{"Namespace": "production", "Path": "nomad/jobs/web", "ModifyIndex": 800, "Items": {"DB_HOST": "10.0.0.9"}}`)

	conflict := conflictOf(t, client)

	// The index it is at now, which a save that replaces the change gives.
	r.Equal(uint64(800), conflict.Index)
	r.False(conflict.Deleted)
	r.Nil(conflict.Lock)
	r.EqualError(conflict, "variable nomad/jobs/web changed since it was read")
}

func TestSubmitVariable_SaysTheVariableWasDeleted(t *testing.T) {
	r := require.New(t)

	conflict := conflictOf(t, conflicted(t, ""))

	// Index 0 creates it, and only if nobody did meanwhile.
	r.True(conflict.Deleted)
	r.Equal(uint64(0), conflict.Index)
	r.EqualError(conflict, "variable nomad/jobs/web was deleted since it was read")
}

func TestSubmitVariable_SaysTheVariableIsLocked(t *testing.T) {
	r := require.New(t)

	client := conflicted(t, `{"Namespace": "production", "Path": "nomad/jobs/web", "ModifyIndex": 770,
		"Lock": {"ID": "874ae5d0-f47a-afb0-804f-fcdb04a14a0b", "TTL": "30m0s", "LockDelay": "15s"}}`)

	conflict := conflictOf(t, client)

	r.Equal(&nomad.VariableLock{ID: "874ae5d0-f47a-afb0-804f-fcdb04a14a0b", TTL: "30m0s", Delay: "15s"}, conflict.Lock)
	r.EqualError(conflict, "variable nomad/jobs/web is locked: only the holder of lock 874ae5d0-f47a-afb0-804f-fcdb04a14a0b can change it")
}

func TestSubmitVariable_AConflictItCannotReadAgain(t *testing.T) {
	r := require.New(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPut {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"Path": "nomad/jobs/web", "ModifyIndex": 0}`))

			return
		}

		http.Error(w, "no leader", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	r.NoError(err)

	// The save was refused either way; what the read says comes after.
	err = client.SubmitVariable(context.Background(), "production", "nomad/jobs/web", "DB_HOST = \"10.0.0.6\"\n", 769)
	r.ErrorContains(err, "variable nomad/jobs/web changed since it was read, and reading it again failed")
	r.ErrorContains(err, "no leader")
}
