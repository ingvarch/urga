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

// The screens that can be described each ask for what the cursor is on, in
// the words of the cluster. Nothing to describe answers with nothing.

func describeJob(m Model) (Model, tea.Cmd) {
	job, ok := selectedOf(m, screenJobs, m.jobs)
	if !ok {
		return m, nil
	}

	client := m.client

	return m, describe(fmt.Sprintf("Job: %s", job.ID), func(ctx context.Context) (string, error) {
		return client.DescribeJob(ctx, job.Namespace, job.ID)
	})
}

func describeAllocation(m Model) (Model, tea.Cmd) {
	alloc, ok := selectedOf(m, screenAllocations, m.visibleAllocs())
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

func describeService(m Model) (Model, tea.Cmd) {
	service, ok := selectedOf(m, screenServices, m.services)
	if !ok {
		return m, nil
	}

	client := m.client

	return m, describe(fmt.Sprintf("Service: %s", service.Name), func(ctx context.Context) (string, error) {
		return client.DescribeService(ctx, service.Namespace, service.Name)
	})
}

// showJobSpec asks for the file the job under the cursor was submitted with.
func showJobSpec(m Model) (Model, tea.Cmd) {
	job, ok := selectedOf(m, screenJobs, m.jobs)
	if !ok {
		return m, nil
	}

	client := m.client

	return m, describe(fmt.Sprintf("Job spec: %s", job.ID), func(ctx context.Context) (string, error) {
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
