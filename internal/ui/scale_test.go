package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func twoGroups() []nomad.TaskGroup {
	return []nomad.TaskGroup{
		{Name: "frontend", JobID: "web", Count: 3, Running: 2, Starting: 1},
		{Name: "backend", JobID: "web", Count: 1, Running: 1},
	}
}

func openTaskGroups(t *testing.T, client *fakeClient) Model {
	t.Helper()

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(key('t'))
	m, _ = m.update(taskGroupsMsg(twoGroups()))

	return m
}

func TestTaskGroups_OpenFromAJob(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), groups: twoGroups()}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, cmd := m.update(key('t'))
	r.IsType(taskGroupsPage{}, m.screen.page)

	m = drain(m, cmd)

	// The groups of that job, asked for in its namespace.
	r.Equal("production", client.askedNamespace)
	r.Equal("web", client.askedID)

	out := plain(m.render())
	r.Contains(out, "Task Groups (Job: web) [2]")
	r.Contains(out, "frontend")
	r.Contains(out, "3/3")
	r.Contains(out, "backend")
}

func TestTaskGroups_EnterOpensTheirAllocations(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), groups: twoGroups(), allocs: twoAllocs()}
	m := openTaskGroups(t, client)

	m, cmd := m.update(enter())
	r.IsType(allocationsPage{}, m.screen.page)

	m = drain(m, cmd)
	m, _ = m.update(allocsMsg(twoAllocs()))

	// Only the allocations of that group.
	out := plain(m.render())
	r.Contains(out, "frontend")
	r.NotContains(out, "backend")
}

func TestScale_AsksForACount(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), groups: twoGroups()}
	m := openTaskGroups(t, client)

	m, _ = m.update(key('s'))

	// The line comes up with the count that runs now, ready to be changed.
	r.Equal(overlayScale, m.overlay)
	r.True(m.overlay.asksForALine())
	r.Contains(plain(m.render()), "scale frontend to:")
	r.Equal("3", m.prompt.text)

	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = typeIn(m, "5")

	m, _ = m.update(enter())

	// A count starts or stops allocations like any other action, and is
	// asked about like one: the question says what runs and what will.
	r.Equal(overlayConfirm, m.overlay)
	r.Contains(plain(m.render()), "Really scale frontend of web from 3 to 5?")
	r.Zero(client.scaled)

	_, cmd := m.update(key('y'))
	drain(m, cmd)

	r.Equal(1, client.scaled)
	r.Equal(5, client.scaledTo)
	r.Equal("frontend", client.askedGroup)
}

func TestScale_CancelLeavesTheCount(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), groups: twoGroups()}
	m := openTaskGroups(t, client)

	m, _ = m.update(key('s'))
	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = typeIn(m, "0")
	m, _ = m.update(enter())
	r.Equal(overlayConfirm, m.overlay)

	// Enter out of habit lands on cancel: a typo of 0 stops nothing.
	m, cmd := m.update(enter())
	drain(m, cmd)

	r.Equal(overlayNone, m.overlay)
	r.Zero(client.scaled)
}

func TestScale_RefusesWhatIsNotACount(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), groups: twoGroups()}
	m := openTaskGroups(t, client)

	m, _ = m.update(key('s'))
	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = typeIn(m, "lots")

	m, cmd := m.update(enter())
	m = drain(m, cmd)

	r.Zero(client.scaled)
	r.Contains(plain(m.render()), "not a count")
}

// groupsElsewhere opens the groups of web, a job of default, from the jobs
// of every namespace, and runs what the key starts.
func groupsElsewhere(t *testing.T, client *fakeClient) Model {
	t.Helper()

	jobs := []nomad.Job{{ID: "web", Name: "web", Namespace: "default", Type: "service", Status: "running"}}
	client.jobs = jobs

	m := New(client, Options{Namespace: nomad.AllNamespaces, Version: "v-test", PollEvery: time.Millisecond})
	m, _ = m.update(sizeMsg())
	m, _ = m.update(jobsMsg(jobs))

	m, cmd := m.update(key('t'))

	return drain(m, cmd)
}

func TestTaskGroups_TheKeysOfAGroup(t *testing.T) {
	r := require.New(t)

	groups := twoGroups()
	groups[1].Queued = 1

	m := openTaskGroups(t, &fakeClient{jobs: twoJobs(), groups: groups})
	m, _ = m.update(taskGroupsMsg(groups))

	keys := []hint{
		{Key: "<enter>", Description: "Allocations"},
		{Key: "<s>", Description: "Scale"},
		{Key: "<l>", Description: "Logs"},
	}
	r.Equal(keys, m.hints())

	// A group with an allocation that waits for a place has a why to ask.
	m, _ = m.update(key('j'))
	r.Equal([]hint{keys[0], keys[1], {Key: "<p>", Description: "Placement"}, keys[2]}, m.hints())
}

func TestTaskGroups_AreAskedWhereTheJobLives(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{groups: twoGroups()}
	m := groupsElsewhere(t, client)

	r.Equal("default", client.askedNamespace)
	r.Equal("web", client.askedID)
	r.Contains(plain(m.render()), "Task Groups (Job: web) [2]")

	// The allocations of the group, of that job, where it lives.
	m, cmd := m.update(enter())
	drain(m, cmd)

	r.Equal("default", client.askedNamespace)
	r.Equal("web", client.askedJobID)
	r.Contains(plain(m.render()), "Allocations (Group: frontend)")
}

func TestScale_WhereTheJobLives(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{groups: twoGroups()}
	m := groupsElsewhere(t, client)

	m, _ = m.update(key('s'))
	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = typeIn(m, "5")
	m, _ = m.update(enter())

	_, cmd := answerYes(m)
	drain(m, cmd)

	r.Equal(1, client.scaled)
	r.Equal("default", client.askedNamespace)
	r.Equal("web", client.askedID)
	r.Equal("frontend", client.askedGroup)
}

func TestTaskGroups_PlacementAndLogsWhereTheJobLives(t *testing.T) {
	r := require.New(t)

	groups := twoGroups()
	groups[0].Queued = 1

	allocs := webAllocs("server")
	for i := range allocs {
		allocs[i].Namespace = "default"
	}

	client := &fakeClient{
		groups: groups, evaluation: shortOfRoom(), allocs: allocs,
		logsByAlloc: map[string]*nomad.LogStream{newer: writing(), older: writing()},
	}
	m := groupsElsewhere(t, client)

	next, cmd := m.update(key('p'))
	drain(next, cmd)

	r.Equal("default", client.askedNamespace)
	r.Equal("web", client.askedJobID)

	client.askedNamespace, client.askedJobID = "", ""

	next, cmd = m.update(key('l'))
	next = playOut(next, cmd)

	r.Equal("default", client.askedNamespace)
	r.Equal("web", client.askedJobID)
	r.IsType(jobLogsPage{}, next.screen.page)
	r.Contains(plain(next.render()), "Logs (Job: web, Task: server)")
}

func TestTaskGroups_AnAnswerAfterLeavingIsDropped(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), groups: twoGroups()}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(key('t'))
	m, _ = m.update(escape())

	// The groups answer on the jobs they were asked from.
	m, _ = m.update(taskGroupsMsg(twoGroups()))

	out := plain(m.render())
	r.Contains(out, "Jobs (production) [2]")
	r.NotContains(out, "frontend")
}
