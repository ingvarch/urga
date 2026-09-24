package nomad_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// frame is one piece of a log as the cluster streams it: Offset is where the
// piece ends in File.
type frame struct {
	Data   []byte `json:",omitempty"`
	File   string `json:",omitempty"`
	Offset int64  `json:",omitempty"`
}

// logServer answers for the allocation a1, whose task server is in state,
// and streams its log in frames. The node of the allocation is not found, so
// the log is read through the server, as it is when the node cannot be
// reached.
func logServer(t *testing.T, state string, frames ...frame) (*nomad.Client, *url.Values) {
	t.Helper()

	asked := &url.Values{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/v1/allocation/a1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"ID": "a1", "Namespace": "production", "NodeID": "n1",
				"TaskStates": {"server": {"State": %q}}}`, state)

		case "/v1/client/fs/logs/a1":
			*asked = req.URL.Query()

			out := json.NewEncoder(w)
			for _, f := range frames {
				_ = out.Encode(f)
			}

		default:
			http.NotFound(w, req)
		}
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	require.NoError(t, err)

	return client, asked
}

// readAll is what the stream hands over until it ends.
func readAll(t *testing.T, stream *nomad.LogStream) string {
	t.Helper()

	var out strings.Builder

	for {
		select {
		case chunk, ok := <-stream.Lines:
			if !ok {
				return out.String()
			}

			out.WriteString(chunk)

		case <-time.After(2 * time.Second):
			t.Fatal("the stream did not end")
		}
	}
}

func TestLogs_OpenWithTheLastOfWhatWasWritten(t *testing.T) {
	r := require.New(t)

	client, asked := logServer(t, "running")

	stream, err := client.Logs(context.Background(), "production", "a1", "server", nomad.LogStdout)
	r.NoError(err)
	readAll(t, stream)

	// A log opened on its end shows nothing until the task writes again,
	// and a task that crashed writes nothing again. The last of what it
	// wrote is what the screen is opened for.
	r.Equal("end", asked.Get("origin"))
	r.Equal("65536", asked.Get("offset"))
	r.Equal("server", asked.Get("task"))
	r.Equal("stdout", asked.Get("type"))
}

func TestLogs_DropTheFirstLineWhenItIsCut(t *testing.T) {
	r := require.New(t)

	client, _ := logServer(t, "running",
		frame{Data: []byte("of a line\nsecond\nthird\n"), File: "alloc/logs/server.stdout.0", Offset: 70000})

	stream, err := client.Logs(context.Background(), "production", "a1", "server", nomad.LogStdout)
	r.NoError(err)

	// The last 64 KiB start wherever the count lands, mostly in the middle of
	// a line. Half a line reads as a line the task never wrote.
	r.Equal("second\nthird\n", readAll(t, stream))
}

func TestLogs_KeepTheFirstLineOfTheWholeLog(t *testing.T) {
	r := require.New(t)

	client, _ := logServer(t, "running",
		frame{Data: []byte("first\nsecond\n"), File: "alloc/logs/server.stdout.0", Offset: 13})

	stream, err := client.Logs(context.Background(), "production", "a1", "server", nomad.LogStdout)
	r.NoError(err)

	// A log shorter than what is read back starts at its start, and its
	// first line is whole.
	r.Equal("first\nsecond\n", readAll(t, stream))
}

func TestLogs_ACutLineCanSpanFrames(t *testing.T) {
	r := require.New(t)

	client, _ := logServer(t, "running",
		frame{Data: []byte("abc"), File: "alloc/logs/server.stdout.0", Offset: 70003},
		frame{Data: []byte("def\nwhole\n"), File: "alloc/logs/server.stdout.0", Offset: 70013})

	stream, err := client.Logs(context.Background(), "production", "a1", "server", nomad.LogStdout)
	r.NoError(err)

	r.Equal("whole\n", readAll(t, stream))
}

func TestLogs_ARotatedFileStartsWhereTheOneBeforeStopped(t *testing.T) {
	r := require.New(t)

	client, _ := logServer(t, "running",
		frame{Data: []byte("the rest\nwhole\n"), File: "alloc/logs/server.stdout.3", Offset: 15})

	stream, err := client.Logs(context.Background(), "production", "a1", "server", nomad.LogStdout)
	r.NoError(err)

	// A log is turned over into a new file by size, not at the end of a
	// line: the start of any file but the first is the rest of a line.
	r.Equal("whole\n", readAll(t, stream))
}

func TestLogs_FollowATaskThatRuns(t *testing.T) {
	r := require.New(t)

	client, asked := logServer(t, "running")

	stream, err := client.Logs(context.Background(), "production", "a1", "server", nomad.LogStdout)
	r.NoError(err)
	readAll(t, stream)

	r.Equal("true", asked.Get("follow"))
	r.False(stream.Finished)
}

func TestLogs_ReadAFinishedTaskToItsEnd(t *testing.T) {
	r := require.New(t)

	client, asked := logServer(t, "dead")

	stream, err := client.Logs(context.Background(), "production", "a1", "server", nomad.LogStdout)
	r.NoError(err)
	readAll(t, stream)

	// A dead task writes nothing more, and following it never ends: the
	// cluster answers with an empty frame every second for as long as the
	// request stays open.
	r.Equal("false", asked.Get("follow"))
	r.True(stream.Finished)
}
