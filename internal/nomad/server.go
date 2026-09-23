package nomad

import (
	"context"
	"errors"
	"net"
)

// ErrNoServer is a server the cluster does not know.
var ErrNoServer = errors.New("no such server")

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

	// Tags are what the agent says about itself in the gossip pool. The
	// fields above are read out of them; the rest is what this cluster was
	// built with, and it belongs on the detail of a server.
	Tags map[string]string

	// Protocol is the version of the gossip the agent speaks, and the range
	// it can speak.
	Protocol    int
	ProtocolMin int
	ProtocolMax int
}

// RaftPeer is one server as the raft of the cluster sees it, which is not
// the same view as the gossip: a server can be alive and no longer count.
type RaftPeer struct {
	ID      string
	Node    string
	Address string

	Leader bool
	Voter  bool

	Protocol string
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

		server.Tags = member.Tags
		server.Protocol = int(member.ProtocolCur)
		server.ProtocolMin = int(member.ProtocolMin)
		server.ProtocolMax = int(member.ProtocolMax)

		server.Leader = leads(member.Addr, member.Tags["port"], leader)

		servers = append(servers, server)
	}

	return servers, nil
}

// Server is one server of the cluster, by the name the list gives it.
func (c *Client) Server(ctx context.Context, name string) (Server, error) {
	servers, err := c.Servers(ctx)
	if err != nil {
		return Server{}, err
	}

	for _, server := range servers {
		if server.Name == name {
			return server, nil
		}
	}

	return Server{}, ErrNoServer
}

// RaftPeers is the raft configuration of the cluster: who votes and who
// leads. It is asked of the operator, which an ACL may not allow.
func (c *Client) RaftPeers(ctx context.Context) ([]RaftPeer, error) {
	config, err := c.api.Operator().RaftGetConfiguration(c.query(ctx, ""))
	if err != nil {
		return nil, err
	}

	peers := make([]RaftPeer, 0, len(config.Servers))

	for _, server := range config.Servers {
		if server == nil {
			continue
		}

		peers = append(peers, RaftPeer{
			ID:       server.ID,
			Node:     server.Node,
			Address:  server.Address,
			Leader:   server.Leader,
			Voter:    server.Voter,
			Protocol: server.RaftProtocol,
		})
	}

	return peers, nil
}

// leads compares a member with the address the cluster gives for its leader,
// which is the RPC address rather than the one the members gossip on.
func leads(address, rpcPort, leader string) bool {
	if leader == "" || rpcPort == "" {
		return false
	}

	return net.JoinHostPort(address, rpcPort) == leader
}
