package ui

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

const mlAlloc = "af1f37df-7b19-6b1c-da67-5e8f482b5a15"

// inML is a job and an allocation of the namespace ml, seen from a session
// that looks at every namespace.
func inML() ([]nomad.Job, []nomad.Alloc) {
	jobs := []nomad.Job{{ID: "web", Name: "web", Namespace: "ml", Type: "service", Status: "running", Running: 1, Desired: 1}}
	allocs := []nomad.Alloc{{
		ID: mlAlloc, Namespace: "ml", JobID: "web", TaskGroup: "frontend", Status: "running",
		Tasks: []nomad.Task{{Name: "server", State: "running"}},
	}}

	return jobs, allocs
}

// onEventsOfAJob is the job list of every namespace, and the events of the
// job under the cursor opened with o.
func onEventsOfAJob(t *testing.T) (Model, *fakeClient, *bool) {
	t.Helper()

	jobs, allocs := inML()
	feed, closed := feedOf(someEvents()...)
	client := &fakeClient{jobs: jobs, allocs: allocs, alloc: allocs[0], feed: feed}

	m := newTestModel(client)
	m.namespace = nomad.AllNamespaces
	m, _ = m.update(jobsMsg(jobs))
	m, cmd := m.update(key('o'))

	return playOut(m, cmd), client, closed
}

func TestEventsOf_AJobKeepsToItsOwnNamespace(t *testing.T) {
	r := require.New(t)

	m, client, _ := onEventsOfAJob(t)

	r.Equal("Events (Job: web) [2]", m.title())
	r.Equal("ml", client.feedNamespace)
	r.Equal(nomad.Subject{JobID: "web"}, client.feedSubject)
	r.Equal(uint64(1), client.feedFrom)
}

func TestEventsOf_AnAllocationOfEveryList(t *testing.T) {
	r := require.New(t)

	_, allocs := inML()
	allocs[0].NodeID = "node-1"

	opens := map[string]func(*fakeClient) Model{
		"allocations": func(c *fakeClient) Model {
			m := newTestModel(c)
			m.namespace = nomad.AllNamespaces
			m, _ = m.update(openMsg{allocationsPage{allocs: allocs}})

			return m
		},
		"tasks": func(c *fakeClient) Model {
			m := newTestModel(c)
			m.namespace = nomad.AllNamespaces
			m, _ = m.update(openMsg{listedTasks(allocs[0])})

			return m
		},
		"client": func(c *fakeClient) Model {
			c.nodes, c.nodeAllocs = busyClient(), allocs
			m, _ := nodeModelOf(c)
			m, cmd := m.update(enter())

			return drain(m, cmd)
		},
	}

	for name, open := range opens {
		feed, _ := feedOf(someEvents()...)
		client := &fakeClient{allocs: allocs, alloc: allocs[0], feed: feed}

		m := open(client)
		m, cmd := m.update(key('o'))
		m = playOut(m, cmd)

		r.Equal("Events (Allocation: af1f37df) [2]", m.title(), name)
		r.Equal("ml", client.feedNamespace, name)
		r.Equal(nomad.Subject{AllocID: mlAlloc}, client.feedSubject, name)
	}
}

func TestEventsOf_SwitchingNamespaceLeavesThemAlone(t *testing.T) {
	r := require.New(t)

	m, client, closed := onEventsOfAJob(t)
	m.namespaceOrder = []string{"production", "ml"}
	feed := m.screen.page.(feedPage).feed
	client.feedNamespace, client.feedFrom = "", 0

	m, cmd := m.update(key('1'))
	m = playOut(m, cmd)

	r.Equal("Events (Job: web) [2]", m.title())
	r.False(m.screen.followsSession())
	r.False(*closed)
	r.Empty(client.feedNamespace)
	r.Zero(client.feedFrom)
	r.Same(feed, m.screen.page.(feedPage).feed)
}

func TestEventsOf_ComingBackGoesOnInTheSameSubject(t *testing.T) {
	r := require.New(t)

	m, client, _ := onEventsOfAJob(t)

	// Into the allocation of the newest event, and back.
	client.feed, _ = feedOf()

	m, cmd := m.update(enter())
	m = playOut(m, cmd)
	r.IsType(tasksPage{}, m.screen.page)

	m, cmd = m.update(escape())
	m = playOut(m, cmd)

	r.Equal("Events (Job: web) [2]", m.title())
	r.Equal("ml", client.feedNamespace)
	r.Equal(nomad.Subject{JobID: "web"}, client.feedSubject)
	r.Equal(uint64(12), client.feedFrom)
}

func TestEventsOf_ByNameStillFollowTheSession(t *testing.T) {
	r := require.New(t)

	feed, _ := feedOf(someEvents()...)
	client := &fakeClient{feed: feed}
	m := onEvents(t, client)

	r.Equal(nomad.Subject{}, client.feedSubject)
	r.True(m.screen.followsSession())
}

func TestEventsOf_AJobThatLaunchesOthersHasNone(t *testing.T) {
	r := require.New(t)

	events := hint{Key: "<o>", Description: "Events"}

	m := newTestModel(&fakeClient{jobs: periodicJobs()})
	m, _ = m.update(jobsMsg(periodicJobs()))
	r.Contains(m.hints(), events)

	// A periodic job has almost no events of its own: its launches do.
	m, _ = m.update(key('j'))
	r.NotContains(m.hints(), events)

	m, cmd := m.update(key('o'))
	r.Nil(cmd)
	r.IsType(jobsPage{}, m.screen.page)
}

func TestEventsOf_ReadOnlyOffersThem(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{jobs: twoJobs(), allocs: twoAllocs()})
	m.opts.ReadOnly = true
	m, _ = m.update(jobsMsg(twoJobs()))
	r.Contains(m.hints(), hint{Key: "<o>", Description: "Events"})

	m, _ = m.update(openMsg{allocationsPage{allocs: twoAllocs()}})
	r.Contains(m.hints(), hint{Key: "<o>", Description: "Alloc Events"})
}
