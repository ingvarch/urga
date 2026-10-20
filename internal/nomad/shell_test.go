package nomad_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/nomad/api"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestRunCommand_RunsWithoutATerminalAndKeepsWhatItPrintedInOrder(t *testing.T) {
	r := require.New(t)

	var (
		query url.Values
		first api.ExecStreamingInput
	)

	upgrader := websocket.Upgrader{}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/allocation/alloc-1", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ID":"alloc-1","NodeID":"node-1","Namespace":"production"}`))
	})
	mux.HandleFunc("/v1/client/allocation/alloc-1/exec", func(w http.ResponseWriter, req *http.Request) {
		query = req.URL.Query()

		conn, err := upgrader.Upgrade(w, req, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		_ = conn.ReadJSON(&first)

		out := func(frame api.ExecStreamingOutput) { _ = conn.WriteJSON(frame) }
		out(api.ExecStreamingOutput{Stdout: &api.ExecStreamingIOOperation{Data: []byte("one\n")}})
		out(api.ExecStreamingOutput{Stderr: &api.ExecStreamingIOOperation{Data: []byte("two\n")}})
		out(api.ExecStreamingOutput{Stdout: &api.ExecStreamingIOOperation{Data: []byte("three\n")}})
		out(api.ExecStreamingOutput{Exited: true, Result: &api.ExecStreamingExitResult{ExitCode: 3}})
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	r.NoError(err)

	got, err := client.RunCommand(context.Background(), "production", "alloc-1", "server", []string{"/bin/sh", "-c", "env"})
	r.NoError(err)

	r.Equal(nomad.CommandOutput{Text: "one\ntwo\nthree\n", ExitCode: 3, Size: 14}, got)

	r.Equal("false", query.Get("tty"))
	r.Equal("server", query.Get("task"))
	r.Equal(`["/bin/sh","-c","env"]`, query.Get("command"))
	r.Equal("production", query.Get("namespace"))

	// Nothing is typed into the command: its stdin is closed at once.
	r.NotNil(first.Stdin)
	r.True(first.Stdin.Close)
	r.Empty(first.Stdin.Data)
}

func TestRunCommand_ReturnsNoOutputOnAnError(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `{}`)

	// The recorder answers every path with an object and speaks no websocket.
	got, err := client.RunCommand(context.Background(), "production", "alloc-1", "server", []string{"env"})

	r.Error(err)
	r.Equal(nomad.CommandOutput{}, got)
}

func TestRunCommand_KeepsTheWholeLinesOfTheLastMiBOfALongOutput(t *testing.T) {
	r := require.New(t)

	var printed strings.Builder
	for i := range 200_000 {
		fmt.Fprintf(&printed, "line %06d\n", i)
	}

	client := printing(t, printed.String())

	got, err := client.RunCommand(context.Background(), "production", "alloc-1", "server", []string{"yes"})
	r.NoError(err)

	// 1 MiB holds 87381 lines of 12 bytes and the end of one more, which is
	// cut and dropped.
	all := printed.String()
	r.Equal(all[len(all)-87381*12:], got.Text)
	r.True(strings.HasSuffix(got.Text, "line 199999\n"))
	r.Equal(int64(len(all)), got.Size)
	r.Equal(int64(len(all)-len(got.Text)), got.From)
}

func TestRunCommand_KeepsTheLineThatStartsWhereTheLastMiBStarts(t *testing.T) {
	r := require.New(t)

	var printed strings.Builder
	for i := range 100_000 {
		fmt.Fprintf(&printed, "line %010d\n", i)
	}

	got, err := printing(t, printed.String()).RunCommand(context.Background(), "production", "alloc-1", "server", []string{"yes"})
	r.NoError(err)

	// 1 MiB holds 65536 lines of 16 bytes, the first of them whole.
	all := printed.String()
	r.Equal(all[len(all)-65536*16:], got.Text)
	r.Equal(int64(len(all)-1<<20), got.From)
}

func TestRunCommand_KeepsTheLastMiBOfALineLongerThanThat(t *testing.T) {
	r := require.New(t)

	all := strings.Repeat("x", 3<<19) + "\n"

	got, err := printing(t, all).RunCommand(context.Background(), "production", "alloc-1", "server", []string{"yes"})
	r.NoError(err)

	r.Equal(all[len(all)-1<<20:], got.Text)
	r.Equal(int64(len(all)), got.Size)
	r.Equal(int64(len(all)-1<<20), got.From)
}

// printing is a client whose task prints the text to stdout in small frames
// and exits with 0.
func printing(t *testing.T, text string) *nomad.Client {
	t.Helper()

	upgrader := websocket.Upgrader{}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/allocation/alloc-1", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ID":"alloc-1","NodeID":"node-1","Namespace":"production"}`))
	})
	mux.HandleFunc("/v1/client/allocation/alloc-1/exec", func(w http.ResponseWriter, req *http.Request) {
		conn, err := upgrader.Upgrade(w, req, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		var first api.ExecStreamingInput
		_ = conn.ReadJSON(&first)

		for chunk := range slices.Chunk([]byte(text), 4096) {
			_ = conn.WriteJSON(api.ExecStreamingOutput{Stdout: &api.ExecStreamingIOOperation{Data: chunk}})
		}

		_ = conn.WriteJSON(api.ExecStreamingOutput{Exited: true, Result: &api.ExecStreamingExitResult{ExitCode: 0}})
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	require.NoError(t, err)

	return client
}
