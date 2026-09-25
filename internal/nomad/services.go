package nomad

import (
	"cmp"
	"context"
	"net/netip"
	"slices"
	"strings"
)

// ServiceInstance is one registration of a service: where it takes traffic,
// and the allocation that registered it, as the cluster holds it now.
type ServiceInstance struct {
	ID        string
	Service   string
	Namespace string
	JobID     string
	AllocID   string
	NodeID    string
	NodeName  string
	Address   string
	Port      int
	Tags      []string

	// AllocStatus is the client status of the allocation, empty when the
	// cluster no longer holds it.
	AllocStatus string
}

// Stale says the registration outlived its allocation: the allocation
// stopped, or the cluster no longer holds it. What runs, is about to, or is
// on a client that may come back still takes traffic.
func (s ServiceInstance) Stale() bool {
	switch s.AllocStatus {
	case "running", "pending", "unknown":
		return false
	}

	return true
}

// ServiceInstances are the registrations of a service, in the order of their
// addresses, each with the allocation that made it.
func (c *Client) ServiceInstances(ctx context.Context, namespace, name string) ([]ServiceInstance, error) {
	registrations, _, err := c.api.Services().Get(name, c.query(ctx, namespace))
	if err != nil {
		return nil, err
	}

	// The allocations of every job that registered the service, read once
	// per job.
	allocs := map[string]Alloc{}
	read := map[string]bool{}

	for _, reg := range registrations {
		if read[reg.JobID] {
			continue
		}

		read[reg.JobID] = true

		list, err := c.Allocations(ctx, namespace, reg.JobID)
		if err != nil {
			return nil, err
		}

		for _, alloc := range list {
			allocs[alloc.ID] = alloc
		}
	}

	out := make([]ServiceInstance, 0, len(registrations))

	for _, reg := range registrations {
		alloc := allocs[reg.AllocID]

		out = append(out, ServiceInstance{
			ID:          reg.ID,
			Service:     reg.ServiceName,
			Namespace:   reg.Namespace,
			JobID:       reg.JobID,
			AllocID:     reg.AllocID,
			NodeID:      reg.NodeID,
			NodeName:    alloc.NodeName,
			Address:     reg.Address,
			Port:        reg.Port,
			Tags:        reg.Tags,
			AllocStatus: alloc.Status,
		})
	}

	slices.SortFunc(out, func(a, b ServiceInstance) int {
		if order := compareAddresses(a.Address, b.Address); order != 0 {
			return order
		}

		return cmp.Compare(a.Port, b.Port)
	})

	return out, nil
}

// compareAddresses orders two addresses the way they read: 10.0.0.9 before
// 10.0.0.10. What is not an IP address, like a host name, is compared as
// text.
func compareAddresses(a, b string) int {
	ipA, errA := netip.ParseAddr(a)
	ipB, errB := netip.ParseAddr(b)

	if errA == nil && errB == nil {
		return ipA.Compare(ipB)
	}

	return strings.Compare(a, b)
}

// DeleteServiceRegistration takes one registration of a service out of the
// catalog.
func (c *Client) DeleteServiceRegistration(ctx context.Context, namespace, name, id string) error {
	_, err := c.api.Services().Delete(name, id, c.write(ctx, namespace))

	return err
}
