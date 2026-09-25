package nomad_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// eventServer answers the event stream with the lines it is given, one
// after another, and records the request it received.
func eventServer(t *testing.T, lines ...string) (*nomad.Client, *http.Request) {
	t.Helper()

	asked := &http.Request{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		*asked = *req

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		for _, line := range lines {
			_, _ = w.Write([]byte(line + "\n"))
			w.(http.Flusher).Flush()
		}

		<-req.Context().Done()
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	require.NoError(t, err)

	return client, asked
}

func TestEvents_SayWhatChanged(t *testing.T) {
	r := require.New(t)

	client, asked := eventServer(t,
		`{"Index": 7, "Events": [{"Topic": "Job", "Type": "JobRegistered", "Key": "web", "Namespace": "production"}]}`,
		`{"Index": 8, "Events": [{"Topic": "Allocation", "Type": "AllocationUpdated", "Key": "af1f37df", "Namespace": "production"}]}`,
	)

	changes, err := client.Events(context.Background(), "production", []string{"Job", "Allocation"})
	r.NoError(err)

	defer changes.Close()

	// The cluster is asked for the topics the screen cares about, in the
	// namespace it is looking at.
	r.Equal("/v1/event/stream", asked.URL.Path)
	r.ElementsMatch([]string{"Job", "Allocation"}, asked.URL.Query()["topic"])
	r.Equal("production", asked.URL.Query().Get("namespace"))

	first := waitForChange(t, changes)
	r.Equal("Job", first.Topic)
	r.Equal("web", first.Key)

	second := waitForChange(t, changes)
	r.Equal("Allocation", second.Topic)
}

func TestEvents_CloseLetsTheRequestGo(t *testing.T) {
	r := require.New(t)

	client, _ := eventServer(t, `{"Index": 7, "Events": [{"Topic": "Job", "Type": "JobRegistered"}]}`)

	changes, err := client.Events(context.Background(), "", []string{"Job"})
	r.NoError(err)

	waitForChange(t, changes)
	changes.Close()

	// Nothing more arrives, and the channel is closed rather than left to
	// hang.
	select {
	case _, open := <-changes.C:
		r.False(open)
	case <-time.After(time.Second):
		t.Fatal("the stream was not closed")
	}
}

func TestEvents_WhenTheClusterWillNotStream(t *testing.T) {
	r := require.New(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Permission denied", http.StatusForbidden)
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	r.NoError(err)

	// An ACL that does not allow the stream returns an error at once, so
	// that the caller can keep polling the way it did before.
	_, err = client.Events(context.Background(), "", []string{"Job"})
	r.Error(err)
}

// waitForChange takes the next change, or fails rather than hanging.
func waitForChange(t *testing.T, changes *nomad.Changes) nomad.Change {
	t.Helper()

	select {
	case change := <-changes.C:
		return change
	case err := <-changes.Err:
		t.Fatalf("the stream ended: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("nothing arrived")
	}

	return nomad.Change{}
}
