package ui

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func mixedJobs() []nomad.Job {
	return []nomad.Job{
		{ID: "api", Name: "api", Namespace: "production", Type: "service", Status: "running", Running: 3, Desired: 3},
		{ID: "web", Name: "web", Namespace: "production", Type: "service", Status: "running", Running: 1, Desired: 3},
		{ID: "broken", Name: "broken", Namespace: "production", Type: "service", Status: "dead"},
		{ID: "nightly", Name: "nightly", Namespace: "production", Type: "batch", Status: "dead"},
	}
}

func TestTrouble_KeepsWhatIsNotRight(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{jobs: mixedJobs()})
	m, _ = m.update(jobsMsg(mixedJobs()))

	m, _ = m.update(key('!'))

	out := plain(m.render())

	// A service short of allocations and a dead one are what is wrong. A
	// batch job that ended did its work, and a service that runs everything
	// it asks for is fine.
	r.Contains(out, "web")
	r.Contains(out, "broken")
	r.NotContains(out, "nightly")
	r.NotContains(out, " api ")

	// The line at the bottom says the list is narrowed and by how much.
	r.Contains(out, "2 of 4")

	m, _ = m.update(key('!'))
	r.Contains(plain(m.render()), "nightly")
}

func TestTrouble_CursorFollowsTheRowsThatAreLeft(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{jobs: mixedJobs()})
	m, _ = m.update(jobsMsg(mixedJobs()))

	m, _ = m.update(key('!'))
	m, _ = m.update(enter())

	// Enter opens the first row that is left, not the first row of the whole
	// list.
	r.Equal("web", m.screen.jobID)
}

func TestTrouble_NothingWrong(t *testing.T) {
	r := require.New(t)

	jobs := []nomad.Job{{ID: "api", Name: "api", Namespace: "production", Type: "service", Status: "running", Running: 1, Desired: 1}}

	m := newTestModel(&fakeClient{jobs: jobs})
	m, _ = m.update(jobsMsg(jobs))

	m, _ = m.update(key('!'))

	// An empty list with the toggle on reads as good news, not as an empty
	// cluster.
	out := plain(m.render())
	r.Contains(out, "0 of 1")
	r.NotContains(out, " api ")
}
