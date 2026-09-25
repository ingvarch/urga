package ui

import (
	"context"
	"fmt"
	"image/color"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// nodesPage is the clients of the region in use, the machines that run the
// work. Nomad calls them clients in its own interface.
type nodesPage struct {
	ofTheSession

	nodes []nomad.Node
}

func (nodesPage) title(_ env, count int) string { return sprintf("Clients [%d]", count) }
func (nodesPage) titles() []string              { return nodeTitles }
func (nodesPage) topics() []string              { return []string{nomad.TopicNode} }

func (nodesPage) fetch(e env) tea.Cmd {
	return fetchList(e.client.Nodes, func(items []nomad.Node) tea.Msg { return nodesMsg(items) })
}

func (p nodesPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	nodes, ok := msg.(nodesMsg)
	if !ok {
		return p, outcome{}, false
	}

	p.nodes = nodes

	return p, outcome{}, true
}

// visible are the clients of the datacenter the session is narrowed to. They
// are narrowed as they are drawn: until the cluster answers, and when it
// does not, nothing of another datacenter stands under the new name. The
// rows, the marks and the keys that find a row by its place read the same
// list, so a key finds the client on the screen.
func (p nodesPage) visible(e env) []nomad.Node { return nodesIn(e.datacenter, p.nodes) }

func (p nodesPage) rows(e env) []tableRow { return nodeRows(p.visible(e), e.usage) }

func (p nodesPage) ids(e env) []string { return names(p.visible(e), nodeMark) }

// readings are the clients on the screen: what each of them is busy with.
func (p nodesPage) readings(e env) []rowRef {
	nodes := p.visible(e)

	refs := make([]rowRef, 0, len(e.index))
	for _, at := range e.index {
		if at < len(nodes) {
			refs = append(refs, rowRef{id: nodes[at].ID})
		}
	}

	return refs
}

func (nodesPage) reading(ctx context.Context, client Client, ref rowRef) (nomad.ResourceUse, error) {
	return client.NodeUsage(ctx, ref.id)
}

var nodesKeys = []pageKey[nodesPage]{
	{press: "enter", label: "Allocations", do: openClient},
	{press: "ctrl+d", label: "Drain", do: drainNode, writes: true},
	{press: "i", label: "Toggle Eligibility", do: toggleEligibility, writes: true},
	markKey[nodesPage](),
	markAllKey[nodesPage](),
}

func (p nodesPage) keys(e env) []keyHint { return hintsOf(p, e, nodesKeys) }

func (p nodesPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, nodesKeys, k)
}

// openClient drills into the client under the cursor: what it runs, under
// what the machine itself is doing.
func openClient(p nodesPage, e env) (nodesPage, outcome) {
	node, ok := pickedFrom(e, p.visible(e))
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg{clientOf(node)})
}

// drainNode starts or stops moving the work off the client under the cursor.
func drainNode(p nodesPage, e env) (nodesPage, outcome) {
	nodes := markedFrom(e, p.visible(e), nodeMark)
	if len(nodes) == 0 {
		return p, outcome{}
	}

	client := e.client

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

	return p, then(askMsg{
		question: fmt.Sprintf("Really %s %s?%s", verb, nodeLabel(nodes), aside),
		apply: each(done, nodeLabel(nodes), nodes, nodeMark, func(ctx context.Context, node nomad.Node) error {
			return client.DrainNode(ctx, node.ID, !node.Drain)
		}),
	})
}

// nodeLabel is what a question about clients says.
func nodeLabel(nodes []nomad.Node) string {
	return many(len(nodes), "the client "+nodes[0].Name, "clients")
}

// toggleEligibility says whether the client may be given new work.
func toggleEligibility(p nodesPage, e env) (nodesPage, outcome) {
	nodes := markedFrom(e, p.visible(e), nodeMark)
	if len(nodes) == 0 {
		return p, outcome{}
	}

	client := e.client

	eligible := func(node nomad.Node) bool { return node.Eligibility == "eligible" }

	verb := bothWays(nodes, eligible, "take new work from", "give new work back to",
		"change the work of")
	done := bothWays(nodes, eligible, "Took new work from", "Gave new work back to",
		"Changed the work of")

	return p, then(askMsg{
		question: fmt.Sprintf("Really %s %s?", verb, nodeLabel(nodes)),
		apply: each(done, nodeLabel(nodes), nodes, nodeMark, func(ctx context.Context, node nomad.Node) error {
			return client.SetNodeEligible(ctx, node.ID, node.Eligibility != "eligible")
		}),
	})
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

var nodeTitles = []string{"ID", "Name", "Datacenter", "Pool", "Version", "Status", "Eligibility", "Drain", "CPU", "MEM", "Address"}

func nodeRows(nodes []nomad.Node, usage map[string]nomad.ResourceUse) []tableRow {
	rows := make([]tableRow, 0, len(nodes))

	for _, n := range nodes {
		use, known := usage[n.ID]

		rows = append(rows, tableRow{
			cells: []string{
				shortID(n.ID),
				n.Name,
				n.Datacenter,
				n.NodePool,
				n.Version,
				n.Status,
				n.Eligibility,
				fmt.Sprintf("%t", n.Drain),
				percentCell(use.CPUPercent, known),
				percentCell(use.MemoryPercent, known),
				n.Address,
			},
			color: nodeColor(n),
		})
	}

	return rows
}

// nodeColor marks a node that takes no work: down, draining or held back.
func nodeColor(n nomad.Node) color.Color {
	switch {
	case n.Status != "ready":
		return colorDead
	case n.Drain, n.Eligibility != "eligible":
		return colorAttention
	}

	return nil
}
