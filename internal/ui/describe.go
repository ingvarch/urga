package ui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// describeMsg is a description the cluster answered with.
type describeMsg struct {
	label   string
	content string
}

// describeCmd asks for what the cursor is on, in the words of the cluster.
// Nothing to describe answers with nothing.
func (m Model) describeCmd() tea.Cmd {
	row, ok := m.selectedIndex()
	if !ok {
		return nil
	}

	client := m.client

	switch m.screen.kind {
	case screenJobs:
		job := m.jobs[row]

		return describe(fmt.Sprintf("Job: %s", job.ID), func(ctx context.Context) (string, error) {
			return client.DescribeJob(ctx, job.Namespace, job.ID)
		})

	case screenAllocations:
		alloc := m.allocs[row]

		return describe(fmt.Sprintf("Allocation: %s", shortID(alloc.ID)), func(ctx context.Context) (string, error) {
			return client.DescribeAllocation(ctx, alloc.Namespace, alloc.ID)
		})

	case screenDeployments:
		deployment := m.deployments[row]

		return describe(fmt.Sprintf("Deployment: %s", shortID(deployment.ID)), func(ctx context.Context) (string, error) {
			return client.DescribeDeployment(ctx, deployment.Namespace, deployment.ID)
		})

	case screenServices:
		service := m.services[row]

		return describe(fmt.Sprintf("Service: %s", service.Name), func(ctx context.Context) (string, error) {
			return client.DescribeService(ctx, service.Namespace, service.Name)
		})
	}

	return nil
}

// jobSpecCmd asks for the file the job was submitted with.
func (m Model) jobSpecCmd() tea.Cmd {
	row, ok := m.selectedIndex()
	if !ok || m.screen.kind != screenJobs {
		return nil
	}

	job := m.jobs[row]
	client := m.client

	return describe(fmt.Sprintf("Job spec: %s", job.ID), func(ctx context.Context) (string, error) {
		return client.JobSpec(ctx, job.Namespace, job.ID)
	})
}

func describe(label string, load func(ctx context.Context) (string, error)) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		content, err := load(ctx)
		if err != nil {
			return errMsg{err: err}
		}

		return describeMsg{label: label, content: content}
	}
}

// showDescribe puts a description on the screen, on top of the list it was
// asked from.
func (m Model) showDescribe(msg describeMsg) (Model, tea.Cmd) {
	m.history = append(m.history, m.screen)
	m.screen = screen{kind: screenDescribe, namespace: m.screen.namespace, label: msg.label}
	m.err = nil

	m.text = newTextModel(msg.content)
	m.layout()

	return m, nil
}

// textKey scrolls the description under the window.
func (m Model) textKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	switch msg.String() {
	case "up", "k":
		m.text.move(-1)
	case "down", "j":
		m.text.move(1)
	case "pgup", "ctrl+b":
		m.text.move(-m.text.height)
	case "pgdown", "ctrl+f":
		m.text.move(m.text.height)
	case "g", "home":
		m.text.move(-len(m.text.lines))
	case "G", "end":
		m.text.move(len(m.text.lines))
	default:
		return m, nil, false
	}

	return m, nil, true
}
