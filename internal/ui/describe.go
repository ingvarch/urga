package ui

import (
	"context"
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// describeMsg is a description the cluster answered with.
type describeMsg struct {
	label   string
	content string

	// lines are a description urga drew, some of it in a colour of its
	// own; it takes the place of content.
	lines []paintedLine
}

// The screens that can be described each ask for what the cursor is on, in
// the words of the cluster. Nothing to describe answers with nothing.

func describeJob(p jobsPage, e env) (jobsPage, outcome) {
	job, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	client := e.client

	return p, outcome{cmd: describe(fmt.Sprintf("Job: %s", job.ID), func(ctx context.Context) (string, error) {
		return client.DescribeJob(ctx, job.Namespace, job.ID)
	})}
}

func describeAllocation(m Model) (Model, tea.Cmd) {
	alloc, ok := m.selectedAlloc()
	if !ok {
		return m, nil
	}

	client := m.client

	return m, describe(fmt.Sprintf("Allocation: %s", shortID(alloc.ID)), func(ctx context.Context) (string, error) {
		return client.DescribeAllocation(ctx, alloc.Namespace, alloc.ID)
	})
}

func describeDeployment(m Model) (Model, tea.Cmd) {
	deployment, ok := selectedOf(m, screenDeployments, m.deployments)
	if !ok {
		return m, nil
	}

	client := m.client

	return m, describe(fmt.Sprintf("Deployment: %s", shortID(deployment.ID)), func(ctx context.Context) (string, error) {
		return client.DescribeDeployment(ctx, deployment.Namespace, deployment.ID)
	})
}

// showJobSpec asks for the file the job under the cursor was submitted with.
func showJobSpec(p jobsPage, e env) (jobsPage, outcome) {
	job, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	client := e.client

	return p, outcome{cmd: describe(fmt.Sprintf("Job spec: %s", job.ID), func(ctx context.Context) (string, error) {
		spec, err := client.JobSpec(ctx, job.Namespace, job.ID)

		// The cluster has no file to show. Saying so is the job of the
		// screen, the client answers with an error and no prose.
		if errors.Is(err, nomad.ErrNoSource) {
			return fmt.Sprintf(
				"The cluster kept no source for %s.\n\n"+
					"It was registered before submissions were stored, or through the API\n"+
					"without one. <e> edits what the cluster does have of it.", job.ID), nil
		}

		return spec.Source, err
	})}
}

func describe(label string, load func(ctx context.Context) (string, error)) tea.Cmd {
	return request(load, func(content string) tea.Msg {
		return describeMsg{label: label, content: content}
	})
}

// showDescribe puts a description on the screen, on top of the list it was
// asked from.
func (m Model) showDescribe(msg describeMsg) (Model, tea.Cmd) {
	next := screen{kind: screenDescribe, namespace: m.screen.namespace, label: msg.label}

	text := newTextModel(msg.content)
	if msg.lines != nil {
		text = paintedText(msg.lines)
	}

	return m.stackText(next, text), nil
}

// describeLines is describe for a description urga draws in colour.
func describeLines(label string, load func(ctx context.Context) ([]paintedLine, error)) tea.Cmd {
	return request(load, func(lines []paintedLine) tea.Msg {
		return describeMsg{label: label, lines: lines}
	})
}
