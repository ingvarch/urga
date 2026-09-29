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

func TestReason_ErrorsTheClusterListed(t *testing.T) {
	r := require.New(t)

	// The cluster writes several errors as a list under a count: a status
	// line shows one line, and the count alone says nothing.
	_, err := refusing(t, http.StatusInternalServerError,
		"1 error occurred:\n\t* namespace \"ml\" has variables associated with it in regions: [global]\n\n").Jobs(context.Background(), "default")
	r.Equal(`namespace "ml" has variables associated with it in regions: [global]`, nomad.Reason(err))

	_, err = refusing(t, http.StatusInternalServerError, "2 errors occurred:\n\t* first\n\t* second\n\n").Jobs(context.Background(), "default")
	r.Equal("first; second", nomad.Reason(err))

	// Lines of another kind are what the cluster said, as it said it.
	_, err = refusing(t, http.StatusInternalServerError, "the job is not valid:\n* count must be positive").Jobs(context.Background(), "default")
	r.Equal("the job is not valid:\n* count must be positive", nomad.Reason(err))
}
