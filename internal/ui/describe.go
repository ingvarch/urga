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
}

// describeCmd asks for what the cursor is on, in the words of the cluster.
// Nothing to describe answers with nothing.
func (m Model) describeCmd() tea.Cmd {
	client := m.client

	switch m.screen.kind {
	case screenJobs:
		job, ok := selectedOf(m, screenJobs, m.jobs)
		if !ok {
			return nil
		}

		return describe(fmt.Sprintf("Job: %s", job.ID), func(ctx context.Context) (string, error) {
			return client.DescribeJob(ctx, job.Namespace, job.ID)
		})

	case screenAllocations:
		alloc, ok := selectedOf(m, screenAllocations, m.visibleAllocs())
		if !ok {
			return nil
		}

		return describe(fmt.Sprintf("Allocation: %s", shortID(alloc.ID)), func(ctx context.Context) (string, error) {
			return client.DescribeAllocation(ctx, alloc.Namespace, alloc.ID)
		})

	case screenDeployments:
		deployment, ok := selectedOf(m, screenDeployments, m.deployments)
		if !ok {
			return nil
		}

		return describe(fmt.Sprintf("Deployment: %s", shortID(deployment.ID)), func(ctx context.Context) (string, error) {
			return client.DescribeDeployment(ctx, deployment.Namespace, deployment.ID)
		})

	case screenServices:
		service, ok := selectedOf(m, screenServices, m.services)
		if !ok {
			return nil
		}

		return describe(fmt.Sprintf("Service: %s", service.Name), func(ctx context.Context) (string, error) {
			return client.DescribeService(ctx, service.Namespace, service.Name)
		})
	}

	return nil
}

// jobSpecCmd asks for the file the job was submitted with.
func (m Model) jobSpecCmd() tea.Cmd {
	job, ok := selectedOf(m, screenJobs, m.jobs)
	if !ok {
		return nil
	}

	client := m.client

	return describe(fmt.Sprintf("Job spec: %s", job.ID), func(ctx context.Context) (string, error) {
		source, err := client.JobSpec(ctx, job.Namespace, job.ID)

		// The cluster has no file to show. Saying so is the job of the
		// screen, the client answers with an error and no prose.
		if errors.Is(err, nomad.ErrNoSource) {
			return fmt.Sprintf(
				"The cluster kept no source for %s.\n\n"+
					"It was registered before submissions were stored, or through the API\n"+
					"without one. <e> edits what the cluster does have of it.", job.ID), nil
		}

		return source, err
	})
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

	return m.stackText(next, msg.content), nil
}
