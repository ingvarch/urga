package ui

import (
	"testing"

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
	r.Equal(screenTaskGroups, m.screen.kind)

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
	r.Equal(screenAllocations, m.screen.kind)

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

	_, cmd := m.update(enter())
	drain(m, cmd)

	r.Equal(1, client.scaled)
	r.Equal(5, client.scaledTo)
	r.Equal("frontend", client.askedGroup)
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
