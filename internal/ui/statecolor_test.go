package ui

import (
	"image/color"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// hue is the hue of a colour in degrees, 0 to 360.
func hue(c color.Color) float64 {
	r16, g16, b16, _ := c.RGBA()
	r, g, b := float64(r16)/0xffff, float64(g16)/0xffff, float64(b16)/0xffff

	high, low := max(r, g, b), min(r, g, b)
	if high == low {
		return 0
	}

	spread := high - low

	var h float64

	switch high {
	case r:
		h = (g - b) / spread
	case g:
		h = 2 + (b-r)/spread
	default:
		h = 4 + (r-g)/spread
	}

	h *= 60
	if h < 0 {
		h += 360
	}

	return h
}

func TestStateColours_ReadLikeK9s(t *testing.T) {
	r := require.New(t)

	// What waits to start is orange, what needs a look is yellow, what goes
	// away is purple: the colours k9s gives a pod in the same state.
	r.InDelta(25, hue(colorPending), 15, "pending is orange")
	r.InDelta(45, hue(colorAttention), 10, "attention is yellow")
	r.InDelta(260, hue(colorStopping), 20, "stopping is purple")

	// Every state reads as its own colour down the list.
	states := []color.Color{colorPending, colorAttention, colorDead, colorSpent, colorStopping, colorCanary}
	for i, a := range states {
		for _, b := range states[i+1:] {
			r.NotEqual(a, b)
		}
	}
}

func TestClusterColours_KeepTheirNames(t *testing.T) {
	r := require.New(t)

	// The settings name a colour: swapping what pending and attention mean
	// must not paint an orange cluster yellow.
	r.Equal(colorPending, clusterColours["orange"])
	r.Equal(colorAttention, clusterColours["yellow"])
	r.Equal(colorCanary, clusterColours["purple"])
}

func TestAllocColor_TheLifeOfAnAllocation(t *testing.T) {
	r := require.New(t)

	running := func(desired, health string) nomad.Alloc {
		return nomad.Alloc{Status: statusRunning, DesiredStatus: desired, Health: health}
	}

	// Placed, its tasks not started yet.
	r.Equal(colorPending, allocColor(nomad.Alloc{Status: statusPending}))

	// Started, its deployment still waits for the checks to pass, or saw
	// them fail.
	r.Equal(colorDead, allocColor(running("run", "checking")))
	r.Equal(colorDead, allocColor(running("run", "unhealthy")))

	// Up and healthy, or up outside a deployment.
	r.Nil(allocColor(running("run", "healthy")))
	r.Nil(allocColor(running("run", "")))

	// Told to stop, still running: on its way out.
	r.Equal(colorStopping, allocColor(running("stop", "healthy")))
	r.Equal(colorStopping, allocColor(running("evict", "")))

	r.Equal(colorDead, allocColor(nomad.Alloc{Status: statusFailed}))
	r.Equal(colorDead, allocColor(nomad.Alloc{Status: statusLost}))
	r.Equal(colorSpent, allocColor(nomad.Alloc{Status: statusComplete}))
}

func TestAllocColor_GoingAwayOutranksTrouble(t *testing.T) {
	r := require.New(t)

	// What is being stopped is purple, whatever its tasks did on the way.
	alloc := nomad.Alloc{
		Status: statusRunning, DesiredStatus: desiredStop, Health: "unhealthy",
		Tasks: []nomad.Task{{Name: "server", LastRestart: time.Now().Add(-time.Minute)}},
	}

	r.Equal(colorStopping, allocColor(alloc))
}

func TestAllocColor_APoststopTaskDoesNotHoldItBack(t *testing.T) {
	r := require.New(t)

	// A poststop task waits for the others to end: it is pending all the
	// time the allocation runs, and says nothing about how it runs.
	alloc := nomad.Alloc{
		Status: statusRunning, DesiredStatus: "run",
		Tasks: []nomad.Task{{Name: "server", State: statusRunning}, {Name: "cleanup", State: statusPending}},
	}

	r.Nil(allocColor(alloc))
}

func TestTrouble_LeavesOutWhatIsGoingAway(t *testing.T) {
	r := require.New(t)

	allocs := []nomad.Alloc{
		{ID: "stopping", Status: statusRunning, DesiredStatus: desiredStop},
		{ID: "checking", Status: statusRunning, DesiredStatus: "run", Health: "checking"},
		{ID: "pending", Status: statusPending},
	}

	rows, index := troubledRows(allocRows(allocs, nil), []int{0, 1, 2})

	// A stop somebody asked for is not trouble; a start that is not healthy
	// yet is red, and red rows are what the trouble filter keeps.
	r.Len(rows, 1)
	r.Equal([]int{1}, index)
}

func TestJobColor_TheLifeOfAJob(t *testing.T) {
	r := require.New(t)

	service := func(running, desired, queued int) nomad.Job {
		return nomad.Job{Type: typeService, Status: statusRunning, Running: running, Desired: desired, Queued: queued}
	}

	// Waits for the cluster to find room for it.
	r.Equal(colorPending, jobColor(nomad.Job{Type: typeService, Status: statusPending}))
	r.Equal(colorPending, jobColor(service(2, 3, 1)))

	// Placed, not all of it up yet.
	r.Equal(colorDead, jobColor(service(2, 3, 0)))
	r.Nil(jobColor(service(3, 3, 0)))

	// More up than it asks for: a rollout or a scale down not done yet.
	r.Equal(colorDead, jobColor(service(4, 3, 0)))

	// A batch job runs as many as it has work for: no count to fall short of.
	r.Nil(jobColor(nomad.Job{Type: typeBatch, Status: statusRunning, Running: 1, Desired: 3}))

	// Stopped on purpose while its allocations still run.
	stopping := service(3, 3, 0)
	stopping.Stopped = true
	r.Equal(colorStopping, jobColor(stopping))
}

func TestDeadJobs_AServiceStillStartingIsNotDead(t *testing.T) {
	r := require.New(t)

	// Red for a start that is not done is not red for a job that died.
	starting := nomad.Job{ID: "web", Type: typeService, Status: statusRunning, Running: 1, Desired: 3}
	r.False(isDead(starting, nil))

	r.True(isDead(nomad.Job{ID: "web", Type: typeService, Status: statusDead}, nil))

	// A service that died with nothing up is short of allocations too, and
	// still dead.
	r.True(isDead(nomad.Job{ID: "web", Type: typeService, Status: statusDead, Running: 0, Desired: 2}, nil))
}

func TestTaskGroupColor_TheLifeOfAGroup(t *testing.T) {
	r := require.New(t)

	// Allocations that wait for room, or wait to start.
	r.Equal(colorPending, taskGroupColor(nomad.TaskGroup{Count: 3, Running: 2, Queued: 1}))
	r.Equal(colorPending, taskGroupColor(nomad.TaskGroup{Count: 3, Running: 2, Starting: 1}))

	// Short of what it asks for, nothing on the way.
	r.Equal(colorDead, taskGroupColor(nomad.TaskGroup{Count: 3, Running: 2}))

	// An allocation failed or lost.
	r.Equal(colorDead, taskGroupColor(nomad.TaskGroup{Count: 3, Running: 3, Failed: 1}))
	r.Equal(colorDead, taskGroupColor(nomad.TaskGroup{Count: 3, Running: 3, Lost: 1}))

	r.Nil(taskGroupColor(nomad.TaskGroup{Count: 3, Running: 3}))
}
