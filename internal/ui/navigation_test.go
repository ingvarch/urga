package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func enter() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyEnter} }
func escape() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyEscape}
}

// headerOf is the top of the screen, without the box under it.
func headerOf(m Model) string {
	rows := lines(m.render())

	return strings.Join(rows[screenPadTop:screenPadTop+headerHeight], "\n")
}

func twoNamespaces() []nomad.Namespace {
	return []nomad.Namespace{{Name: "production"}, {Name: "staging"}}
}

func twoAllocs() []nomad.Alloc {
	return []nomad.Alloc{
		{
			ID:        "af1f37df-7b19-6b1c-da67-5e8f482b5a15",
			Namespace: "production",
			JobID:     "web",
			TaskGroup: "frontend",
			NodeName:  "node-01",
			Status:    "running",
			Tasks: []nomad.Task{
				{Name: "server", State: "running"},
				{Name: "sidecar", State: "dead", Failed: true},
			},
		},
		{ID: "b2222222-0000-0000-0000-000000000000", Namespace: "production", JobID: "web", TaskGroup: "backend", Status: "pending"},
	}
}

func TestNavigation_EnterOpensTheAllocationsOfAJob(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, cmd := m.update(enter())
	r.Equal(screenAllocations, m.screen.kind)
	r.Equal("web", m.screen.jobID)

	// The allocations are asked for in the namespace of that job, not in the
	// one the session happens to look at.
	r.NotNil(cmd)
	m = drain(m, cmd)

	r.Equal("production", client.askedNamespace)
	r.Equal("web", client.askedJobID)
	r.Contains(plain(m.render()), "Allocations (Job: web) [2]")
	r.Contains(plain(m.render()), "frontend")
}

func TestNavigation_EnterOpensTheTasksOfAnAllocation(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{jobs: twoJobs(), allocs: twoAllocs()})
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(enter())
	m, _ = m.update(allocsMsg(twoAllocs()))

	m, _ = m.update(enter())

	r.Equal(screenTasks, m.screen.kind)

	out := plain(m.render())
	r.Contains(out, "Tasks (Allocation: af1f37df) [2]")
	r.Contains(out, "server")
	r.Contains(out, "sidecar")
}

func TestNavigation_EscapeWalksBack(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{jobs: twoJobs(), allocs: twoAllocs()})
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(enter())
	m, _ = m.update(allocsMsg(twoAllocs()))
	m, _ = m.update(enter())

	r.Equal(screenTasks, m.screen.kind)

	m, _ = m.update(escape())
	r.Equal(screenAllocations, m.screen.kind)

	m, _ = m.update(escape())
	r.Equal(screenJobs, m.screen.kind)

	// The job list is the floor, escape on it stays there.
	m, _ = m.update(escape())
	r.Equal(screenJobs, m.screen.kind)
}

func TestNavigation_AnswerForAScreenThatWasLeftIsDropped(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{jobs: twoJobs(), allocs: twoAllocs()})
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(enter())

	// The job list answers after the allocations were opened. It belongs to
	// a screen that is gone, so it does not show up under the wrong title.
	m, _ = m.update(jobsMsg(twoJobs()))

	r.Equal(screenAllocations, m.screen.kind)
	r.Contains(plain(m.render()), "Allocations (Job: web) [0]")
}

func TestNavigation_EnterOnAnEmptyListDoesNothing(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{})

	m, cmd := m.update(enter())

	r.Equal(screenJobs, m.screen.kind)
	r.Nil(cmd)
}

func TestNavigation_HeaderShowsTheKeysOfTheScreen(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{jobs: twoJobs(), allocs: twoAllocs()})
	m, _ = m.update(jobsMsg(twoJobs()))

	// The job list offers what a job can do.
	r.Contains(headerOf(m), "Allocations")

	m, _ = m.update(enter())
	m, _ = m.update(allocsMsg(twoAllocs()))

	// The allocation list offers what an allocation can do, and nothing of
	// the screen before it.
	r.Contains(headerOf(m), "Tasks")
	r.NotContains(headerOf(m), "Allocations")
}

func TestNavigation_PollKeepsToTheOpenScreen(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(enter())

	client.calls = 0

	_, cmd := m.update(pollMsg{})
	r.NotNil(cmd)
	r.IsType(allocsMsg{}, cmd())

	// A poll asks for what is on the screen, not for the job list.
	r.Zero(client.calls)
	r.Equal(1, client.allocCalls)
}

func TestNavigation_AScreenStartsWithoutTheLastOnesFilter(t *testing.T) {
	r := require.New(t)

	lines := make(chan string, 1)
	lines <- "the task says something\n"

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), describe: "{}", logs: &nomad.LogStream{Lines: lines}}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(enter())
	m, _ = m.update(allocsMsg(twoAllocs()))

	// A filter that belongs to the allocation list.
	m, _ = m.update(key('/'))
	m = typeIn(m, "frontend")
	m, _ = m.update(enter())
	r.Equal("frontend", m.filter)

	// Into the tasks, and a filter of that list as well.
	m, _ = m.update(enter())
	r.Empty(m.filter)

	m, _ = m.update(key('/'))
	m = typeIn(m, "server")
	m, _ = m.update(enter())
	r.Equal("server", m.filter)

	// The output of a task is not filtered by what the task list was
	// filtered to.
	m, cmd := m.update(enter())
	m = drain(m, cmd)
	m = drain(m, m.waitForLog())

	r.Empty(m.filter)
	r.Contains(plain(m.render()), "the task says something")
}

func TestNavigation_DescribeStartsWithoutTheFilter(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), describe: "{\n  \"ID\": \"web\"\n}"}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, _ = m.update(key('/'))
	m = typeIn(m, "web")
	m, _ = m.update(enter())

	m, cmd := m.update(key('d'))
	m = drain(m, cmd)

	// The description is shown whole, not narrowed by what the list was
	// filtered to.
	r.Empty(m.filter)
	r.Contains(plain(m.render()), "\"ID\": \"web\"")
}

func TestAllocations_WithoutAJob(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{allocs: twoAllocs()})
	m, _ = m.update(key(':'))
	m = typeIn(m, "allocations")
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	// The command line opens the allocations of the namespace rather than
	// of a job, and the title says so instead of naming a job that is not
	// there.
	r.Contains(plain(m.render()), "Allocations (production)")
	r.NotContains(plain(m.render()), "Job: )")
}
