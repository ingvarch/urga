package nomad_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

const identityClaims = `{"Claims": {
	"exp": 1792454400, "iat": 1792368000, "iss": "nomad",
	"nomad_node_id": "node-1", "nomad_node_pool": "default"
}}`

func TestNodeIdentity_ReadsTheExpiryAndKeepsTheClaims(t *testing.T) {
	r := require.New(t)

	client, _, asked := metaServer(t, identityClaims)

	identity, err := client.NodeIdentity(context.Background(), "node-1")
	r.NoError(err)

	r.Equal("/v1/client/identity", asked.URL.Path)
	r.Equal("node-1", asked.URL.Query().Get("node_id"))
	r.Empty(asked.URL.Query().Get("namespace"))

	r.True(time.Unix(1792454400, 0).Equal(identity.Expires))
	r.Contains(identity.Claims, `"nomad_node_id": "node-1"`)
}

func TestNodeIdentity_WithoutAnExpiryHasNone(t *testing.T) {
	for _, body := range []string{
		`{"Claims": {"iss": "nomad"}}`,
		`{"Claims": {"exp": "soon"}}`,
		`{"Claims": null}`,
	} {
		client, _, _ := metaServer(t, body)

		identity, err := client.NodeIdentity(context.Background(), "node-1")
		require.NoError(t, err, body)
		require.True(t, identity.Expires.IsZero(), body)
	}
}

func TestNodeIdentity_ANotFoundIsNoIdentity(t *testing.T) {
	client := refusing(t, http.StatusNotFound, "")

	_, err := client.NodeIdentity(context.Background(), "node-1")

	require.ErrorIs(t, err, nomad.ErrNoIdentity)
}

func TestNodeIdentity_AnotherErrorIsPassedOn(t *testing.T) {
	r := require.New(t)

	client := refusing(t, http.StatusInternalServerError, "rpc error")

	_, err := client.NodeIdentity(context.Background(), "node-1")

	r.Error(err)
	r.NotErrorIs(err, nomad.ErrNoIdentity)
}

func TestRenewNodeIdentity_PostsTheNode(t *testing.T) {
	r := require.New(t)

	client, sent, asked := metaServer(t, `{}`)

	r.NoError(client.RenewNodeIdentity(context.Background(), "node-1"))

	r.Equal(http.MethodPost, asked.Method)
	r.Equal("/v1/client/identity/renew", asked.URL.Path)
	r.JSONEq(`{"NodeID": "node-1"}`, string(*sent))
}
