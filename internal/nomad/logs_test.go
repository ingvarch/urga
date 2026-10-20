package nomad_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strconv"
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

// logServer answers for the allocation a1, which replaced a0 and whose task
// server is in state, and streams its log in frames. The node of the
// allocation is not found, so the log is read through the server, as it is
// when the node cannot be reached.
func logServer(t *testing.T, state string, frames ...frame) (*nomad.Client, *url.Values) {
	t.Helper()

	asked := &url.Values{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/v1/allocation/a1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"ID": "a1", "Namespace": "production", "NodeID": "n1",
				"PreviousAllocation": "a0", "TaskStates": {"server": {"State": %q}}}`, state)

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

// readAll collects what the stream sends until it ends.
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

	// A log opened at its end shows nothing until the task writes again,
	// and a task that crashed never writes again. The screen is opened to
	// see the last lines it wrote.
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

func TestLogs_KeepTheFirstLineOfALogWhoseFrameHoldsTwoReads(t *testing.T) {
	r := require.New(t)

	// A read that comes before the frame of the one before it is sent goes
	// into that frame, which keeps the Offset of the first read: its data
	// reaches back past the start of the file.
	client, _ := logServer(t, "running",
		frame{Data: []byte("first\nsecond\n"), File: "alloc/logs/server.stdout.0", Offset: 6})

	stream, err := client.Logs(context.Background(), "production", "a1", "server", nomad.LogStdout)
	r.NoError(err)

	r.Equal("first\nsecond\n", readAll(t, stream))
	r.Equal(nomad.LogMark{File: "alloc/logs/server.stdout.0"}, stream.Start())
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

	// A log is rotated into a new file by size, not at the end of a line:
	// the start of any file but the first is the rest of a line.
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

func TestLogs_SayWhichAllocationCameBefore(t *testing.T) {
	r := require.New(t)

	client, _ := logServer(t, "running")

	stream, err := client.Logs(context.Background(), "production", "a1", "server", nomad.LogStdout)
	r.NoError(err)
	readAll(t, stream)

	// A task that crashed was placed again: the new allocation starts with
	// an empty log, and why it crashed is in the one it replaced.
	r.Equal("a0", stream.Previous)
}

// started reads a stream to its end and says where its first line began.
func started(t *testing.T, frames ...frame) nomad.LogMark {
	t.Helper()

	client, _ := logServer(t, "running", frames...)

	stream, err := client.Logs(context.Background(), "production", "a1", "server", nomad.LogStdout)
	require.NoError(t, err)
	readAll(t, stream)

	return stream.Start()
}

func TestLogs_StartIsWhereTheFirstLineBegins(t *testing.T) {
	const log = "alloc/logs/server.stdout."

	tests := []struct {
		name   string
		frames []frame
		want   nomad.LogMark
	}{
		{"a whole log", []frame{{Data: []byte("first\nsecond\n"), File: log + "0", Offset: 13}}, nomad.LogMark{File: log + "0"}},
		{"a cut first line", []frame{{Data: []byte("of a line\nsecond\nthird\n"), File: log + "0", Offset: 70000}}, nomad.LogMark{File: log + "0", Offset: 69987}},
		{"a cut over two frames", []frame{
			{Data: []byte("abc"), File: log + "0", Offset: 70003},
			{Data: []byte("def\nwhole\n"), File: log + "0", Offset: 70013},
		}, nomad.LogMark{File: log + "0", Offset: 70007}},
		{"a rotated file", []frame{{Data: []byte("the rest\nwhole\n"), File: log + "3", Offset: 15}}, nomad.LogMark{File: log + "3", Offset: 9}},
		{"the first frame kept, not a later one", []frame{
			{Data: []byte("tail\nfirst\n"), File: log + "1", Offset: 100},
			{Data: []byte("second\n"), File: log + "1", Offset: 107},
		}, nomad.LogMark{File: log + "1", Offset: 94}},
		{"no frame", nil, nomad.LogMark{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, started(t, tt.frames...))
		})
	}
}

func TestLogMark_AtStartIsTheFirstByteOfTheFirstFile(t *testing.T) {
	r := require.New(t)

	r.True(nomad.LogMark{File: "alloc/logs/server.stdout.0"}.AtStart())
	r.False(nomad.LogMark{File: "alloc/logs/server.stdout.0", Offset: 4}.AtStart())
	r.False(nomad.LogMark{File: "alloc/logs/server.stdout.1"}.AtStart())
	r.False(nomad.LogMark{}.AtStart())
}

// backAsk is one read of a file the server saw.
type backAsk struct {
	path          string
	offset, limit int64
	namespace     string
}

// backServer answers for the files of the allocation a1, by name: it lists
// the directory of its logs and reads a part of a file, and writes down what
// it was asked to read and the namespaces it was asked to list in.
func backServer(t *testing.T, files map[string]string) (*nomad.Client, *[]backAsk, *[]string) {
	t.Helper()

	asked, listedIn := &[]backAsk{}, &[]string{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		query := req.URL.Query()

		switch req.URL.Path {
		case "/v1/client/fs/ls/a1":
			*listedIn = append(*listedIn, query.Get("namespace"))

			var listed []map[string]any

			for name, content := range files {
				if path.Dir(name) == query.Get("path") {
					listed = append(listed, map[string]any{"Name": path.Base(name), "Size": len(content)})
				}
			}

			_ = json.NewEncoder(w).Encode(listed)

		case "/v1/client/fs/readat/a1":
			offset, _ := strconv.ParseInt(query.Get("offset"), 10, 64)
			limit, _ := strconv.ParseInt(query.Get("limit"), 10, 64)
			*asked = append(*asked, backAsk{query.Get("path"), offset, limit, query.Get("namespace")})

			content, ok := files[query.Get("path")]
			if !ok {
				http.NotFound(w, req)

				return
			}

			_, _ = w.Write([]byte(content[offset:min(offset+limit, int64(len(content)))]))

		default:
			http.NotFound(w, req)
		}
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	require.NoError(t, err)

	return client, asked, listedIn
}

// numbered is n lines of 9 bytes each, so a step of 1 MiB lands inside one.
func numbered(n int) string {
	var out strings.Builder

	for i := range n {
		fmt.Fprintf(&out, "%08d\n", i)
	}

	return out.String()
}

const (
	backLog  = "alloc/logs/server.stdout."
	logSized = 1 << 20
)

func TestLogBefore_ReadsOneStepBackAndDropsTheCutLine(t *testing.T) {
	r := require.New(t)

	older, newer := numbered(50_000), numbered(400_000)
	client, asked, listedIn := backServer(t, map[string]string{backLog + "0": older, backLog + "1": newer})

	at := nomad.LogMark{File: backLog + "1", Offset: 9 * 300_000}
	part, err := client.LogBefore(context.Background(), "production", "a1", at, 200_000)
	r.NoError(err)

	// One read, of a step, in the namespace given.
	r.Equal([]backAsk{{backLog + "1", at.Offset - logSized, logSized, "production"}}, *asked)
	r.Equal([]string{"production"}, *listedIn)

	// The step starts inside a line (the 6th byte of line 183491): that line
	// is dropped, and the first one kept is whole.
	r.Len(part.Lines, 299_999-183_492+1)
	r.Equal("00183492", part.Lines[0])
	r.Equal("00299999", part.Lines[len(part.Lines)-1])
	r.Equal(nomad.LogMark{File: backLog + "1", Offset: 9 * 183_492}, part.From)

	// What the cluster keeps before it: all of .0 and what is of .1 before it.
	r.Equal(int64(len(older))+9*183_492, part.Before)
	r.Equal(int64(len(older)+len(newer)), part.Size)
}

func TestLogBefore_JoinsTheFilesWhenTheStepCrossesOne(t *testing.T) {
	r := require.New(t)

	client, asked, _ := backServer(t, map[string]string{
		backLog + "0": "alpha\nbeta\nsplit-",
		backLog + "1": "line-over\nmid\nrest\n",
	})

	part, err := client.LogBefore(context.Background(), "production", "a1", nomad.LogMark{File: backLog + "1", Offset: 14}, 1000)
	r.NoError(err)

	// A line the rotation split is one line again, and the first line of
	// the first file is kept: nothing was cut there.
	r.Equal([]string{"alpha", "beta", "split-line-over", "mid"}, part.Lines)
	r.Equal(nomad.LogMark{File: backLog + "0"}, part.From)
	r.Zero(part.Before)
	r.Equal([]backAsk{{backLog + "1", 0, 14, "production"}, {backLog + "0", 0, 17, "production"}}, *asked)
}

func TestLogBefore_ReadsAPathWrittenWithBackslashes(t *testing.T) {
	r := require.New(t)

	client, asked, _ := backServer(t, map[string]string{
		backLog + "0": "alpha\nbeta\nsplit-",
		backLog + "1": "line-over\nmid\nrest\n",
	})

	// A client on Windows joins the path of a frame with backslashes.
	at := nomad.LogMark{File: `alloc\logs\server.stdout.1`, Offset: 14}
	part, err := client.LogBefore(context.Background(), "production", "a1", at, 1000)
	r.NoError(err)

	r.Equal([]string{"alpha", "beta", "split-line-over", "mid"}, part.Lines)
	r.Equal(nomad.LogMark{File: backLog + "0"}, part.From)
	r.Zero(part.Before)
	r.Equal([]backAsk{{backLog + "1", 0, 14, "production"}, {backLog + "0", 0, 17, "production"}}, *asked)
}

func TestLogBefore_StopsAtTheOldestFileThatIsLeft(t *testing.T) {
	r := require.New(t)

	// .0 and .1 were deleted: .2 starts in the middle of a line.
	client, _, _ := backServer(t, map[string]string{backLog + "2": "cut-off\nwhole\nlines\n"})

	part, err := client.LogBefore(context.Background(), "production", "a1", nomad.LogMark{File: backLog + "2", Offset: 20}, 1000)
	r.NoError(err)

	r.Equal([]string{"whole", "lines"}, part.Lines)
	r.Equal(nomad.LogMark{File: backLog + "2", Offset: 8}, part.From)

	// The cut line is not readable, so there is nothing further back.
	r.Zero(part.Before)
}

func TestLogBefore_ACutLineAloneAtTheOldestFileIsDropped(t *testing.T) {
	r := require.New(t)

	client, _, _ := backServer(t, map[string]string{backLog + "3": "the rest\nwhole\n"})

	part, err := client.LogBefore(context.Background(), "production", "a1", nomad.LogMark{File: backLog + "3", Offset: 9}, 1000)
	r.NoError(err)

	r.Empty(part.Lines)
	r.Zero(part.Before)
}

func TestLogBefore_ReadsNothingFromAFileThatIsGone(t *testing.T) {
	r := require.New(t)

	client, asked, _ := backServer(t, map[string]string{backLog + "2": "whole\n"})

	at := nomad.LogMark{File: backLog + "1", Offset: 10}
	part, err := client.LogBefore(context.Background(), "production", "a1", at, 1000)
	r.NoError(err)

	r.Empty(part.Lines)
	r.Zero(part.Before)
	r.Equal(at, part.From)
	r.Empty(*asked)
}

func TestLogBefore_KeepsTheLastLinesOfALimit(t *testing.T) {
	r := require.New(t)

	older := numbered(50_000)
	client, _, _ := backServer(t, map[string]string{backLog + "0": older, backLog + "1": numbered(400_000)})

	part, err := client.LogBefore(context.Background(), "production", "a1", nomad.LogMark{File: backLog + "1", Offset: 9 * 300_000}, 3)
	r.NoError(err)

	// The lines left out are still to be read: From and Before say so.
	r.Equal([]string{"00299997", "00299998", "00299999"}, part.Lines)
	r.Equal(nomad.LogMark{File: backLog + "1", Offset: 9 * 299_997}, part.From)
	r.Equal(int64(len(older))+9*299_997, part.Before)
}

func TestLogBefore_KeepsALineLongerThanAStepWhole(t *testing.T) {
	r := require.New(t)

	long := strings.Repeat("x", 2<<20)
	client, _, _ := backServer(t, map[string]string{backLog + "1": "head\n" + long + "\n"})

	size := int64(5 + len(long) + 1)
	part, err := client.LogBefore(context.Background(), "production", "a1", nomad.LogMark{File: backLog + "1", Offset: size}, 1000)
	r.NoError(err)

	// A step with no line break in it is one line, as a line longer than
	// the tail of a log is when it opens.
	r.Len(part.Lines, 1)
	r.Len(part.Lines[0], logSized-1)
	r.Equal(nomad.LogMark{File: backLog + "1", Offset: size - logSized}, part.From)
}

func TestLogBefore_AStepThatEndsAtTheStartOfAFileDropsTheLineCutThere(t *testing.T) {
	r := require.New(t)

	older := "aaa\nbb"
	client, asked, _ := backServer(t, map[string]string{
		backLog + "0": older,
		backLog + "1": "cc\n" + strings.Repeat("y", logSized-4) + "\n",
	})

	at := nomad.LogMark{File: backLog + "1", Offset: logSized}
	part, err := client.LogBefore(context.Background(), "production", "a1", at, 1000)
	r.NoError(err)

	// The step is all of .1. Its first line began in .0, which is still
	// there: the next step reads it whole, so it counts as before.
	r.Equal([]backAsk{{backLog + "1", 0, logSized, "production"}}, *asked)
	r.Equal(nomad.LogMark{File: backLog + "1", Offset: 3}, part.From)
	r.Equal(int64(3+len(older)), part.Before)
}

func TestLogBefore_AStepThatEndsAtTheStartOfAFileKeepsALineWithNoBreakWhole(t *testing.T) {
	r := require.New(t)

	client, _, _ := backServer(t, map[string]string{
		backLog + "0": "aaa\nbb",
		backLog + "1": strings.Repeat("y", logSized-1) + "\n",
	})

	part, err := client.LogBefore(context.Background(), "production", "a1", nomad.LogMark{File: backLog + "1", Offset: logSized}, 1000)
	r.NoError(err)

	// A file before it is still there, so the break at the end of the step
	// ends the one line in it, which is kept.
	r.Len(part.Lines, 1)
	r.Positive(part.Before)
}
