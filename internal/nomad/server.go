package nomad

import (
	"context"
	"net"
)

// Server is a Nomad server: the agents that schedule the work and hold the
// state, as against the clients that run it.
type Server struct {
	Name       string
	Address    string
	Port       int
	Datacenter string
	Region     string
	Version    string
	Status     string

	// Leader says this is the server the others follow.
	Leader bool
}

// Servers lists the servers of the cluster and says which one leads.
func (c *Client) Servers(_ context.Context) ([]Server, error) {
	members, err := c.api.Agent().Members()
	if err != nil {
		return nil, err
	}

	// Who leads is a separate question, and one the cluster may not be able
	// to answer during an election. The list is worth showing either way.
	leader, _ := c.api.Status().Leader()

	servers := make([]Server, 0, len(members.Members))

	for _, member := range members.Members {
		if member == nil {
			continue
		}

		server := Server{
			Name:       member.Name,
			Address:    member.Addr,
			Port:       int(member.Port),
			Datacenter: member.Tags["dc"],
			Region:     member.Tags["region"],
			Version:    member.Tags["build"],
			Status:     member.Status,
		}

		server.Leader = leads(member.Addr, member.Tags["port"], leader)

		servers = append(servers, server)
	}

	return servers, nil
}

// leads compares a member with the address the cluster gives for its leader,
// which is the RPC address rather than the one the members gossip on.
func leads(address, rpcPort, leader string) bool {
	if leader == "" || rpcPort == "" {
		return false
	}

	return net.JoinHostPort(address, rpcPort) == leader
}
