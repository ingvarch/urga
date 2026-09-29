package nomad_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// refusing answers every request with a status and a text, as the cluster
// refuses one.
func refusing(t *testing.T, status int, text string) *nomad.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(text))
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	require.NoError(t, err)

	return client
}

func TestReason(t *testing.T) {
	r := require.New(t)

	_, err := refusing(t, http.StatusInternalServerError, "job not found").Jobs(context.Background(), "default")
	r.Error(err)

	// What the cluster said, without the code the client wraps it in.
	r.Equal("job not found", nomad.Reason(err))

	// What urga says around it stays.
	r.Equal("could not start: job not found", nomad.Reason(fmt.Errorf("could not start: %w", err)))

	// Anything else is as it is.
	r.Equal("no editor", nomad.Reason(fmt.Errorf("no editor")))
}

func TestReason_ARefusalWithNothingSaid(t *testing.T) {
	r := require.New(t)

	_, err := refusing(t, http.StatusNotFound, "").Jobs(context.Background(), "default")
	r.Error(err)

	// The code is all there is to say.
	r.Equal(err.Error(), nomad.Reason(err))
}
