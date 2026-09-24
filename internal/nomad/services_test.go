package nomad_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// servedRegistrations are three instances of served: one of a running
// allocation, one of an allocation that completed, one of an allocation the
// cluster no longer holds.
const servedRegistrations = `[
	{"ID": "reg-b", "ServiceName": "served", "Namespace": "default", "NodeID": "n2", "Datacenter": "dc1",
		"JobID": "served", "AllocID": "alloc-done", "Tags": ["web"], "Address": "10.0.0.6", "Port": 21659},
	{"ID": "reg-a", "ServiceName": "served", "Namespace": "default", "NodeID": "n1", "Datacenter": "dc1",
		"JobID": "served", "AllocID": "alloc-running", "Tags": ["web", "v2"], "Address": "10.0.0.5", "Port": 23133},
	{"ID": "reg-c", "ServiceName": "served", "Namespace": "default", "NodeID": "n3abcdef-0000", "Datacenter": "dc1",
		"JobID": "served", "AllocID": "alloc-gone", "Tags": [], "Address": "10.0.0.7", "Port": 24011}
]`

const servedAllocs = `[
	{"ID": "alloc-running", "JobID": "served", "NodeName": "node-01", "ClientStatus": "running"},
	{"ID": "alloc-done", "JobID": "served", "NodeName": "node-02", "ClientStatus": "complete"}
]`

func TestServiceInstances(t *testing.T) {
	r := require.New(t)

	client, asked := jobServer(t, map[string]string{
		"/v1/service/served":         servedRegistrations,
		"/v1/job/served/allocations": servedAllocs,
	})

	instances, err := client.ServiceInstances(context.Background(), "default", "served")
	r.NoError(err)

	r.Equal("default", sentTo(t, *asked, "/v1/service/served").namespace)
	r.Equal("default", sentTo(t, *asked, "/v1/job/served/allocations").namespace)

	// In the order of their addresses, each with the allocation it came
	// from as the cluster holds it now.
	r.Equal([]nomad.ServiceInstance{
		{ID: "reg-a", Service: "served", Namespace: "default", JobID: "served", AllocID: "alloc-running",
			NodeID: "n1", NodeName: "node-01", Address: "10.0.0.5", Port: 23133, Tags: []string{"web", "v2"}, AllocStatus: "running"},
		{ID: "reg-b", Service: "served", Namespace: "default", JobID: "served", AllocID: "alloc-done",
			NodeID: "n2", NodeName: "node-02", Address: "10.0.0.6", Port: 21659, Tags: []string{"web"}, AllocStatus: "complete"},
		{ID: "reg-c", Service: "served", Namespace: "default", JobID: "served", AllocID: "alloc-gone",
			NodeID: "n3abcdef-0000", Address: "10.0.0.7", Port: 24011, Tags: []string{}},
	}, instances)
}

func TestServiceInstance_Stale(t *testing.T) {
	r := require.New(t)

	// What runs, or is about to, still takes traffic.
	for _, status := range []string{"running", "pending", "unknown"} {
		r.False(nomad.ServiceInstance{AllocStatus: status}.Stale(), status)
	}

	// An allocation that stopped, or that the cluster no longer holds,
	// left its registration behind.
	for _, status := range []string{"complete", "failed", "lost", ""} {
		r.True(nomad.ServiceInstance{AllocStatus: status}.Stale(), status)
	}
}

func TestDeleteServiceRegistration(t *testing.T) {
	r := require.New(t)

	client, asked := jobServer(t, map[string]string{"/v1/service/served/reg-c": `{}`})

	r.NoError(client.DeleteServiceRegistration(context.Background(), "default", "served", "reg-c"))

	r.Len(*asked, 1)
	r.Equal("default", (*asked)[0].namespace)
}

func TestServiceInstances_InTheOrderOfTheirAddresses(t *testing.T) {
	r := require.New(t)

	client, _ := jobServer(t, map[string]string{
		"/v1/service/served": `[
			{"ID": "a", "JobID": "served", "Address": "10.0.0.10", "Port": 80},
			{"ID": "b", "JobID": "served", "Address": "10.0.0.9", "Port": 8080},
			{"ID": "c", "JobID": "served", "Address": "10.0.0.9", "Port": 443}
		]`,
		"/v1/job/served/allocations": `[]`,
	})

	instances, err := client.ServiceInstances(context.Background(), "default", "served")
	r.NoError(err)

	ids := []string{}
	for _, instance := range instances {
		ids = append(ids, instance.ID)
	}

	// Numbers read as numbers: .9 comes before .10, 443 before 8080.
	r.Equal([]string{"c", "b", "a"}, ids)
}
