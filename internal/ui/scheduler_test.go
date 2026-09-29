package ui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// onScheduler is the screen of the scheduler, opened by name.
func onScheduler(t *testing.T, client *fakeClient, editor *fakeEditor) Model {
	t.Helper()

	// Every field differs from the ones next to it.
	client.scheduler = nomad.SchedulerConfig{
		Algorithm: "spread", PreemptSystem: true, PreemptBatch: true, MemoryOversubscription: true, PauseEvalBroker: true,
	}

	m := New(client, Options{Namespace: "production", Version: "v-test", Editor: editor, PollEvery: time.Millisecond})
	m, _ = m.update(sizeMsg())

	return typeCommand(m, "scheduler")
}

func TestScheduler_ShowsHowTheClusterPlacesWork(t *testing.T) {
	r := require.New(t)

	m := onScheduler(t, &fakeClient{}, &fakeEditor{})

	r.Contains(plain(m.render()), "Scheduler")

	rows := m.screen.page.rows(m.env())

	for i, field := range [][2]string{
		{"Algorithm", "spread"},
		{"System preemption", "yes"},
		{"Sysbatch preemption", "no"},
		{"Batch preemption", "yes"},
		{"Service preemption", "no"},
		{"Memory oversubscription", "yes"},
		{"Reject job registration", "no"},
		{"Pause eval broker", "yes"},
	} {
		r.Equal(field[:], rows[i].cells, field[0])
	}

	// It belongs to the cluster: the next run opens it again.
	r.Equal(schedulerView, m.screen.view)
}

func TestScheduler_EditSavesOverWhatWasRead(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{schedulerSpec: `{"SchedulerAlgorithm": "spread", "ModifyIndex": 42}`}
	editor := &fakeEditor{replace: `{"SchedulerAlgorithm": "binpack", "ModifyIndex": 42}`}
	m := onScheduler(t, client, editor)

	m, cmd := m.update(key('e'))
	m = follow(m, cmd, 5)

	r.Equal([]string{"SubmitScheduler"}, client.writes)
	r.Equal(`{"SchedulerAlgorithm": "binpack", "ModifyIndex": 42}`, client.submittedSource)
	r.Contains(plain(m.render()), "Scheduler configuration saved.")
}

func TestScheduler_AnEditOverAChangedOneOpensAgain(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{schedulerSpec: `{"ModifyIndex": 42}`, refusals: []error{nomad.ErrSchedulerChanged}}
	editor := &fakeEditor{edits: []string{`{"SchedulerAlgorithm": "binpack", "ModifyIndex": 42}`}}
	m := onScheduler(t, client, editor)

	m, cmd := m.update(key('e'))
	_ = follow(m, cmd, 16)

	// What was typed is not lost.
	r.Len(editor.seen, 2)
	r.Equal(reasonFor(nomad.ErrSchedulerChanged.Error())+`{"SchedulerAlgorithm": "binpack", "ModifyIndex": 42}`, editor.seen[1])
}

func TestScheduler_CopiesTheValueUnderTheCursor(t *testing.T) {
	r := require.New(t)

	m := onScheduler(t, &fakeClient{}, &fakeEditor{})

	_, cmd := m.update(key('c'))
	r.NotNil(cmd)
	r.Equal("spread", clipboardOf(cmd))
}

func TestScheduler_NothingBeforeTheClusterAnswers(t *testing.T) {
	// A "no" would read as a setting.
	require.Empty(t, schedulerPage{}.rows(env{}))
}
