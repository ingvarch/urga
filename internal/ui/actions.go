package ui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// doneMsg is what came of an action.
type doneMsg struct {
	said string
	err  error
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
		c.button("cancel", buttonCancel) + "   " + c.button("confirm", buttonConfirm),
	}, width)
}

// button is one of the two, filled when the cursor is on it.
func (c confirmModel) button(label string, at int) string {
	if c.choice == at {
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
			return m.closeConfirm(), m.confirm.apply
		}

		return m.closeConfirm(), nil

	case "y":
		return m.closeConfirm(), m.confirm.apply

	case "esc", "n":
		return m.closeConfirm(), nil
	}

	return m, nil
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
func (m Model) startStopJob() (Model, tea.Cmd) {
	job, ok := selectedOf(m, screenJobs, m.jobs)
	if !ok {
		return m, nil
	}

	client := m.client

	if job.Status == statusDead {
		return m.ask(
			fmt.Sprintf("Really start the job %s?", job.ID),
			act(fmt.Sprintf("Job %s started.", job.ID), func(ctx context.Context) error {
				return client.StartJob(ctx, job.Namespace, job.ID)
			}),
		)
	}

	return m.ask(
		fmt.Sprintf("Really stop the job %s?", job.ID),
		act(fmt.Sprintf("Job %s stopped.", job.ID), func(ctx context.Context) error {
			return client.StopJob(ctx, job.Namespace, job.ID)
		}),
	)
}

// revertJob puts the version before the one that runs back in place.
func (m Model) revertJob() (Model, tea.Cmd) {
	job, ok := selectedOf(m, screenJobs, m.jobs)
	if !ok {
		return m, nil
	}

	client := m.client

	return m.ask(
		fmt.Sprintf("Really revert the job %s to its previous version?", job.ID),
		act(fmt.Sprintf("Job %s reverted.", job.ID), func(ctx context.Context) error {
			return client.RevertJob(ctx, job.Namespace, job.ID)
		}),
	)
}

// restartAllocation restarts every task of an allocation.
func (m Model) restartAllocation() (Model, tea.Cmd) {
	alloc, ok := selectedOf(m, screenAllocations, m.visibleAllocs())
	if !ok {
		return m, nil
	}

	client := m.client
	short := shortID(alloc.ID)

	return m.ask(
		fmt.Sprintf("Really restart the allocation %s?", short),
		act(fmt.Sprintf("Allocation %s restarted.", short), func(ctx context.Context) error {
			return client.RestartAllocation(ctx, alloc.Namespace, alloc.ID)
		}),
	)
}

// stopAllocation stops an allocation. The scheduler places a new one when the
// job still asks for it.
func (m Model) stopAllocation() (Model, tea.Cmd) {
	alloc, ok := selectedOf(m, screenAllocations, m.visibleAllocs())
	if !ok {
		return m, nil
	}

	client := m.client
	short := shortID(alloc.ID)

	return m.ask(
		fmt.Sprintf("Really stop the allocation %s?", short),
		act(fmt.Sprintf("Allocation %s stopped.", short), func(ctx context.Context) error {
			return client.StopAllocation(ctx, alloc.Namespace, alloc.ID)
		}),
	)
}
