package ui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// drainNode starts or stops moving the work off the client under the cursor.
func (m Model) drainNode() (Model, tea.Cmd) {
	node, ok := selectedOf(m, screenNodes, m.nodes)
	if !ok {
		return m, nil
	}

	client := m.client

	if node.Drain {
		return m.ask(
			fmt.Sprintf("Really stop draining the client %s?", node.Name),
			act(fmt.Sprintf("Client %s takes work again.", node.Name), func(ctx context.Context) error {
				return client.DrainNode(ctx, node.ID, false)
			}),
		)
	}

	return m.ask(
		fmt.Sprintf("Really drain the client %s? Its allocations move elsewhere.", node.Name),
		act(fmt.Sprintf("Client %s draining.", node.Name), func(ctx context.Context) error {
			return client.DrainNode(ctx, node.ID, true)
		}),
	)
}

// toggleEligibility says whether the client may be given new work.
func (m Model) toggleEligibility() (Model, tea.Cmd) {
	node, ok := selectedOf(m, screenNodes, m.nodes)
	if !ok {
		return m, nil
	}

	client := m.client
	eligible := node.Eligibility != "eligible"

	question := fmt.Sprintf("Really stop giving new work to %s?", node.Name)
	said := fmt.Sprintf("Client %s takes no new work.", node.Name)

	if eligible {
		question = fmt.Sprintf("Really give new work to %s again?", node.Name)
		said = fmt.Sprintf("Client %s takes new work.", node.Name)
	}

	return m.ask(question, act(said, func(ctx context.Context) error {
		return client.SetNodeEligible(ctx, node.ID, eligible)
	}))
}

// promoteDeployment takes the canaries of the deployment under the cursor
// into service.
func (m Model) promoteDeployment() (Model, tea.Cmd) {
	deployment, ok := selectedOf(m, screenDeployments, m.deployments)
	if !ok {
		return m, nil
	}

	client := m.client

	return m.ask(
		fmt.Sprintf("Really promote the canaries of %s?", deployment.JobID),
		act(fmt.Sprintf("Deployment of %s promoted.", deployment.JobID), func(ctx context.Context) error {
			return client.PromoteDeployment(ctx, deployment.Namespace, deployment.ID)
		}),
	)
}

// failDeployment stops a deployment where it is.
func (m Model) failDeployment() (Model, tea.Cmd) {
	deployment, ok := selectedOf(m, screenDeployments, m.deployments)
	if !ok {
		return m, nil
	}

	client := m.client

	return m.ask(
		fmt.Sprintf("Really fail the deployment of %s? It rolls back where the job says to.", deployment.JobID),
		act(fmt.Sprintf("Deployment of %s failed.", deployment.JobID), func(ctx context.Context) error {
			return client.FailDeployment(ctx, deployment.Namespace, deployment.ID)
		}),
	)
}

var (
	nodeHints = []hint{
		{Key: "<ctrl-d>", Description: "Drain"},
		{Key: "<i>", Description: "Eligibility"},
	}

	deploymentHints = []hint{
		{Key: "<d>", Description: "Describe"},
		{Key: "<p>", Description: "Promote canaries"},
		{Key: "<f>", Description: "Fail"},
	}
)
