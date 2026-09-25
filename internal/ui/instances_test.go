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
	m, _ = m.show(servicesView)
	m, _ = m.update(servicesMsg(client.services))

	m, cmd := m.update(enter())

	return drain(m, cmd)
}

func TestServices_EnterOpensTheInstances(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onServed(t, client)

	r.IsType(serviceInstancesPage{}, m.screen.page)
	r.Equal("served", client.instancesName)
	r.Equal("default", client.instancesNamespace)

	r.Contains(plain(m.render()), "Service served (default) [3]")

	// The stream watches the services where this one lives.
	r.Equal("default", client.watchedNamespace)
	r.Equal([]string{nomad.TopicService}, client.watchedTopics)

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

	r.IsType(tasksPage{}, m.screen.page)
	r.Contains(plain(m.render()), "Tasks (Allocation: "+shortID(servedRunning)+")")
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
	r.Equal("-", m.list.table.rows[1].cells[checks])
}

func TestServices_TheListOfTheNamespaceAndItsKeys(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{services: []nomad.Service{
		{Name: "api", Namespace: "production", Tags: []string{"http", "v1"}},
		{Name: "web", Namespace: "production"},
	}}
	m := typeCommand(newTestModel(client), "services")

	// Asked in the namespace of the session, and titled with it.
	r.Equal("production", client.askedNamespace)
	r.Contains(plain(m.render()), "Services (production) [2]")

	header := fileRow(t, m, -1)
	for _, title := range []string{"Name", "Namespace", "Tags"} {
		r.Contains(header, title)
	}

	r.Contains(fileRow(t, m, 0), "api")
	r.Contains(fileRow(t, m, 0), "http, v1")
	r.Contains(fileRow(t, m, 1), "web")

	r.Equal([]hint{{Key: "<enter>", Description: "Instances"}, {Key: "<d>", Description: "Describe"}}, m.hints())
}

func TestServices_DescribeTheOneUnderTheCursor(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		services: []nomad.Service{{Name: "api", Namespace: "production"}, {Name: "web", Namespace: "default"}},
		describe: "{\n  \"Name\": \"web\"\n}",
	}
	m, _ := runLine(newTestModel(client), "services")
	m, _ = m.update(servicesMsg(client.services))
	m, _ = m.update(down())

	m, cmd := m.update(key('d'))
	m = drain(m, cmd)

	// Asked where the service lives, under a title that names it.
	r.Equal("default", client.askedNamespace)
	r.Equal("web", client.askedID)
	r.IsType(describePage{}, m.screen.page)

	out := plain(m.render())
	r.Contains(out, "Service: web")
	r.Contains(out, `"Name": "web"`)
}

func TestServices_EnterOpensTheInstancesOfTheOneUnderTheCursor(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{services: []nomad.Service{{Name: "api", Namespace: "production"}, {Name: "web", Namespace: "default"}}}
	m, _ := runLine(newTestModel(client), "services")
	m, _ = m.update(servicesMsg(client.services))
	m, _ = m.update(down())

	m, cmd := m.update(enter())
	m = drain(m, cmd)

	r.Equal("web", client.instancesName)
	r.Equal("default", client.instancesNamespace)
	r.Contains(plain(m.render()), "Service web (default) [0]")
}

func TestServices_AListThatAnswersLateIsDropped(t *testing.T) {
	r := require.New(t)

	m := onServed(t, &fakeClient{})

	// The list answers while the instances of one of its services are open.
	m, _ = m.update(servicesMsg{{Name: "billing", Namespace: "default"}})
	r.Contains(fileRow(t, m, 0), "10.0.0.5:23133")

	// Back on the list, it shows what it held.
	m, _ = m.update(escape())

	out := plain(m.render())
	r.Contains(out, "Services (production) [1]")
	r.Contains(fileRow(t, m, 0), "served")
	r.NotContains(out, "billing")
}

func TestServices_OfTheRegionLeftAreLetGo(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{services: []nomad.Service{{Name: "served", Namespace: "default"}}, instances: servedInstances()}
	m := regionalModel(t, client)
	m = typeCommand(m, "services")
	r.Contains(fileRow(t, m, 0), "served")

	// From the instances of one of them: the region is left for the list.
	m, cmd := m.update(enter())
	m = drain(m, cmd)
	r.Contains(plain(m.render()), "Service served (default) [3]")

	// The services of eu must not stand under the name of us before us
	// answers.
	client.services = nil
	m, _ = runLine(m, "region us")

	out := plain(m.render())
	r.Contains(out, "Services (production) [0]")
	r.NotContains(out, "served")
}

func TestServiceInstances_TheKeysOfAnInstance(t *testing.T) {
	r := require.New(t)

	m := onServed(t, &fakeClient{})

	// One that takes traffic has tasks to open and nothing to delete.
	r.Equal([]hint{{Key: "<enter>", Description: "Tasks"}}, m.hints())

	// One its lost allocation left behind has both.
	m, _ = m.update(key('j'))
	r.Equal([]hint{{Key: "<enter>", Description: "Tasks"}, {Key: "<ctrl-d>", Description: "Delete"}}, m.hints())

	// One whose allocation is gone has no tasks left.
	m, _ = m.update(key('j'))
	r.Equal([]hint{{Key: "<ctrl-d>", Description: "Delete"}}, m.hints())
}

func TestServiceInstances_ReadOnlyDeletesNothing(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onServed(t, client)
	m.opts.ReadOnly = true

	m, _ = m.update(key('j'))
	r.Equal([]hint{{Key: "<enter>", Description: "Tasks"}}, m.hints())

	m, _ = m.update(ctrl('d'))
	r.Contains(plain(m.render()), "read-only: Delete is off")
	r.Equal(overlayNone, m.overlay)
	r.Empty(client.deletedRegistration)
}

func TestServiceInstances_TheTasksOpenWhereTheAllocationLives(t *testing.T) {
	r := require.New(t)

	m := onServed(t, &fakeClient{})
	m, _ = m.update(enter())

	r.Equal("default", m.screen.page.(tasksPage).namespace)
	r.Contains(plain(m.render()), "Tasks (Allocation: 37f18b8e)")
}

func TestServiceInstances_WhatNeedsLookingAtStandsOut(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{checks: []nomad.Check{{Service: "served", Name: "ready", Status: "failure"}}}
	m := onServed(t, client)

	m, cmd := m.update(pollInstanceChecksMsg{})
	m = drain(m, cmd)

	// Failing its checks, and outliving its allocation.
	rows := m.list.table.rows
	r.Equal(colorAttention, rows[0].color)
	r.Equal(colorDead, rows[1].color)
	r.Equal(colorDead, rows[2].color)
}

func TestServiceInstances_TheChecksAreReadOnceForEveryVisit(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{checks: []nomad.Check{{Service: "served", Name: "alive", Status: "success"}}}
	m := onServed(t, client)

	// The first answer started the reading. The instances are read again
	// on every change: none of those starts a second one.
	m, cmd := m.update(instancesMsg(servedInstances()))
	drain(m, cmd)
	r.Zero(client.checksCalls)

	// Coming back to them is a visit of its own, with its own reading.
	m, _ = m.update(enter())
	m, _ = m.update(escape())

	m, cmd = m.update(instancesMsg(servedInstances()))
	m = drain(m, cmd)

	r.Equal(1, client.checksCalls)
	r.Contains(fileRow(t, m, 0), "1 passing")
}

func TestServiceInstances_EveryReadingOfTheChecksSetsTheTimerForTheNext(t *testing.T) {
	r := require.New(t)

	m := onServed(t, &fakeClient{})

	_, cmd := m.update(instanceChecksMsg{service: "served", byAlloc: map[string][]nomad.Check{}})
	r.NotNil(cmd)
}

func TestServiceInstances_ChecksOfAnotherServiceAreDropped(t *testing.T) {
	r := require.New(t)

	m := onServed(t, &fakeClient{})

	failing := map[string][]nomad.Check{servedRunning: {{Service: "served", Name: "ready", Status: "failure"}}}
	m, cmd := m.update(instanceChecksMsg{service: "other", byAlloc: failing})

	r.Nil(cmd)
	r.NotContains(fileRow(t, m, 0), "failing")
}

func TestServiceInstances_AReadingOfTheLastVisitIsDropped(t *testing.T) {
	r := require.New(t)

	m := onServed(t, &fakeClient{checks: []nomad.Check{{Service: "served", Name: "ready", Status: "failure"}}})
	_, read := m.update(pollInstanceChecksMsg{})

	// The instances are left and come back before the reading answers.
	m, _ = m.update(enter())
	m, _ = m.update(escape())

	// It answers for the visit that asked, and starts no second timer.
	m, cmd := m.update(read())
	r.Nil(cmd)
	r.NotContains(fileRow(t, m, 0), "failing")
}

func TestServiceInstances_AReadingOfTheChecksLeavesTheErrorUp(t *testing.T) {
	r := require.New(t)

	m := onServed(t, &fakeClient{})
	m, _ = m.update(errMsg{err: errTest})

	// A reading is no answer of the cluster about the instances: what went
	// wrong with them still stands.
	m, _ = m.update(pollInstanceChecksMsg{})
	m, _ = m.update(instanceChecksMsg{service: "served", byAlloc: map[string][]nomad.Check{}})

	r.Contains(plain(m.render()), errTest.Error())
}
