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

func TestFeed_ReadsWhatHappened(t *testing.T) {
	r := require.New(t)

	client, asked := eventServer(t,
		`{"Index": 11, "Events": [
			{"Topic": "Allocation", "Type": "AllocationUpdated", "Key": "af1f37df", "Namespace": "production", "Index": 11,
				"Payload": {"Allocation": {"ID": "af1f37df", "Name": "web.frontend[0]", "Namespace": "production",
					"ClientStatus": "failed", "ModifyTime": 1790671937831483000}}},
			{"Topic": "Node", "Type": "NodeDrain", "Key": "n1", "Namespace": "", "Index": 11,
				"Payload": {"Node": {"ID": "n1", "Name": "client-01", "Status": "ready"}}}
		]}`,
		`{"Index": 12, "Events": [
			{"Topic": "ACLToken", "Type": "ACLTokenUpserted", "Key": "a1", "Index": 12}
		]}`,
	)

	feed, err := client.Feed(context.Background(), "production")
	r.NoError(err)

	defer feed.Close()

	// Every topic of the namespace the session looks at, from the oldest
	// event the cluster still keeps.
	query := asked.URL.Query()
	r.Equal([]string{"*"}, query["topic"])
	r.Equal("production", query.Get("namespace"))
	r.Equal("1", query.Get("index"))

	// What each event names, and the state and time the object has.
	alloc := waitForEvent(t, feed)
	r.WithinDuration(time.Unix(0, 1790671937831483000), alloc.At, time.Microsecond)
	alloc.At = time.Time{}
	r.Equal(nomad.Event{
		Index: 11, Topic: "Allocation", Type: "AllocationUpdated", Namespace: "production", Key: "af1f37df",
		Name: "web.frontend[0]", State: "failed",
	}, alloc)

	node := waitForEvent(t, feed)
	r.Equal("client-01", node.Name)
	r.Equal("ready", node.State)
	r.True(node.At.IsZero())

	// A topic urga reads nothing of is named by its key.
	token := waitForEvent(t, feed)
	r.Equal("ACLToken", token.Topic)
	r.Equal("a1", token.Name)
}

func TestFeed_NamesEachKindOfObject(t *testing.T) {
	r := require.New(t)

	for topic, want := range map[string]struct{ payload, name, state string }{
		"Job": {`{"Job": {"ID": "web", "Namespace": "ml", "Status": "running", "SubmitTime": 1790671900000000000}}`, "web", "running"},
		"Deployment": {
			`{"Deployment": {"ID": "d1", "Namespace": "ml", "JobID": "web", "Status": "failed", "ModifyTime": 1790671900000000000}}`,
			"web", "failed",
		},
		"Evaluation": {
			`{"Evaluation": {"ID": "e1", "Namespace": "ml", "JobID": "web", "Status": "blocked", "ModifyTime": 1790671900000000000}}`,
			"web", "blocked",
		},
		"NodePool": {`{"NodePool": {"Name": "gpu"}}`, "gpu", ""},
		"Service":  {`{"Service": {"ID": "s1", "Namespace": "ml", "ServiceName": "web-http"}}`, "web-http", ""},
	} {
		client, _ := eventServer(t, `{"Index": 3, "Events": [{"Topic": "`+topic+`", "Type": "Updated", "Key": "k1", "Index": 3, "Payload": `+want.payload+`}]}`)

		feed, err := client.Feed(context.Background(), "")
		r.NoError(err, topic)

		event := waitForEvent(t, feed)
		feed.Close()

		r.Equal(want.name, event.Name, topic)
		r.Equal(want.state, event.State, topic)

		// The namespace of the object; a pool belongs to none.
		if topic != "NodePool" {
			r.Equal("ml", event.Namespace, topic)
		}

		// The time the object changed, where it keeps one.
		if topic == "NodePool" || topic == "Service" {
			r.True(event.At.IsZero(), topic)
		} else {
			r.WithinDuration(time.Unix(0, 1790671900000000000), event.At, time.Microsecond, topic)
		}
	}
}

// waitForEvent takes the next event of a feed, or fails rather than hanging.
func waitForEvent(t *testing.T, feed *nomad.Feed) nomad.Event {
	t.Helper()

	select {
	case event := <-feed.C:
		return event
	case err := <-feed.Err:
		t.Fatalf("the feed ended: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("nothing arrived")
	}

	return nomad.Event{}
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
