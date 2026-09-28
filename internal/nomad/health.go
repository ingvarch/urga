package nomad

import (
	"context"
	"time"
)

// ClusterHealth is how the servers of the region stand, as autopilot sees
// them.
type ClusterHealth struct {
	Healthy bool

	// FailureTolerance is how many servers can be lost without an outage.
	FailureTolerance int

	Servers []ServerHealth
}

// ServerHealth is how one server stands in the raft of the region.
type ServerHealth struct {
	// Name and Address are the name the server gossips under and the
	// address it takes calls on.
	Name, Address string

	Healthy, Voter, Leader bool

	// LastContact is how long ago the server last heard from the leader;
	// LastIndex is the last raft entry it holds.
	LastContact time.Duration
	LastIndex   uint64

	// StableSince is when the server last became healthy or unhealthy.
	StableSince time.Time
}

// ServerHealth reads how the servers of the region stand. It uses the
// operator endpoint, which an ACL may not allow.
func (c *Client) ServerHealth(ctx context.Context) (ClusterHealth, error) {
	reply, _, err := c.api.Operator().AutopilotServerHealth(c.query(ctx, ""))
	if err != nil {
		return ClusterHealth{}, err
	}

	health := ClusterHealth{Healthy: reply.Healthy, FailureTolerance: reply.FailureTolerance}

	for _, s := range reply.Servers {
		health.Servers = append(health.Servers, ServerHealth{
			Name:        s.Name,
			Address:     s.Address,
			Healthy:     s.Healthy,
			Voter:       s.Voter,
			Leader:      s.Leader,
			LastContact: s.LastContact,
			LastIndex:   s.LastIndex,
			StableSince: s.StableSince,
		})
	}

	return health, nil
}
