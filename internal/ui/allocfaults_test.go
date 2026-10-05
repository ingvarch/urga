package ui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// allocFaultsAlloc is an allocation of web in production with the tasks given.
func allocFaultsAlloc(id, status string, tasks ...nomad.Task) nomad.Alloc {
	return nomad.Alloc{
		ID: id, Namespace: "production", JobID: "web", TaskGroup: "frontend",
		Status: status, Tasks: tasks,
	}
}

func allocFaultsRestartedAgo(task string, ago time.Duration) nomad.Task {
	return nomad.Task{Name: task, Restarts: 1, LastRestart: time.Now().Add(-ago)}
}

func allocFaultsKilledAgo(ago time.Duration) nomad.Task {
	event := nomad.TaskEvent{Time: time.Now().Add(-ago), Type: "Terminated", OOM: true}

	return nomad.Task{Name: "server", Events: []nomad.TaskEvent{event}}
}

func TestAllocFaults_FailedOrLostThatNothingReplaced(t *testing.T) {
	r := require.New(t)

	replaced := allocFaultsAlloc("replaced", statusFailed)
	replaced.Next = "newer"

	page := allocationsPage{read: time.Now(), fault: faultFailedOrLost, allocs: []nomad.Alloc{
		allocFaultsAlloc("failed", statusFailed),
		allocFaultsAlloc("lost", statusLost),
		replaced,
		allocFaultsAlloc("running", statusRunning),
		allocFaultsAlloc("complete", statusComplete),
	}}

	r.Equal([]string{"failed", "lost"}, page.ids(env{}))
}

func TestAllocFaults_RestartingWithinTheHour(t *testing.T) {
	r := require.New(t)

	page := allocationsPage{read: time.Now(), fault: faultRestarting, allocs: []nomad.Alloc{
		allocFaultsAlloc("recent", statusRunning, allocFaultsRestartedAgo("server", 10*time.Minute)),
		allocFaultsAlloc("old", statusRunning, allocFaultsRestartedAgo("server", 2*time.Hour)),
		allocFaultsAlloc("failed", statusFailed, allocFaultsRestartedAgo("server", 10*time.Minute)),
	}}

	r.Equal([]string{"recent"}, page.ids(env{}))
}

func TestAllocFaults_KilledForMemoryWithinTheHour(t *testing.T) {
	r := require.New(t)

	page := allocationsPage{read: time.Now(), fault: faultOOMKilled, allocs: []nomad.Alloc{
		allocFaultsAlloc("recent", statusRunning, allocFaultsKilledAgo(5*time.Minute)),
		allocFaultsAlloc("old", statusRunning, allocFaultsKilledAgo(3*time.Hour)),
		allocFaultsAlloc("failed", statusFailed, allocFaultsKilledAgo(5*time.Minute)),
		allocFaultsAlloc("restarted", statusRunning, allocFaultsRestartedAgo("server", 5*time.Minute)),
	}}

	r.Equal([]string{"recent"}, page.ids(env{}))
}

func TestAllocFaults_TitleSaysWhatTheListIsNarrowedTo(t *testing.T) {
	r := require.New(t)

	allocs := []nomad.Alloc{
		allocFaultsAlloc("recent", statusRunning, allocFaultsRestartedAgo("server", 10*time.Minute)),
		allocFaultsAlloc("fine", statusRunning),
	}

	m := newTestModel(&fakeClient{allocs: allocs})
	m, _ = m.update(openMsg{allocationsPage{read: time.Now(), allocs: allocs, fault: faultRestarting}})

	r.Equal("Allocations (production, restarting) [1]", m.title())
}

func TestAllocFaults_KeysActOnTheRowsThatAreLeft(t *testing.T) {
	r := require.New(t)

	allocs := []nomad.Alloc{
		allocFaultsAlloc("aaaa1111", statusRunning),
		allocFaultsAlloc("bbbb2222", statusRunning, allocFaultsRestartedAgo("server", 10*time.Minute)),
	}

	m := newTestModel(&fakeClient{allocs: allocs, changes: newChanges()})
	m, _ = m.update(openMsg{allocationsPage{read: time.Now(), allocs: allocs, fault: faultRestarting}})

	m, cmd := m.update(enter())
	m = playOut(m, cmd)

	r.IsType(tasksPage{}, m.screen.page)
	r.Equal("bbbb2222", m.screen.page.(tasksPage).allocID)
}

func TestAllocFaults_LogsReadTheAllocationsOfTheListOnly(t *testing.T) {
	r := require.New(t)

	fine := allocFaultsAlloc("aaaa1111", statusRunning, nomad.Task{Name: "quiet"})
	troubled := allocFaultsAlloc("bbbb2222", statusRunning,
		allocFaultsRestartedAgo("server", 10*time.Minute),
		allocFaultsRestartedAgo("proxy", 10*time.Minute))

	allocs := []nomad.Alloc{fine, troubled}

	m := newTestModel(&fakeClient{allocs: allocs, changes: newChanges()})
	m, _ = m.update(openMsg{allocationsPage{read: time.Now(), allocs: allocs, fault: faultRestarting}})

	m, cmd := m.update(key('l'))
	m = playOut(m, cmd)

	r.IsType(logTasksPage{}, m.screen.page)

	found := m.screen.page.(logTasksPage).choices
	r.Len(found, 2)
	r.Equal("proxy", found[0].task)
	r.Equal("server", found[1].task)
}

func TestAllocFaults_TheLogsOfTheNarrowedListHaveAName(t *testing.T) {
	r := require.New(t)

	allocs := []nomad.Alloc{allocFaultsAlloc("aaaa1111", statusFailed)}

	m := newTestModel(&fakeClient{allocs: allocs, changes: newChanges()})
	m, _ = m.update(openMsg{allocationsPage{read: time.Now(), allocs: allocs, fault: faultFailedOrLost}})

	// None of the failed allocations runs, so there is nothing to read.
	m, cmd := m.update(key('l'))
	m = playOut(m, cmd)

	out := plain(m.render())
	r.Contains(out, "the list of failed or lost allocations has no allocation running")
	r.NotContains(out, "the namespace")
}

func TestAllocFaults_AReloadOfLogsFollowsTheReplacementsToo(t *testing.T) {
	r := require.New(t)

	restarting := allocFaultsAlloc("aaaa1111", statusRunning, allocFaultsRestartedAgo("server", 10*time.Minute))
	replacement := allocFaultsAlloc("bbbb2222", statusRunning, nomad.Task{Name: "server"})

	client := &fakeClient{
		allocs:      []nomad.Alloc{restarting},
		changes:     newChanges(),
		logsByAlloc: map[string]*nomad.LogStream{"aaaa1111": writing(), "bbbb2222": writing()},
	}
	m := newTestModel(client)
	m, _ = m.update(openMsg{allocationsPage{read: time.Now(), allocs: client.allocs, fault: faultRestarting}})

	m, cmd := m.update(key('l'))
	m = playOut(m, cmd)
	r.IsType(jobLogsPage{}, m.screen.page)

	// A deployment replaced the allocation, and the new one never restarted.
	client.allocs, client.logsOpened = []nomad.Alloc{replacement}, nil

	m, cmd = m.update(key('r'))
	m = playOut(m, cmd)

	r.NotContains(plain(m.render()), "runs in no allocation now")
	r.Equal([]string{"bbbb2222"}, client.logsOpened)
}

func TestAllocFaults_TheListKeepsTheClockOfItsRead(t *testing.T) {
	r := require.New(t)

	// The first restart is 59m59s old at the read and an hour old by now.
	read := time.Now().Add(-5 * time.Second)
	ago := func(d time.Duration) time.Time { return read.Add(-d) }

	restart := func(id string, at time.Time) nomad.Alloc {
		return allocFaultsAlloc(id, statusRunning, nomad.Task{Name: "server", Restarts: 1, LastRestart: at})
	}

	allocs := []nomad.Alloc{
		restart("aaaa1111", ago(59*time.Minute+59*time.Second)),
		restart("bbbb2222", ago(10*time.Minute)),
		restart("cccc3333", ago(5*time.Minute)),
	}

	m := newTestModel(&fakeClient{allocs: allocs, changes: newChanges()})
	m, _ = m.update(openMsg{allocationsPage{read: read, allocs: allocs, fault: faultRestarting}})

	m, _ = m.update(key('j'))
	m, cmd := m.update(enter())
	m = playOut(m, cmd)

	r.IsType(tasksPage{}, m.screen.page)
	r.Equal("bbbb2222", m.screen.page.(tasksPage).allocID)
}

func TestAllocFaults_ANewAnswerSetsTheClockOfTheRead(t *testing.T) {
	r := require.New(t)

	now := time.Now()
	old := allocFaultsAlloc("aaaa1111", statusRunning,
		nomad.Task{Name: "server", Restarts: 1, LastRestart: now.Add(-2*time.Hour - 10*time.Minute)})
	allocs := []nomad.Alloc{old}

	m := newTestModel(&fakeClient{allocs: allocs, changes: newChanges()})
	m, _ = m.update(openMsg{allocationsPage{read: now.Add(-2 * time.Hour), allocs: allocs, fault: faultRestarting}})
	r.Equal("Allocations (production, restarting) [1]", m.title())

	m, _ = m.update(allocsMsg(allocs))
	r.Equal("Allocations (production, restarting) [0]", m.title())
}

func TestAllocFaults_LogsJudgeTheListByItsReadNotByTheClock(t *testing.T) {
	r := require.New(t)

	now := time.Now()
	old := allocFaultsAlloc("aaaa1111", statusRunning,
		nomad.Task{Name: "server", Restarts: 1, LastRestart: now.Add(-2*time.Hour - 10*time.Minute)})

	client := &fakeClient{
		allocs:      []nomad.Alloc{old},
		changes:     newChanges(),
		logsByAlloc: map[string]*nomad.LogStream{"aaaa1111": writing()},
	}
	m := newTestModel(client)
	m, _ = m.update(openMsg{allocationsPage{read: now.Add(-2 * time.Hour), allocs: client.allocs, fault: faultRestarting}})

	m, cmd := m.update(key('l'))
	m = playOut(m, cmd)

	r.NotContains(plain(m.render()), "has no allocation running")
	r.Equal([]string{"aaaa1111"}, client.logsOpened)
}
