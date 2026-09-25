package nomad

import (
	"context"
	"slices"
	"strings"
)

// Regions are the regions the cluster knows. The context is unused and only
// keeps the signature like the other calls: the endpoint of Nomad takes none.
// The agent answers it, and it knows every region it gossips with.
func (c *Client) Regions(_ context.Context) ([]string, error) {
	return c.api.Regions().List()
}

// Datacenters are the datacenters of the region the client requests in: the
// ones its nodes are in.
func (c *Client) Datacenters(ctx context.Context) ([]string, error) {
	nodes, _, err := c.api.Nodes().List(c.query(ctx, ""))
	if err != nil {
		return nil, err
	}

	datacenters := make([]string, 0, len(nodes))
	for _, node := range nodes {
		datacenters = append(datacenters, node.Datacenter)
	}

	slices.Sort(datacenters)

	return slices.Compact(datacenters), nil
}

// RunsIn says the job may be placed in the datacenter. An empty datacenter
// is every one of them, and so is a job that names none.
func (j Job) RunsIn(datacenter string) bool {
	if datacenter == "" || len(j.Datacenters) == 0 {
		return true
	}

	for _, pattern := range j.Datacenters {
		if matches(pattern, datacenter) {
			return true
		}
	}

	return false
}

// matches compares a datacenter of a job the way Nomad does: a star matches
// any run of characters, everything else matches only itself.
func matches(pattern, name string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == name
	}

	first, last := parts[0], parts[len(parts)-1]
	if !strings.HasPrefix(name, first) {
		return false
	}

	rest := name[len(first):]

	for _, part := range parts[1 : len(parts)-1] {
		at := strings.Index(rest, part)
		if at < 0 {
			return false
		}

		rest = rest[at+len(part):]
	}

	return strings.HasSuffix(rest, last)
}
