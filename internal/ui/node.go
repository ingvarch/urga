package ui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// drainNode starts or stops moving the work off the client under the cursor.
func drainNode(m Model) (Model, tea.Cmd) {
	nodes := marked(m, screenNodes, m.nodes)
	if len(nodes) == 0 {
		return m, nil
	}

	client := m.client

	// Each machine is asked to do what it is not doing, so a question about
	// several of them says what they have in common, or both things when
	// they have nothing.
	draining := func(node nomad.Node) bool { return node.Drain }

	verb := bothWays(nodes, draining, "stop draining", "drain", "change the draining of")
	done := bothWays(nodes, draining, "Stopped draining", "Draining", "Changed")

	// Draining moves the work off the machine; stopping a drain moves
	// nothing, it stops the moving.
	aside := " Allocations move elsewhere."
	if allOf(nodes, draining) {
		aside = ""
	}

	return m.ask(
		fmt.Sprintf("Really %s %s?%s", verb, nodeLabel(nodes), aside),
		each(done, nodeLabel(nodes), nodes, nodeMark, func(ctx context.Context, node nomad.Node) error {
			return client.DrainNode(ctx, node.ID, !node.Drain)
		}),
	)
}

// nodeLabel is what a question about clients says.
func nodeLabel(nodes []nomad.Node) string {
	return many(len(nodes), "the client "+nodes[0].Name, "clients")
}

// toggleEligibility says whether the client may be given new work.
func toggleEligibility(m Model) (Model, tea.Cmd) {
	nodes := marked(m, screenNodes, m.nodes)
	if len(nodes) == 0 {
		return m, nil
	}

	client := m.client

	eligible := func(node nomad.Node) bool { return node.Eligibility == "eligible" }

	verb := bothWays(nodes, eligible, "take new work from", "give new work back to",
		"change the work of")
	done := bothWays(nodes, eligible, "Took new work from", "Gave new work back to",
		"Changed the work of")

	return m.ask(
		fmt.Sprintf("Really %s %s?", verb, nodeLabel(nodes)),
		each(done, nodeLabel(nodes), nodes, nodeMark, func(ctx context.Context, node nomad.Node) error {
			return client.SetNodeEligible(ctx, node.ID, node.Eligibility != "eligible")
		}),
	)
}

// allOf says every one of them answers the same way.
func allOf[T any](items []T, yes func(T) bool) bool {
	for _, item := range items {
		if !yes(item) {
			return false
		}
	}

	return true
}

// bothWays is how a question or a report names an action that reads one way
// for some of what it is about and the other way for the rest. Rows that
// disagree get a phrase of their own: two stuck together with an "or" is not
// a sentence.
func bothWays[T any](items []T, yes func(T) bool, whenYes, whenNo, whenBoth string) string {
	some, rest := false, false

	for _, item := range items {
		if yes(item) {
			some = true
		} else {
			rest = true
		}
	}

	switch {
	case some && rest:
		return whenBoth
	case some:
		return whenYes
	}

	return whenNo
}

// promoteDeployment takes the canaries of the deployment under the cursor
// into service.
func promoteDeployment(m Model) (Model, tea.Cmd) {
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
func failDeployment(m Model) (Model, tea.Cmd) {
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
	nodeBindings = []binding{
		{press: "enter", label: "What it runs", do: openClient},
		{press: "ctrl+d", label: "Drain", do: drainNode},
		{press: "i", label: "Eligibility", do: toggleEligibility},
		{press: "space", label: "Mark", do: mark},
		{press: "ctrl+a", label: "Mark all", do: markAll},
	}

	deploymentBindings = []binding{
		{press: "d", label: "Describe", do: describeDeployment},
		{press: "p", label: "Promote canaries", do: promoteDeployment},
		{press: "f", label: "Fail", do: failDeployment},
	}
)
