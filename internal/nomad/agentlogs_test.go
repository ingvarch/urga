package nomad_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

const (
	monitorPath = "/v1/agent/monitor"
	exportPath  = "/v1/agent/monitor/export"
)

// asks records what the agent endpoints were asked.
type asks struct {
	mu    sync.Mutex
	query map[string]url.Values
	hits  map[string]int
}

func (a *asks) note(req *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.query[req.URL.Path] = req.URL.Query()
	a.hits[req.URL.Path]++
}

func (a *asks) of(path string) url.Values {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.query[path]
}

func (a *asks) hit(path string) int {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.hits[path]
}

// agentServer answers the two endpoints of an agent log with the handlers.
func agentServer(t *testing.T, export, monitor http.HandlerFunc) (*nomad.Client, *asks) {
	t.Helper()

	got := &asks{query: map[string]url.Values{}, hits: map[string]int{}}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		got.note(req)

		switch req.URL.Path {
		case exportPath:
			export(w, req)
		case monitorPath:
			monitor(w, req)
		default:
			http.NotFound(w, req)
		}
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	require.NoError(t, err)

	return client, got
}

// sending answers with the texts, one frame each.
func sending(texts ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		out := json.NewEncoder(w)
		for _, text := range texts {
			_ = out.Encode(frame{Data: []byte(text)})
		}
	}
}

// cutting sends the texts, one frame each, and then the start of a frame it
// never ends, the way an answer breaks off when an agent restarts. The pause
// lets the frames be read before the break.
func cutting(texts ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		sending(texts...)(w, req)
		w.(http.Flusher).Flush()

		time.Sleep(20 * time.Millisecond)

		_, _ = io.WriteString(w, `{"Data":"`)
	}
}

func answeringWith(status int, message string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { http.Error(w, message, status) }
}

// beating sends heartbeats until the request ends, then reports it.
func beating(ended chan<- struct{}) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		for {
			select {
			case <-req.Context().Done():
				close(ended)

				return
			case <-time.After(10 * time.Millisecond):
				_, _ = fmt.Fprintln(w, "{}")
				w.(http.Flusher).Flush()
			}
		}
	}
}

func (a *asks) waitFor(t *testing.T, path string) {
	t.Helper()

	require.Eventually(t, func() bool { return a.hit(path) > 0 }, 2*time.Second, time.Millisecond, "%s was not asked", path)
}

func TestAgentLogs_AskTheAgentAtTheLevel(t *testing.T) {
	r := require.New(t)

	client, got := agentServer(t, answeringWith(http.StatusNotFound, ""), sending("a [DEBUG]  one\n", "a [DEBUG]  two\n"))

	stream := client.AgentLogs(t.Context(), nomad.AgentTarget{}, "debug")
	defer stream.Close()

	r.Equal("a [DEBUG]  one\na [DEBUG]  two\n", readAll(t, stream))

	query := got.of(monitorPath)
	r.Equal("debug", query.Get("log_level"))
	r.False(query.Has("node_id"))
	r.False(query.Has("server_id"))
}

func TestAgentLogs_AskThroughTheClusterForAClientOrAServer(t *testing.T) {
	r := require.New(t)

	client, got := agentServer(t, sending("a [INFO]  old\n"), sending("a [INFO]  new\n"))

	stream := client.AgentLogs(t.Context(), nomad.AgentTarget{NodeID: "n1"}, "info")
	readAll(t, stream)
	stream.Close()

	r.Equal("n1", got.of(monitorPath).Get("node_id"))
	r.Equal("n1", got.of(exportPath).Get("node_id"))

	stream = client.AgentLogs(t.Context(), nomad.AgentTarget{ServerID: "server-01.global"}, "info")
	readAll(t, stream)
	stream.Close()

	r.Equal("server-01.global", got.of(monitorPath).Get("server_id"))
	r.False(got.of(monitorPath).Has("node_id"))
}

func TestAgentLogs_ThePastComesFirst(t *testing.T) {
	r := require.New(t)

	client, got := agentServer(t,
		sending("... [INFO]  old 1\n... [INFO]  old 2\n"),
		sending("... [INFO]  new\n"))

	stream := client.AgentLogs(t.Context(), nomad.AgentTarget{}, "info")
	defer stream.Close()

	r.Equal("... [INFO]  old 1\n... [INFO]  old 2\n... [INFO]  new\n", readAll(t, stream))

	query := got.of(exportPath)
	r.Equal("true", query.Get("on_disk"))
	r.False(query.Has("follow"))
	r.False(query.Has("log_level"))
}

func TestAgentLogs_NoPastAndNoErrorWhereTheClusterHasNone(t *testing.T) {
	for name, export := range map[string]http.HandlerFunc{
		"a cluster before 1.10.4":    answeringWith(http.StatusNotFound, "404 page not found"),
		"an agent without a file":    answeringWith(http.StatusBadRequest, "No nomad log file defined"),
		"a token without the export": answeringWith(http.StatusForbidden, "Permission denied"),
		"an empty log file":          sending(),
		"a read that breaks partway": cutting("a [INFO]  old 1\n", "a [INFO]  old 2\n"),
	} {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			client, _ := agentServer(t, export, sending("a [INFO]  new\n"))

			stream := client.AgentLogs(t.Context(), nomad.AgentTarget{}, "info")
			defer stream.Close()

			r.Equal("a [INFO]  new\n", readAll(t, stream))

			select {
			case err := <-stream.Err:
				t.Fatalf("an error was sent: %v", err)
			case <-time.After(100 * time.Millisecond):
			}
		})
	}
}

func TestAgentLogs_AServerIsNotAskedForItsPast(t *testing.T) {
	r := require.New(t)

	client, got := agentServer(t, sending("a [INFO]  old\n"), sending("a [INFO]  new\n"))

	stream := client.AgentLogs(t.Context(), nomad.AgentTarget{ServerID: "server-01.global"}, "info")
	defer stream.Close()

	r.Equal("a [INFO]  new\n", readAll(t, stream))
	r.Zero(got.hit(exportPath))
}

func TestAgentLogs_ThePastKeepsToTheLevel(t *testing.T) {
	past := strings.Join([]string{
		"no level at the start",
		"a [TRACE]  trace",
		"a [DEBUG]  debug",
		"  continued debug",
		"a [INFO]  info",
		"a [WARN]  warn",
		"  continued warn",
		"a [ERROR]  error",
		`{"@level":"debug","@message":"json debug"}`,
		`{"@level":"error","@message":"json error"}`,
	}, "\n") + "\n"

	tests := map[string]string{
		"warn": "no level at the start\na [WARN]  warn\n  continued warn\na [ERROR]  error\n" +
			`{"@level":"error","@message":"json error"}` + "\n",
		"info": "no level at the start\na [INFO]  info\na [WARN]  warn\n  continued warn\na [ERROR]  error\n" +
			`{"@level":"error","@message":"json error"}` + "\n",
	}

	for level, want := range tests {
		t.Run(level, func(t *testing.T) {
			client, _ := agentServer(t, sending(past), sending())

			stream := client.AgentLogs(t.Context(), nomad.AgentTarget{}, level)
			defer stream.Close()

			require.Equal(t, want, readAll(t, stream))
		})
	}
}

// bigPast is a log file of 3 MiB of numbered lines in the format, in frames
// of 32 KiB, and all of it.
func bigPast(format string) (frames []string, all string) {
	var block, whole strings.Builder

	for n := 0; whole.Len() < 3<<20; n++ {
		line := fmt.Sprintf(format, n)
		block.WriteString(line)
		whole.WriteString(line)

		if block.Len() >= 32<<10 {
			frames = append(frames, block.String())
			block.Reset()
		}
	}

	return append(frames, block.String()), whole.String()
}

// firstChunk is the first text the stream sends. The wait only guards
// against a hang: with the race detector on a slow runner, a past of 3 MiB
// takes a few seconds.
func firstChunk(t *testing.T, stream *nomad.LogStream) string {
	t.Helper()

	select {
	case chunk := <-stream.Lines:
		return chunk
	case <-time.After(10 * time.Second):
		t.Fatal("no past was sent")
	}

	return ""
}

// samePast compares a big past by its first line and its size before the
// whole of it, so a failure does not print a MiB.
func samePast(t *testing.T, want, got string) {
	t.Helper()

	wantFirst, _, _ := strings.Cut(want, "\n")
	gotFirst, _, _ := strings.Cut(got, "\n")

	require.Equal(t, wantFirst, gotFirst)
	require.Equal(t, len(want), len(got))
	require.True(t, want == got, "the past differs")
}

func TestAgentLogs_ThePastIsItsLastMiB(t *testing.T) {
	// A line of 20 bytes is cut where the last MiB starts.
	frames, all := bigPast("[INFO]  line %06d\n")

	_, want, _ := strings.Cut(all[len(all)-1<<20:], "\n")

	client, _ := agentServer(t, sending(frames...), sending())

	stream := client.AgentLogs(t.Context(), nomad.AgentTarget{}, "info")
	defer stream.Close()

	samePast(t, want, firstChunk(t, stream))
}

func TestAgentLogs_ThePastKeepsALineThatStartsItsLastMiB(t *testing.T) {
	// Lines of 16 bytes fill the last MiB whole.
	frames, all := bigPast("[INFO]  l%06d\n")

	client, _ := agentServer(t, sending(frames...), sending())

	stream := client.AgentLogs(t.Context(), nomad.AgentTarget{}, "info")
	defer stream.Close()

	samePast(t, all[len(all)-1<<20:], firstChunk(t, stream))
}

func TestAgentLogs_ThePastEndsWithALineBreak(t *testing.T) {
	client, _ := agentServer(t, sending("a [INFO]  old 1\na [INFO]  old 2"), sending("a [INFO]  new\n"))

	stream := client.AgentLogs(t.Context(), nomad.AgentTarget{}, "info")
	defer stream.Close()

	require.Equal(t, "a [INFO]  old 1\na [INFO]  old 2\na [INFO]  new\n", readAll(t, stream))
}

func TestAgentLogs_ARefusalIsTheErrorOfTheStream(t *testing.T) {
	r := require.New(t)

	client, _ := agentServer(t, answeringWith(http.StatusForbidden, "Permission denied"), answeringWith(http.StatusForbidden, "Permission denied"))

	stream := client.AgentLogs(t.Context(), nomad.AgentTarget{}, "info")

	select {
	case err := <-stream.Err:
		r.True(nomad.Forbidden(err))
	case <-time.After(2 * time.Second):
		t.Fatal("no error was sent")
	}

	// The stream ends after its error, with no line and no second error.
	r.Empty(readAll(t, stream))

	select {
	case err := <-stream.Err:
		t.Fatalf("a second error was sent: %v", err)
	default:
	}

	stream.Close()
}

func TestAgentLogs_ABreakOfTheLiveLogIsItsErrorAfterItsLines(t *testing.T) {
	r := require.New(t)

	client, _ := agentServer(t, answeringWith(http.StatusNotFound, ""), cutting("a [INFO]  last\n"))

	stream := client.AgentLogs(t.Context(), nomad.AgentTarget{}, "info")
	defer stream.Close()

	// The break is read while the last line waits to be taken: a reader of
	// both would otherwise take the error and miss the line.
	select {
	case err := <-stream.Err:
		t.Fatalf("the error came before the last line: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	r.Equal("a [INFO]  last\n", readAll(t, stream))

	// The error is there by the time the lines end, so a reader that takes
	// the end can still tell a break from it.
	select {
	case err := <-stream.Err:
		r.ErrorIs(err, io.ErrUnexpectedEOF)
	default:
		t.Fatal("the lines ended before the error")
	}
}

func TestAgentLogs_CloseEndsTheLiveRequest(t *testing.T) {
	ended := make(chan struct{})

	client, got := agentServer(t, answeringWith(http.StatusNotFound, ""), beating(ended))

	stream := client.AgentLogs(t.Context(), nomad.AgentTarget{}, "info")

	got.waitFor(t, monitorPath)
	stream.Close()

	select {
	case <-ended:
	case <-time.After(2 * time.Second):
		t.Fatal("the request stayed open")
	}

	readAll(t, stream)
}

func TestAgentLogs_CloseWhileThePastIsReadAsksNoLiveLog(t *testing.T) {
	r := require.New(t)

	ended := make(chan struct{})

	client, got := agentServer(t, beating(ended), sending("a [INFO]  new\n"))

	stream := client.AgentLogs(t.Context(), nomad.AgentTarget{}, "info")

	got.waitFor(t, exportPath)
	stream.Close()

	select {
	case <-ended:
	case <-time.After(2 * time.Second):
		t.Fatal("the export stayed open")
	}

	readAll(t, stream)
	r.Zero(got.hit(monitorPath))
}
