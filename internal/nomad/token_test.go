package nomad_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestToken(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{"AccessorID": "b1", "Name": "deploy-bot", "Type": "client",
		"Policies": ["readonly"], "ExpirationTime": "2026-09-24T23:27:01Z"}`)

	token, err := client.Token(context.Background())
	r.NoError(err)

	r.Equal("/v1/acl/token/self", asked.URL.Path)
	r.Equal(nomad.Token{
		ID:      "b1",
		Name:    "deploy-bot",
		Type:    "client",
		Expires: time.Date(2026, 9, 24, 23, 27, 1, 0, time.UTC),
	}, token)
}

func TestToken_WithoutOne(t *testing.T) {
	r := require.New(t)

	// Sent no token, the session is anonymous.
	client, _ := recorder(t, `{"AccessorID": "anonymous", "Name": "Anonymous Token", "Type": "client"}`)

	token, err := client.Token(context.Background())
	r.NoError(err)
	r.True(token.Anonymous)
}

func TestToken_ACLsOff(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `{"AccessorID": "acls-disabled", "Name": "ACLs disabled token", "Type": "client"}`)

	token, err := client.Token(context.Background())
	r.NoError(err)
	r.True(token.ACLsOff)
}

func TestToken_Refused(t *testing.T) {
	r := require.New(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Permission denied", http.StatusForbidden)
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	r.NoError(err)

	// A token the cluster does not know, or no longer knows: that is an
	// answer, not a failed request.
	token, err := client.Token(context.Background())
	r.NoError(err)
	r.True(token.Refused)
}

func TestForbidden(t *testing.T) {
	r := require.New(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Permission denied", http.StatusForbidden)
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	r.NoError(err)

	// The token lacks the permission: the cluster answered and refused.
	_, err = client.Jobs(context.Background(), "default")
	r.True(nomad.Forbidden(err))

	r.False(nomad.Forbidden(errors.New("connection refused")))
	r.False(nomad.Forbidden(nil))
}

func TestUnanswered(t *testing.T) {
	r := require.New(t)

	// A proxy in front of the cluster answers 502, 503 or 504 when the
	// cluster behind it does not.
	for status, unanswered := range map[int]bool{
		http.StatusForbidden: false, http.StatusNotFound: false, http.StatusInternalServerError: false,
		http.StatusBadGateway: true, http.StatusServiceUnavailable: true, http.StatusGatewayTimeout: true,
	} {
		_, err := refusing(t, status, http.StatusText(status)).Jobs(context.Background(), "default")
		r.Error(err)
		r.Equal(unanswered, nomad.Unanswered(err), status)
	}

	// Servers that have no leader, as while they start, answer 500 with the
	// text Nomad has for it.
	_, err := refusing(t, http.StatusInternalServerError, "No cluster leader").Jobs(context.Background(), "default")
	r.Error(err)
	r.True(nomad.Unanswered(err))

	// Nothing listens at the address.
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()

	client, err := nomad.New(nomad.Config{Address: server.URL})
	r.NoError(err)

	_, err = client.Jobs(context.Background(), "default")
	r.Error(err)
	r.True(nomad.Unanswered(err))

	r.False(nomad.Unanswered(nil))
}
