package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// drainNode starts or stops moving the work off the client under the cursor.
func (m Model) drainNode() (Model, tea.Cmd) {
	nodes := marked(m, screenNodes, m.nodes)
	if len(nodes) == 0 {
		return m, nil
	}

	client := m.client

	// Each machine is asked to do what it is not doing, so a question about
	// several of them says what they have in common, or both things when
	// they have nothing.
	draining := func(node nomad.Node) bool { return node.Drain }

	verb := bothWays(nodes, draining, "stop draining", "drain")
	done := bothWays(nodes, draining, "Stopped draining", "Draining")

	return m.ask(
		fmt.Sprintf("Really %s %s? Allocations move elsewhere.", verb, nodeLabel(nodes)),
		eachNode(done, nodeLabel(nodes), nodes, func(ctx context.Context, node nomad.Node) error {
			return client.DrainNode(ctx, node.ID, !node.Drain)
		}),
	)
}

// nodeLabel is what a question about clients says.
func nodeLabel(nodes []nomad.Node) string {
	return many(len(nodes), "the client "+nodes[0].Name, "clients")
}

// eachNode runs the action against every machine on its own, so that one
// that will not answer does not stop the rest.
func eachNode(done, label string, nodes []nomad.Node, do func(context.Context, nomad.Node) error) tea.Cmd {
	return func() tea.Msg {
		out := doneMsg{said: fmt.Sprintf("%s %s.", done, label)}

		went := 0
		failed := []string{}

		for _, node := range nodes {
			ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
			err := do(ctx, node)

			cancel()

			if err != nil {
				failed = append(failed, err.Error())

				continue
			}

			went++
		}

		if len(failed) > 0 {
			out.err = fmt.Errorf("%s %d of %d: %s", strings.ToLower(done), went, len(nodes), failed[0])
		}

		return out
	}
}

// toggleEligibility says whether the client may be given new work.
func (m Model) toggleEligibility() (Model, tea.Cmd) {
	nodes := marked(m, screenNodes, m.nodes)
	if len(nodes) == 0 {
		return m, nil
	}

	client := m.client

	eligible := func(node nomad.Node) bool { return node.Eligibility == "eligible" }

	verb := bothWays(nodes, eligible, "stop giving new work to", "give new work again to")
	done := bothWays(nodes, eligible, "No new work for", "New work again for")

	return m.ask(
		fmt.Sprintf("Really %s %s?", verb, nodeLabel(nodes)),
		eachNode(done, nodeLabel(nodes), nodes, func(ctx context.Context, node nomad.Node) error {
			return client.SetNodeEligible(ctx, node.ID, node.Eligibility != "eligible")
		}),
	)
}

// bothWays is how a question names an action that reads one way for some of
// what it is about and the other way for the rest.
func bothWays[T any](items []T, yes func(T) bool, whenYes, whenNo string) string {
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
		return whenYes + " or " + whenNo
	case some:
		return whenYes
	}

	return whenNo
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
		{Key: "<enter>", Description: "What it runs"},
		{Key: "<ctrl-d>", Description: "Drain"},
		{Key: "<i>", Description: "Eligibility"},
		{Key: "<space>", Description: "Mark"},
	}

	deploymentHints = []hint{
		{Key: "<d>", Description: "Describe"},
		{Key: "<p>", Description: "Promote canaries"},
		{Key: "<f>", Description: "Fail"},
	}
)
