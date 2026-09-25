package ui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func allocsWithEvents() []nomad.Alloc {
	allocs := twoAllocs()

	allocs[0].Tasks = []nomad.Task{{
		Name:  "server",
		State: "running",
		Events: []nomad.TaskEvent{
			{Time: time.Now().Add(-time.Minute), Type: "Terminated", Message: "Exit Code: 1", Failed: true},
			{Time: time.Now().Add(-time.Hour), Type: "Started", Message: "Task started by client"},
		},
	}}

	return allocs
}

// onTasks walks to the tasks of an allocation that has something to tell.
func onTasks(t *testing.T, allocs []nomad.Alloc) Model {
	t.Helper()

	m := newTestModel(&fakeClient{jobs: twoJobs(), allocs: allocs})
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(enter())
	m, _ = m.update(allocsMsg(allocs))
	m, _ = m.update(enter())

	return m
}

func TestTaskEvents_OpenOnATask(t *testing.T) {
	r := require.New(t)

	m := onTasks(t, allocsWithEvents())
	m, _ = m.update(key('e'))

	r.IsType(taskEventsPage{}, m.screen.page)

	out := plain(m.render())
	r.Contains(out, "Events (Task: server) [2]")

	// What happened to the task, newest first, in the words of the client
	// that ran it.
	r.Contains(out, "Terminated")
	r.Contains(out, "Exit Code: 1")
	r.Contains(out, "Started")
}

func TestTaskEvents_WhatTookTheTaskDownStandsOut(t *testing.T) {
	r := require.New(t)

	rows := taskEventRows(allocsWithEvents()[0].Tasks[0].Events)

	r.Len(rows, 2)
	r.Equal(colorDead, rows[0].color)
	r.Nil(rows[1].color)
}

func TestTaskEvents_ATaskThatNothingHappenedTo(t *testing.T) {
	r := require.New(t)

	m := onTasks(t, twoAllocs())
	m, _ = m.update(key('e'))

	// The screen opens anyway and says it holds nothing, rather than the
	// key doing nothing at all.
	r.IsType(taskEventsPage{}, m.screen.page)
	r.Contains(plain(m.render()), "[0]")
}

func TestTaskEvents_HaveNoKeysOfTheirOwn(t *testing.T) {
	r := require.New(t)

	m := onTasks(t, allocsWithEvents())
	m, _ = m.update(key('e'))

	r.Empty(m.hints())
}

func TestTaskEvents_AnAnswerForAnotherAllocationIsDropped(t *testing.T) {
	r := require.New(t)

	other := allocsWithEvents()[0]
	other.ID = "b2222222-0000-0000-0000-000000000000"
	other.Tasks[0].Events = []nomad.TaskEvent{{Time: time.Now(), Type: "Killed", Message: "Sent interrupt"}}

	m := onTasks(t, allocsWithEvents())
	m, _ = m.update(key('e'))
	m, _ = m.update(allocMsg(other))

	out := plain(m.render())
	r.Contains(out, "Exit Code: 1")
	r.NotContains(out, "Sent interrupt")
}
