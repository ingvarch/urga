package ui

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// schedulerMsg is how the scheduler of the cluster places work.
type schedulerMsg nomad.SchedulerConfig

// schedulerPage is how the scheduler of the cluster places work. It belongs
// to the cluster, not to a namespace.
type schedulerPage struct {
	ofTheSession

	// config is what the cluster answered, once read.
	config nomad.SchedulerConfig
	read   bool
}

func (schedulerPage) title(env, int) string { return "Scheduler" }
func (schedulerPage) titles() []string      { return fieldTitles }

// topics: none. The cluster sends no event when the configuration changes;
// the page is polled.
func (schedulerPage) topics() []string { return nil }

func (schedulerPage) fetch(e env) tea.Cmd {
	return request(e.client.Scheduler, func(config nomad.SchedulerConfig) tea.Msg { return schedulerMsg(config) })
}

func (p schedulerPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	config, ok := msg.(schedulerMsg)
	if !ok {
		return p, outcome{}, false
	}

	p.config, p.read = nomad.SchedulerConfig(config), true

	return p, outcome{}, true
}

// rows are the fields of the configuration. Nothing before the cluster
// answers: a "no" would read as a setting.
func (p schedulerPage) rows(env) []tableRow {
	if !p.read {
		return nil
	}

	c := p.config

	return []tableRow{
		{cells: []string{"Algorithm", c.Algorithm}},
		{cells: []string{"System preemption", yesNo(c.PreemptSystem)}},
		{cells: []string{"Sysbatch preemption", yesNo(c.PreemptSysBatch)}},
		{cells: []string{"Batch preemption", yesNo(c.PreemptBatch)}},
		{cells: []string{"Service preemption", yesNo(c.PreemptService)}},
		{cells: []string{"Memory oversubscription", yesNo(c.MemoryOversubscription)}},
		{cells: []string{"Reject job registration", yesNo(c.RejectJobRegistration)}},
		{cells: []string{"Pause eval broker", yesNo(c.PauseEvalBroker)}},
	}
}

var schedulerKeys = []pageKey[schedulerPage]{
	{press: "e", label: "Edit", do: editScheduler, writes: true},
	copyKey[schedulerPage](),
}

func (p schedulerPage) keys(e env) []keyHint { return hintsOf(p, e, schedulerKeys) }

func (p schedulerPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, schedulerKeys, k)
}

// editScheduler opens the configuration in the editor, with the index it was
// read at: the cluster keeps a configuration that changed since.
func editScheduler(p schedulerPage, e env) (schedulerPage, outcome) {
	return p, outcome{cmd: openEditor(schedulerFile(e.client))}
}

// schedulerFile is the configuration as a file, saved over the version it was
// read at.
func schedulerFile(client schedulerClient) load {
	return func(ctx context.Context) (file, error) {
		spec, err := client.SchedulerSpec(ctx)

		return file{extension: "json", content: spec.Source, submit: saveScheduler(client, spec.Index)}, err
	}
}

// saveScheduler sends an edit of the configuration read at index. A refusal
// goes back to the editor instead of being lost; one for a change made since
// is sent at the index the configuration has now, so saving again replaces
// that change.
func saveScheduler(client schedulerClient, index uint64) func(source string) tea.Cmd {
	return func(source string) tea.Cmd {
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
			defer cancel()

			err := client.SubmitScheduler(ctx, source, index)
			if err == nil {
				return doneMsg{said: "Scheduler configuration saved."}
			}

			next, advice := index, ""

			var conflict *nomad.SchedulerConflict
			if errors.As(err, &conflict) {
				next, advice = conflict.Index, "Saving again replaces that change."
			}

			again := file{extension: "json", content: source, submit: saveScheduler(client, next)}

			return refusedEditMsg{err: err, advice: advice, again: again}
		}
	}
}
