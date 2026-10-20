package ui

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// identityFrom is the first Nomad that gives a client an identity.
const identityFrom = "1.11.0"

// identityMsg is the identity of a client, nil when it has none. It carries
// the id of the client it was requested for.
type identityMsg struct {
	nodeID   string
	identity *nomad.NodeIdentity
}

// hasIdentity says the client is new enough to have an identity. An older
// one answers an error when asked, so it is not asked, and neither is one
// whose version is not read yet.
func hasIdentity(node nomad.Node) bool { return nomad.AtLeast(node.Version, identityFrom) }

// fetchIdentity reads the identity of a client. A cluster with none to read
// is not an error.
func fetchIdentity(client nodesClient, nodeID string) tea.Cmd {
	return request(func(ctx context.Context) (*nomad.NodeIdentity, error) {
		identity, err := client.NodeIdentity(ctx, nodeID)
		if errors.Is(err, nomad.ErrNoIdentity) {
			return nil, nil
		}

		if err != nil {
			return nil, err
		}

		return &identity, nil
	}, func(identity *nomad.NodeIdentity) tea.Msg { return identityMsg{nodeID: nodeID, identity: identity} })
}

// identityText is what the panel says of the identity: when it expires, or
// nothing when it has no expiry.
func identityText(identity *nomad.NodeIdentity) string {
	if identity == nil || identity.Expires.IsZero() {
		return ""
	}

	left := expiresIn(identity.Expires)
	if left == "expired" {
		return left
	}

	return "expires " + left
}

func holdsIdentity(p clientPage, _ env) bool { return p.host.identity != nil }

// showIdentity opens the claims the page holds.
func showIdentity(p clientPage, _ env) (clientPage, outcome) {
	return p, then(describeMsg{
		label:   sprintf("Identity (Client: %s)", p.name),
		content: p.host.identity.Claims,
	})
}

// renewIdentity asks the client to get a new identity, after the user
// confirms.
func renewIdentity(p clientPage, e env) (clientPage, outcome) {
	client, nodeID := e.client, p.nodeID

	return p, then(askMsg{
		question: sprintf("Really renew the identity of the client %s?", p.name),
		apply: act(sprintf("The client %s gets a new identity at its next heartbeat.", p.name), func(ctx context.Context) error {
			return client.RenewNodeIdentity(ctx, nodeID)
		}),
	})
}
