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

		// The cluster answers at once, before it has an event to send.
		w.(http.Flusher).Flush()

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

	feed, err := client.Feed(context.Background(), "production", nomad.Subject{}, 1)
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

		feed, err := client.Feed(context.Background(), "", nomad.Subject{}, 1)
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

func TestFeed_GoesOnFromAnIndex(t *testing.T) {
	r := require.New(t)

	client, asked := eventServer(t)

	feed, err := client.Feed(context.Background(), "production", nomad.Subject{}, 58)
	r.NoError(err)

	defer feed.Close()

	// What was read before is not read again.
	r.Equal("58", asked.URL.Query().Get("index"))
}

func TestFeed_AsksForTheTopicsOfItsSubject(t *testing.T) {
	r := require.New(t)

	for name, want := range map[string]struct {
		subject nomad.Subject
		topics  []string
	}{
		"namespace":  {nomad.Subject{}, []string{"*"}},
		"job":        {nomad.Subject{JobID: "web"}, []string{"Job:web", "Allocation:web", "Evaluation:web", "Deployment:web"}},
		"allocation": {nomad.Subject{AllocID: "af1f37df"}, []string{"Allocation:af1f37df"}},
	} {
		client, asked := eventServer(t)

		feed, err := client.Feed(context.Background(), "ml", want.subject, 7)
		r.NoError(err, name)

		feed.Close()

		query := asked.URL.Query()
		r.ElementsMatch(want.topics, query["topic"], name)
		r.Equal("ml", query.Get("namespace"), name)
		r.Equal("7", query.Get("index"), name)
	}
}

func TestFeed_KeepsWhatArrivesWhileNobodyReads(t *testing.T) {
	r := require.New(t)

	client, _ := eventServer(t,
		`{"Index": 7, "Events": [{"Topic": "Job", "Type": "JobRegistered", "Key": "web", "Index": 7}]}`,
		`{"Index": 8, "Events": [{"Topic": "Job", "Type": "JobRegistered", "Key": "api", "Index": 8}]}`,
		`{"Index": 9, "Events": [{"Topic": "Job", "Type": "JobRegistered", "Key": "db", "Index": 9}]}`,
	)

	feed, err := client.Feed(context.Background(), "", nomad.Subject{}, 1)
	r.NoError(err)

	defer feed.Close()

	// The events wait for the reader together, so that it takes them in one
	// go rather than one screen each.
	r.Eventually(func() bool { return len(feed.C) == 3 }, 2*time.Second, 10*time.Millisecond)
}

func TestFeed_LeavesOutWhatCameBeforeTheIndex(t *testing.T) {
	r := require.New(t)

	// Asked for an index past its newest event, the cluster sends its
	// newest one again, and then what comes after.
	client, _ := eventServer(t,
		`{"Index": 18, "Events": [{"Topic": "Node", "Type": "NodeRegistration", "Key": "n1", "Index": 18}]}`,
		`{"Index": 19, "Events": [{"Topic": "Job", "Type": "JobRegistered", "Key": "web", "Index": 19}]}`,
	)

	feed, err := client.Feed(context.Background(), "", nomad.Subject{}, 19)
	r.NoError(err)

	defer feed.Close()

	// What was read before is not read twice.
	r.Equal(uint64(19), waitForEvent(t, feed).Index)
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

func TestAtLeast(t *testing.T) {
	for _, c := range []struct {
		version, minimum string
		want             bool
	}{
		{"1.10.0", "1.10.0", true},
		{"1.10.0-beta.1", "1.10.0", true},
		{"2.0.7+ent", "2.0.0", true},
		{"10.0.0", "2.0.0", true},
		{"1.11.3", "1.10.0", true},
		{"1.9.7", "1.10.0", false},
		{"1.11.4+ent", "2.0.0", false},
		{"1.10.0", "1.10.1", false},
		{"", "1.10.0", false},
		{"dev", "1.10.0", false},
		{"1.10", "1.10.0", false},
		{"v1.10.0", "1.10.0", false},
	} {
		require.Equal(t, c.want, nomad.AtLeast(c.version, c.minimum), "%q >= %q", c.version, c.minimum)
	}
}

func TestStreams(t *testing.T) {
	for _, c := range []struct {
		version string
		topics  []string
		want    bool
	}{
		{"", []string{nomad.TopicJob, nomad.TopicAllocation}, true},
		{"1.9.7", []string{nomad.TopicNode}, true},
		{"", []string{nomad.TopicCSIVolume}, false},
		{"dev", []string{nomad.TopicCSIVolume}, false},
		{"1.9.7", []string{nomad.TopicCSIVolume}, false},
		{"1.10.0", []string{nomad.TopicCSIVolume}, true},
		{"1.10.0-beta.1", []string{nomad.TopicHostVolume}, true},
		{"1.11.3", []string{nomad.TopicCSIVolume, nomad.TopicHostVolume, nomad.TopicCSIPlugin}, true},
		{"1.11.4+ent", []string{nomad.TopicVariable}, false},
		{"2.0.0", []string{nomad.TopicVariable}, true},
		{"10.0.0", []string{nomad.TopicVariable}, true},
		{"1.11.3", []string{nomad.TopicCSIPlugin, nomad.TopicVariable}, false},
		{"1.11.3", []string{nomad.TopicAllocation, nomad.TopicCSIVolume}, true},
	} {
		require.Equal(t, c.want, nomad.Streams(c.version, c.topics), "%q %v", c.version, c.topics)
	}
}

func TestFeed_NamesVolumesPluginsAndVariables(t *testing.T) {
	r := require.New(t)

	for _, c := range []struct {
		name, topic, payload, key string
		want                      nomad.Event
	}{
		{
			"csi volume", "CSIVolume",
			`{"Volume": {"ID": "v1", "Name": "data", "Namespace": "ml", "Schedulable": false, "ModifyTime": 1790671900000000000}}`, "v1",
			nomad.Event{Key: "v1", Name: "data", Namespace: "ml", State: "unschedulable"},
		},
		{
			"host volume", "HostVolume",
			`{"Volume": {"ID": "h1", "Name": "cache", "Namespace": "ml", "State": "ready", "ModifyTime": 1790671900000000000}}`, "h1",
			nomad.Event{Key: "h1", Name: "cache", Namespace: "ml", State: "ready"},
		},
		{
			"deleted host volume", "HostVolume",
			`{"Volume": {"ID": "h2", "Name": "cache", "Namespace": "ml", "State": "ready", "ModifyTime": 1790671900000000000}}`, "",
			nomad.Event{Key: "h2", Name: "cache", Namespace: "ml", State: "ready"},
		},
		{
			"plugin", "CSIPlugin",
			`{"Plugin": {"ID": "ebs", "ModifyTime": 1790671900000000000}}`, "ebs",
			nomad.Event{Key: "ebs", Name: "ebs"},
		},
		{
			"variable", "Variable",
			`{"Metadata": {"Path": "nomad/jobs/web", "Namespace": "ml", "ModifyTime": 1790671900000000000}}`, "nomad/jobs/web",
			nomad.Event{Key: "nomad/jobs/web", Name: "nomad/jobs/web", Namespace: "ml"},
		},
	} {
		client, _ := eventServer(t, `{"Index": 3, "Events": [{"Topic": "`+c.topic+`", "Type": "Updated", "Key": "`+c.key+`", "Index": 3, "Payload": `+c.payload+`}]}`)

		feed, err := client.Feed(context.Background(), "", nomad.Subject{}, 1)
		r.NoError(err, c.name)

		event := waitForEvent(t, feed)
		feed.Close()

		r.Equal(c.want.Key, event.Key, c.name)
		r.Equal(c.want.Name, event.Name, c.name)
		r.Equal(c.want.Namespace, event.Namespace, c.name)
		r.Equal(c.want.State, event.State, c.name)
		r.WithinDuration(time.Unix(0, 1790671900000000000), event.At, time.Microsecond, c.name)
	}
}
