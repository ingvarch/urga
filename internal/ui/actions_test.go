package ui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func ctrlKey(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Mod: tea.ModCtrl}
}

// answerYes walks the cursor to the confirm button and presses it, the way a
// person answers the question. The cursor starts on cancel.
func answerYes(m Model) (Model, tea.Cmd) {
	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyRight})

	return m.update(enter())
}

func TestAction_StopAJobAsks(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs()}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, _ = m.update(ctrlKey('s'))

	// The cluster is not touched before the question is answered.
	r.Equal(overlayConfirm, m.overlay)
	r.Contains(plain(m.render()), "stop the job web")
	r.Zero(client.stopped)

	m, cmd := answerYes(m)
	r.Equal(overlayNone, m.overlay)

	m = drain(m, cmd)
	r.Equal(1, client.stopped)
	r.Equal("production", client.askedNamespace)

	// What happened is said in one line, not in a window that has to be
	// clicked away.
	r.Contains(plain(m.render()), "Stopped the job web")
}

func TestAction_CancelLeavesTheClusterAlone(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs()}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, _ = m.update(ctrlKey('s'))
	m, cmd := m.update(escape())

	r.Equal(overlayNone, m.overlay)
	r.Nil(cmd)
	r.Zero(client.stopped)
}

func TestAction_StartADeadJob(t *testing.T) {
	r := require.New(t)

	jobs := []nomad.Job{{ID: "cron", Name: "cron", Namespace: "production", Type: "batch", Status: "dead"}}
	client := &fakeClient{jobs: jobs}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(jobs))

	m, _ = m.update(ctrlKey('s'))

	// A job that is already dead is started, not stopped.
	r.Contains(plain(m.render()), "start the job cron")

	_, cmd := answerYes(m)
	drain(m, cmd)

	r.Equal(1, client.started)
}

func TestAction_RestartAnAllocation(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(enter())
	m, _ = m.update(allocsMsg(twoAllocs()))

	m, _ = m.update(key('r'))
	r.Contains(plain(m.render()), "restart the allocation af1f37df")

	m, cmd := answerYes(m)
	m = drain(m, cmd)

	r.Equal(1, client.restarted)
	r.Contains(plain(m.render()), "Restarted the allocation af1f37df")
}

func TestAction_StopAnAllocation(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(enter())
	m, _ = m.update(allocsMsg(twoAllocs()))

	m, _ = m.update(ctrlKey('k'))
	_, cmd := answerYes(m)
	drain(m, cmd)

	r.Equal(1, client.stoppedAllocs)
}

func TestAction_RevertAJob(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), plan: nomad.Plan{To: 3, Version: 4, Index: 42}}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	// Going back is submitting an older version, and it is planned first
	// like any other submit: the version before the one that runs.
	m, cmd := m.update(key('u'))
	m = drain(m, cmd)

	r.Nil(client.plannedTo)
	r.IsType(planPage{}, m.screen.page)
	r.Contains(plain(m.render()), "Revert (Job: web, Version: 3)")

	m, cmd = m.update(key('y'))
	m = drain(m, cmd)

	// From the version it was planned at: a job that moved on is not moved
	// back.
	r.Equal(uint64(3), client.revertedTo)
	r.Equal(uint64(4), client.revertedFrom)
	r.Contains(plain(m.render()), "Job web reverted to version 3")
}

func TestAction_FailureIsSaid(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), actionErr: errors.New("permission denied")}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, _ = m.update(ctrlKey('s'))
	m, cmd := answerYes(m)
	m = drain(m, cmd)

	r.Contains(plain(m.render()), "permission denied")
}

func TestAction_ConfirmTakesTheKeys(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs()}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, _ = m.update(ctrlKey('s'))

	// While the question is up, the list does not move and q does not quit.
	m, cmd := m.update(key('q'))
	r.Nil(cmd)
	r.Equal(overlayConfirm, m.overlay)

	m, _ = m.update(key('j'))
	r.Zero(m.list.table.cursor)
}
