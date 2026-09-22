package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// drain runs the commands a model asks for and feeds the answers back, the
// way the program loop does.
func drain(m Model, cmd tea.Cmd) Model {
	if cmd == nil {
		return m
	}

	msg := cmd()

	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = drain(m, c)
		}

		return m
	}

	m, _ = m.update(msg)

	return m
}

func TestClusterToScreen(t *testing.T) {
	r := require.New(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch req.URL.Path {
		case "/v1/jobs":
			_, _ = w.Write([]byte(`[{
				"ID": "pelmeni_buh_bot",
				"Name": "pelmeni_buh_bot",
				"Namespace": "production",
				"Type": "service",
				"Status": "running",
				"SubmitTime": 1758499200000000000,
				"JobSummary": {"Summary": {"bot": {"Running": 1}}}
			}]`))
		case "/v1/agent/self":
			_, _ = w.Write([]byte(`{"member": {"Tags": {"build": "1.11.1"}}}`))
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()

	client, err := nomad.New(nomad.Config{Address: server.URL})
	r.NoError(err)

	m := New(client, Options{Namespace: "production", Version: "v-test"})
	m, _ = m.update(tea.WindowSizeMsg{Width: 120, Height: 30})

	// Everything the first screen shows comes over HTTP from the cluster.
	m = drain(m, m.Init())

	out := plain(m.render())

	r.Contains(out, "Jobs (production) [1]")
	r.Contains(out, "pelmeni_buh_bot")
	r.Contains(out, "1/1")
	r.Contains(out, "1.11.1")
	r.Contains(out, server.URL)
}
