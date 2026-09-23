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

// confirmModel is the question before something in the cluster changes.
type confirmModel struct {
	question string
	apply    tea.Cmd
}

func (c confirmModel) view(width int) string {
	lines := []string{
		"",
		styleText.Render(c.question),
		"",
		styleKey.Render("<enter>") + styleText.Render(" confirm") +
			"    " + styleKey.Render("<esc>") + styleText.Render(" cancel"),
	}

	return center(lines, width)
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
	apply := m.confirm.apply

	switch msg.String() {
	case "enter", "y":
		m.overlay = overlayNone
		m.confirm = confirmModel{}
		m.layout()

		return m, apply

	case "esc", "n":
		m.overlay = overlayNone
		m.confirm = confirmModel{}
		m.layout()
	}

	return m, nil
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
