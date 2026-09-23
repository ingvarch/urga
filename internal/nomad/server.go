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

	// RPCAddress is where the server takes calls, which is the address the
	// rest of the cluster names it by. An agent may gossip on one address
	// and take calls on another.
	RPCAddress string

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

// Servers lists the servers of the region the client asks in and says which
// one leads.
func (c *Client) Servers(ctx context.Context) ([]Server, error) {
	members, err := c.api.Agent().MembersOpts(c.query(ctx, ""))
	if err != nil {
		return nil, err
	}

	// The gossip spans every region. A client that names none is answered
	// in the one of the agent, which the answer says itself.
	region := c.region
	if region == "" {
		region = members.ServerRegion
	}

	// Who leads is a separate question, and one the cluster may not be able
	// to answer during an election. The list is worth showing either way.
	// That endpoint takes no options, so it carries no context of its own.
	// Every region has a leader of its own.
	leader, _ := c.api.Status().RegionLeader(c.region)

	servers := make([]Server, 0, len(members.Members))

	for _, member := range members.Members {
		if member == nil || (region != "" && member.Tags["region"] != region) {
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

		server.RPCAddress = rpcAddress(member.Addr, member.Tags)
		server.Tags = member.Tags
		server.Protocol = int(member.ProtocolCur)
		server.ProtocolMin = int(member.ProtocolMin)
		server.ProtocolMax = int(member.ProtocolMax)

		server.Leader = leader != "" && server.RPCAddress == leader

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

// rpcAddress is where a member takes calls. The cluster names its leader and
// its raft peers by that address, and an agent may advertise one of its own
// instead of the address it gossips on.
func rpcAddress(address string, tags map[string]string) string {
	if advertised := tags["rpc_addr"]; advertised != "" {
		address = advertised
	}

	port := tags["port"]
	if address == "" || port == "" {
		return ""
	}

	return net.JoinHostPort(address, port)
}
