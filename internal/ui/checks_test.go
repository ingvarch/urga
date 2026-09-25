package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// servedChecks are the checks of an allocation of served, failures first as
// the cluster layer puts them.
func servedChecks() []nomad.Check {
	return []nomad.Check{
		{Service: "served-admin", Name: "admin-up", Status: "failure",
			Output: "dial tcp 127.0.0.1:22689:\nconnect: connection refused", Time: time.Now().Add(-12 * time.Second)},
		{Service: "served", Name: "ready", Status: "pending"},
		{Service: "served", Name: "alive", Status: "success", Output: "nomad: http ok", Time: time.Now().Add(-3 * time.Second)},
	}
}

// running is the first allocation of twoAllocs, running and read in full.
func running() nomad.Alloc {
	alloc := twoAllocs()[0]
	alloc.DesiredStatus = "run"

	return alloc
}

// unstyled are rows as they read, without their colours.
func unstyled(t *testing.T, rows []string) []string {
	t.Helper()

	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, plain(row))
	}

	return out
}

func TestAllocPanel_Checks(t *testing.T) {
	r := require.New(t)

	// A check that fails says why under it; one that passes or has not run
	// yet needs no more than its name.
	r.Equal([]string{
		" Status running   Desired run   Client node-01   Version 0",
		"",
		" Checks",
		"  ● served-admin/admin-up  failure  12s ago",
		"    dial tcp 127.0.0.1:22689: connect: connection refused",
		"  ● served/ready           pending",
		"  ● served/alive           success  3s ago",
		"",
	}, unstyled(t, allocPanel(running(), servedChecks(), 120, 100)))
}

func TestAllocPanel_ChecksThatDoNotFit(t *testing.T) {
	r := require.New(t)

	checks := []nomad.Check{}
	for i := range 5 {
		checks = append(checks, nomad.Check{Service: "web", Name: fmt.Sprintf("c%d", i), Status: "success"})
	}

	// What does not fit is counted, not cut off in the middle.
	r.Equal([]string{
		" Status running   Desired run   Client node-01   Version 0",
		"",
		" Checks",
		"  ● web/c0  success",
		"  ● web/c1  success",
		"  + 3 more checks",
		"",
	}, unstyled(t, allocPanel(running(), checks, 120, 7)))

	// With no room for one of them, the tasks come first.
	r.Equal([]string{
		" Status running   Desired run   Client node-01   Version 0",
		"",
	}, unstyled(t, allocPanel(running(), checks, 120, 4)))
}

func TestTasks_TheirChecksComeWithTheAllocation(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), checks: servedChecks()}
	m := openTasks(t, client)

	m, cmd := m.update(allocMsg(running()))
	m = drain(m, cmd)

	// The checks are read from the client that runs them, in the namespace
	// of the allocation.
	r.Equal("af1f37df-7b19-6b1c-da67-5e8f482b5a15", client.checksAllocID)
	r.Equal("production", client.checksNamespace)
	r.Contains(plain(m.render()), "served-admin/admin-up  failure")

	// The allocation is read again on every change the cluster reports.
	// None of those is a reading of the checks: they have a timer of their
	// own.
	for range 3 {
		m, cmd = m.update(allocMsg(running()))
		m = drain(m, cmd)
	}

	r.Equal(1, client.checksCalls)
}

func TestTasks_EveryReadingOfTheChecksSetsTheTimerForTheNext(t *testing.T) {
	r := require.New(t)

	m := openTasks(t, &fakeClient{jobs: twoJobs(), allocs: twoAllocs()})
	m, _ = m.update(allocMsg(running()))

	_, cmd := m.update(checksMsg{allocID: running().ID, checks: servedChecks()})
	r.NotNil(cmd)

	// A client that did not answer is asked again all the same.
	_, cmd = m.update(checksMsg{allocID: running().ID, err: errTest})
	r.NotNil(cmd)
}

func TestTasks_TheTimerReadsTheChecks(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), checks: servedChecks()}
	m := openTasks(t, client)
	m, _ = m.update(allocMsg(running()))

	m, cmd := m.update(pollChecksMsg{})
	drain(m, cmd)

	r.Equal(1, client.checksCalls)
}

func TestTasks_ChecksOfAnotherAllocationAreDropped(t *testing.T) {
	r := require.New(t)

	m := openTasks(t, &fakeClient{jobs: twoJobs(), allocs: twoAllocs()})
	m, _ = m.update(allocMsg(running()))

	m, cmd := m.update(checksMsg{allocID: "b2222222-0000-0000-0000-000000000000", checks: servedChecks()})

	r.Nil(cmd)
	r.NotContains(plain(m.render()), "Checks")
}

func TestTasks_AnAllocationThatStoppedHasNoChecksToRead(t *testing.T) {
	r := require.New(t)

	stopped := running()
	stopped.Status = "complete"

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), checks: servedChecks()}
	m := openTasks(t, client)

	m, cmd := m.update(allocMsg(stopped))
	m = drain(m, cmd)

	_, cmd = m.update(pollChecksMsg{})
	r.Nil(cmd)
	r.Zero(client.checksCalls)
}

func TestTasks_ChecksGoWhenTheAllocationStops(t *testing.T) {
	r := require.New(t)

	m := openTasks(t, &fakeClient{jobs: twoJobs(), allocs: twoAllocs()})
	m, _ = m.update(allocMsg(running()))
	m, _ = m.update(checksMsg{allocID: running().ID, checks: servedChecks()})
	r.Contains(plain(m.render()), "Checks")

	// What its client said while it ran is not what it is now.
	stopped := running()
	stopped.Status = "complete"
	m, _ = m.update(allocMsg(stopped))

	r.NotContains(plain(m.render()), "Checks")
}

func TestTasks_AReadingOfTheChecksThatFailsSaysSo(t *testing.T) {
	r := require.New(t)

	m := openTasks(t, &fakeClient{jobs: twoJobs(), allocs: twoAllocs()})
	m, _ = m.update(allocMsg(running()))

	m, _ = m.update(checksMsg{allocID: running().ID, err: errTest})

	r.Contains(plain(m.render()), errTest.Error())
}

func TestTasks_AReadingOfTheChecksTakesAnErrorDown(t *testing.T) {
	r := require.New(t)

	m := openTasks(t, &fakeClient{jobs: twoJobs(), allocs: twoAllocs()})
	m, _ = m.update(allocMsg(running()))
	m, _ = m.update(errMsg{err: errTest})
	r.Contains(plain(m.render()), errTest.Error())

	// The client that runs the checks answered: what went wrong is over.
	m, _ = m.update(checksMsg{allocID: running().ID, checks: servedChecks()})

	r.NotContains(plain(m.render()), errTest.Error())
}

func TestTasks_ChecksOfTasksThatWereLeftAreDropped(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), checks: servedChecks()}
	m := openTasks(t, client)
	m, _ = m.update(allocMsg(running()))
	m, _ = m.update(escape())

	// The reading and the timer of the tasks answer on the allocations.
	m, cmd := m.update(checksMsg{allocID: running().ID, checks: servedChecks()})
	r.Nil(cmd)

	_, cmd = m.update(pollChecksMsg{})
	r.Nil(cmd)
	r.Zero(client.checksCalls)
}

func TestTasks_TheChecksAreReadAgainOnEveryVisit(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), checks: servedChecks()}
	m := openTasks(t, client)

	m, cmd := m.update(allocMsg(running()))
	m = drain(m, cmd)
	r.Equal(1, client.checksCalls)

	// The timer of the checks ended with the visit; coming back starts
	// another one.
	m, _ = m.update(key('e'))
	m, _ = m.update(escape())

	m, cmd = m.update(allocMsg(running()))
	drain(m, cmd)

	r.Equal(2, client.checksCalls)
}
