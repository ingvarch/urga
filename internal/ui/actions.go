package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// doneMsg is what came of an action.
type doneMsg struct {
	said string
	err  error

	// kept are the marks that outlive the action: what did not happen is
	// still marked, so the same key tries it again, and what did happen is
	// let go of, so the key does not undo it.
	kept map[string]bool
}

// The buttons of a question, in the order they are walked.
const (
	buttonCancel = iota
	buttonConfirm
)

// confirmModel is the question before something in the cluster changes, and
// which of its buttons the cursor is on.
type confirmModel struct {
	question string
	apply    tea.Cmd

	// choice is the button under the cursor. It starts on cancel: enter out
	// of habit then changes nothing in the cluster.
	choice int
}

// view is the question as a box to put over the screen.
func (c confirmModel) view(width int) string {
	return dialog("Confirm", []string{
		styleText.Render(c.question),
		"",
		button("cancel", c.choice == buttonCancel) + "   " + button("confirm", c.choice == buttonConfirm),
	}, width)
}

// button is one of the two of a question, filled when the cursor is on it.
func button(label string, on bool) string {
	if on {
		return styleButtonOn.Render(" " + label + " ")
	}

	return styleButton.Render(" " + label + " ")
}

// ask puts a question up. Nothing is asked of the cluster until it is
// answered.
func (m Model) ask(question string, apply tea.Cmd) (Model, tea.Cmd) {
	m.overlay = overlayConfirm
	m.confirm = confirmModel{question: question, apply: apply}
	m.layout()

	return m, nil
}

// confirmKey answers the question, and nothing else while it is up.
func (m Model) confirmKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "left", "shift+tab", "h":
		m.confirm.choice = buttonCancel

	case "right", "tab", "l":
		m.confirm.choice = buttonConfirm

	case "enter":
		if m.confirm.choice == buttonConfirm {
			return m.applyConfirm()
		}

		return m.closeConfirm(), nil

	case "y":
		return m.applyConfirm()

	case "esc", "n":
		return m.closeConfirm(), nil
	}

	return m, nil
}

// applyConfirm does what was asked about. The marks stay until it is known
// to have worked: what did not happen is still marked, and the same key
// tries it again.
func (m Model) applyConfirm() (Model, tea.Cmd) {
	apply := m.confirm.apply

	return m.closeConfirm(), apply
}

// closeConfirm takes the question off the screen.
func (m Model) closeConfirm() Model {
	m.overlay = overlayNone
	m.confirm = confirmModel{}
	m.layout()

	return m
}

// act runs one change against the cluster and says what came of it.
func act(said string, do func(ctx context.Context) error) tea.Cmd {
	return request(
		func(ctx context.Context) (string, error) { return said, do(ctx) },
		func(said string) tea.Msg { return doneMsg{said: said} },
	)
}

// startStopJob stops a job that runs, starts one that is dead.
func startStopJob(m Model) (Model, tea.Cmd) {
	jobs := marked(m, screenJobs, m.jobs)
	if len(jobs) == 0 {
		return m, nil
	}

	client := m.client

	// Each job is asked to do what it is not doing, so a question about
	// several of them says what they have in common, or both things when
	// they have nothing.
	dead := func(job nomad.Job) bool { return job.Status == statusDead }

	verb := bothWays(jobs, dead, "start", "stop", "start or stop")
	done := bothWays(jobs, dead, "Started", "Stopped", "Changed")

	return m.ask(
		fmt.Sprintf("Really %s %s?", verb, jobLabel(jobs)),
		each(done, jobLabel(jobs), jobs, jobMark, func(ctx context.Context, job nomad.Job) error {
			if job.Status == statusDead {
				return client.StartJob(ctx, job.Namespace, job.ID)
			}

			return client.StopJob(ctx, job.Namespace, job.ID)
		}),
	)
}

// jobLabel is what a question about jobs says.
func jobLabel(jobs []nomad.Job) string {
	return many(len(jobs), "the job "+jobs[0].ID, "jobs")
}

// revertJob puts the version before the one that runs back in place, which
// is submitting it again: it is planned first, like any submit.
func revertJob(m Model) (Model, tea.Cmd) {
	job, ok := selectedOf(m, screenJobs, m.jobs)
	if !ok {
		return m, nil
	}

	return m, planFor(m.client, planState{revert: true, namespace: job.Namespace, jobID: job.ID})
}

// restartAllocation restarts every task of an allocation.
func restartAllocation(m Model) (Model, tea.Cmd) {
	client := m.client

	return m.askEachAlloc("restart", "Restarted", func(ctx context.Context, alloc nomad.Alloc) error {
		return client.RestartAllocation(ctx, alloc.Namespace, alloc.ID)
	})
}

// stopAllocation stops an allocation. The scheduler places a new one when the
// job still asks for it.
func stopAllocation(m Model) (Model, tea.Cmd) {
	client := m.client

	return m.askEachAlloc("stop", "Stopped", func(ctx context.Context, alloc nomad.Alloc) error {
		return client.StopAllocation(ctx, alloc.Namespace, alloc.ID)
	})
}

// askEachAlloc asks about the allocations an action is to take, and then
// takes each of them on its own: one that will not answer must not stop the
// rest, and a screenful of them must not share one timeout.
func (m Model) askEachAlloc(verb, done string, do func(context.Context, nomad.Alloc) error) (Model, tea.Cmd) {
	if !m.screen.listsAllocs() {
		return m, nil
	}

	allocs := marked(m, m.screen.kind, m.visibleAllocs())
	if len(allocs) == 0 {
		return m, nil
	}

	label := allocLabel(allocs)

	return m.ask(
		fmt.Sprintf("Really %s %s?", verb, label),
		each(done, label, allocs, allocMark, do),
	)
}

// each runs the action against every resource on its own: one that will not
// answer must not stop the rest, and a screenful of them must not share one
// timeout. What went through is reported, and what did not stays marked.
func each[T any](done, label string, items []T, mark func(T) string, do func(context.Context, T) error) tea.Cmd {
	return func() tea.Msg {
		out := doneMsg{said: fmt.Sprintf("%s %s.", done, label), kept: map[string]bool{}}

		went := 0
		failed := []string{}

		for _, item := range items {
			ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
			err := do(ctx, item)

			cancel()

			if err != nil {
				failed = append(failed, err.Error())
				out.kept[mark(item)] = true

				continue
			}

			went++
		}

		if len(failed) > 0 {
			out.err = fmt.Errorf("%s %d of %d: %s", strings.ToLower(done), went, len(items), failed[0])
		}

		return out
	}
}
