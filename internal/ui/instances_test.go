package ui

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// Allocations behind the instances of served.
const (
	servedRunning = "37f18b8e-0000-0000-0000-000000000000"
	servedLost    = "4f2a1c9e-0000-0000-0000-000000000000"
	servedGone    = "9a1b2c3d-0000-0000-0000-000000000000"
)

// servedInstances are the instances of served: one takes traffic, one was
// left by an allocation on a client that was lost, one by an allocation the
// cluster no longer holds.
func servedInstances() []nomad.ServiceInstance {
	return []nomad.ServiceInstance{
		{ID: "reg-a", Service: "served", Namespace: "default", JobID: "served", AllocID: servedRunning,
			NodeName: "node-01", Address: "10.0.0.5", Port: 23133, Tags: []string{"web"}, AllocStatus: "running"},
		{ID: "reg-c", Service: "served", Namespace: "default", JobID: "served", AllocID: servedLost,
			NodeName: "node-03", Address: "10.0.0.7", Port: 24011, AllocStatus: "lost"},
		{ID: "reg-d", Service: "served", Namespace: "default", JobID: "served", AllocID: servedGone,
			NodeID: "n4abcdef-0000-0000-0000-000000000000", Address: "10.0.0.8", Port: 25000},
	}
}

// onServed is the instances of served, opened from the list of services.
func onServed(t *testing.T, client *fakeClient) Model {
	t.Helper()

	client.services = []nomad.Service{{Name: "served", Namespace: "default"}}
	client.instances = servedInstances()

	m := newTestModel(client)
	m, _ = m.show(screenServices)
	m, _ = m.update(servicesMsg(client.services))

	m, cmd := m.update(enter())

	return drain(m, cmd)
}

func TestServices_EnterOpensTheInstances(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onServed(t, client)

	r.Equal(screenServiceInstances, m.screen.kind)
	r.Equal("served", client.instancesName)
	r.Equal("default", client.instancesNamespace)

	r.Contains(plain(m.render()), "Service served (default) [3]")

	// Where each instance takes traffic, what registered it, and whether
	// it still does.
	r.Contains(fileRow(t, m, 0), "10.0.0.5:23133")
	r.Contains(fileRow(t, m, 0), "37f18b8e")
	r.Contains(fileRow(t, m, 0), "node-01")
	r.Contains(fileRow(t, m, 0), "web")
	r.Contains(fileRow(t, m, 0), "running")
	r.Contains(fileRow(t, m, 1), "stale: alloc lost")
	r.Contains(fileRow(t, m, 2), "stale: alloc gone")

	// A client the cluster no longer names is named by its id.
	r.Contains(fileRow(t, m, 2), "n4abcdef")
}

func TestServiceInstances_OnlyAStaleOneIsDeleted(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onServed(t, client)

	// The one that takes traffic stays.
	r.False(offers(m, "ctrl-d"))

	m, _ = m.update(key('j'))
	r.True(offers(m, "ctrl-d"))

	m, _ = m.update(ctrl('d'))
	r.Contains(plain(m.render()), "Really delete the registration of served at 10.0.0.7:24011? Its allocation is lost.")

	m = confirmed(t, m)

	r.Equal("reg-c", client.deletedRegistration)
	r.Equal("served", client.instancesName)
	r.Equal("default", client.askedNamespace)
	r.Contains(plain(m.render()), "Registration of served at 10.0.0.7:24011 deleted.")

	// One the cluster no longer holds says so.
	m, _ = m.update(key('j'))
	m, _ = m.update(ctrl('d'))
	r.Contains(plain(m.render()), "Its allocation is gone.")
}

func TestServiceInstances_EnterOpensTheAllocation(t *testing.T) {
	r := require.New(t)

	m := onServed(t, &fakeClient{})

	m, _ = m.update(enter())

	r.Equal(screenTasks, m.screen.kind)
	r.Equal(servedRunning, m.screen.allocID)
	r.Equal("served", m.screen.jobID)
}

func TestServiceInstances_NoTasksOfAnAllocationThatIsGone(t *testing.T) {
	r := require.New(t)

	m := onServed(t, &fakeClient{})
	m, _ = m.update(key('G'))

	r.False(offers(m, "enter"))
}

func TestServiceInstances_TheirChecks(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{checks: []nomad.Check{
		{Service: "served", Name: "alive", Status: "success"},
		{Service: "served", Name: "ready", Status: "failure"},
		{Service: "served-admin", Name: "admin-up", Status: "success"},
	}}

	m := onServed(t, client)

	m, cmd := m.update(pollInstanceChecksMsg{})
	m = drain(m, cmd)

	// Read only for what runs, and only the checks of this service: the
	// worst of them first.
	r.Equal(1, client.checksCalls)
	r.Equal(servedRunning, client.checksAllocID)
	r.Contains(fileRow(t, m, 0), "1 failing")

	// What does not run has no checks to read.
	checks := slices.Index(instanceTitles, "Checks")
	r.Equal("-", m.table.rows[1].cells[checks])
}
