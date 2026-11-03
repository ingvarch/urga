package ui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func drainingClient(t *testing.T, d *nomad.DrainStrategy, allocs []nomad.Alloc) Model {
	t.Helper()

	nodes := busyClient()
	nodes[0].Drain, nodes[0].DrainStrategy = true, d

	client := &fakeClient{nodes: nodes, nodeAllocs: allocs, use: map[string]nomad.ResourceUse{"node-1": {}}}

	m, _ := nodeModelOf(client)
	m, cmd := m.update(enter())
	m = drain(m, cmd)
	m.now = fixedTime()

	return m
}

func allocLike(status string, change func(*nomad.Alloc)) nomad.Alloc {
	alloc := clientAllocs()[0]
	alloc.Status = status

	if change != nil {
		change(&alloc)
	}

	return alloc
}

func TestClient_ADrainingClientSaysWhatIsLeftToMoveAndItsDeadline(t *testing.T) {
	r := require.New(t)

	then := fixedTime()

	m := drainingClient(t, &nomad.DrainStrategy{
		Started:  then.Add(-10 * time.Minute),
		Deadline: then.Add(42*time.Minute + 30*time.Second),
	}, []nomad.Alloc{
		allocLike("running", nil),
		allocLike("pending", nil),
		allocLike("running", func(a *nomad.Alloc) { a.DesiredStatus = "stop" }),
		allocLike("complete", func(a *nomad.Alloc) { a.Modified = then.Add(-5 * time.Minute) }),
		allocLike("complete", func(a *nomad.Alloc) { a.Modified = then.Add(-2 * time.Hour) }),
	})

	r.Contains(bodyOf(m), "Left to move 3 of 4")
	r.Contains(bodyOf(m), "Deadline in 42m")
}

func TestClient_ADrainThatLeavesSystemJobsDoesNotCountThem(t *testing.T) {
	r := require.New(t)

	m := drainingClient(t, &nomad.DrainStrategy{
		Started:          fixedTime().Add(-time.Minute),
		IgnoreSystemJobs: true,
	}, []nomad.Alloc{
		allocLike("running", nil),
		allocLike("running", func(a *nomad.Alloc) { a.JobType = "system" }),
	})

	r.Contains(bodyOf(m), "Left to move 1 of 1")
}

func TestClient_ADeadlineThatPassedSaysSoAndNoDeadlineShowsNone(t *testing.T) {
	r := require.New(t)

	then := fixedTime()

	m := drainingClient(t, &nomad.DrainStrategy{Started: then.Add(-time.Hour), Deadline: then.Add(-time.Minute)},
		clientAllocs())
	r.Contains(bodyOf(m), "Deadline passed")

	m = drainingClient(t, &nomad.DrainStrategy{Started: then.Add(-time.Hour), Deadline: then}, clientAllocs())
	r.Contains(bodyOf(m), "Deadline passed")

	m = drainingClient(t, &nomad.DrainStrategy{Started: then.Add(-time.Hour)}, clientAllocs())
	r.NotContains(bodyOf(m), "Deadline")
	r.Contains(bodyOf(m), "Left to move")
}

func TestClient_DoesNotCountWhatIsLeftToMoveBeforeTheAllocationsAreRead(t *testing.T) {
	r := require.New(t)

	nodes := busyClient()
	nodes[0].Drain = true
	client := &fakeClient{nodes: nodes, nodeAllocs: clientAllocs()}

	m, _ := nodeModelOf(client)
	m, _ = m.update(enter())

	drained := nodes[0]
	drained.DrainStrategy = &nomad.DrainStrategy{Started: fixedTime()}
	m, _ = m.update(hostMsg(drained))
	m.now = fixedTime()

	r.Contains(bodyOf(m), "Left to move n/a")
	r.NotContains(bodyOf(m), "Left to move 0 of 0")
}

func TestClient_AClientThatDoesNotDrainShowsNothingOfADrain(t *testing.T) {
	r := require.New(t)

	m, _ := onClient(t)
	r.NotContains(bodyOf(m), "Left to move")
	r.NotContains(bodyOf(m), "Deadline")

	m = drainingClient(t, &nomad.DrainStrategy{Started: fixedTime()}, clientAllocs())
	r.Contains(bodyOf(m), "Left to move")

	// The drain ended, or the cluster is too old to send its strategy.
	m, _ = m.update(hostMsg(nomad.Node{ID: "node-1", Drain: true}))
	r.NotContains(bodyOf(m), "Left to move")
}
