package ui

import (
	"errors"
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

	client := &fakeClient{schedulerSpec: nomad.SchedulerSource{Source: `{"SchedulerAlgorithm": "spread"}`, Index: 42}}
	editor := &fakeEditor{replace: `{"SchedulerAlgorithm": "binpack"}`}
	m := onScheduler(t, client, editor)

	m, cmd := m.update(key('e'))
	m = follow(m, cmd, 5)

	r.Equal([]string{"SubmitScheduler"}, client.writes)
	r.Equal(`{"SchedulerAlgorithm": "binpack"}`, client.submittedSource)
	r.Equal(uint64(42), client.submittedIndex)
	r.Contains(plain(m.render()), "Scheduler configuration saved.")
}

func TestScheduler_AnEditOverAChangedOneOpensAgainAtTheNewIndex(t *testing.T) {
	r := require.New(t)

	edit := `{"SchedulerAlgorithm": "binpack"}`
	conflict := &nomad.SchedulerConflict{Index: 77}
	client := &fakeClient{schedulerSpec: nomad.SchedulerSource{Source: `{}`, Index: 42}, refusals: []error{conflict}}

	// The second time, the lines of the refusal are deleted.
	editor := &fakeEditor{edits: []string{edit, edit}}
	m := onScheduler(t, client, editor)

	m, cmd := m.update(key('e'))
	m = follow(m, cmd, 16)

	// What was typed is not lost, and the advice says what saving does.
	r.Len(editor.seen, 2)
	r.Equal("# Not saved: "+conflict.Error()+".\n# Saving again replaces that change.\n"+
		"# To save, change the file: delete these lines at least.\n# To drop your edit, quit without saving.\n"+edit, editor.seen[1])

	r.Equal([]string{"SubmitScheduler", "SubmitScheduler"}, client.writes)
	r.Equal(uint64(77), client.submittedIndex)
	r.Contains(plain(m.render()), "Scheduler configuration saved.")
}

func TestScheduler_AnotherRefusalReopensAtTheSameIndex(t *testing.T) {
	r := require.New(t)

	edit := `{"SchedulerAlgorithm": "binpack"}`
	client := &fakeClient{schedulerSpec: nomad.SchedulerSource{Source: `{}`, Index: 42}, refusals: []error{errors.New("not valid JSON")}}
	editor := &fakeEditor{edits: []string{edit, edit}}
	m := onScheduler(t, client, editor)

	m, cmd := m.update(key('e'))
	_ = follow(m, cmd, 16)

	r.NotContains(editor.seen[1], "Saving again")
	r.Equal(uint64(42), client.submittedIndex)
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
