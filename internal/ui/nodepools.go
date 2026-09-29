package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// nodePoolsPage is the pools the nodes of the cluster are grouped in.
type nodePoolsPage struct {
	ofTheSession

	pools []nomad.NodePool
}

var nodePoolTitles = []string{"Name", "Scheduler", "Description"}

// A pool belongs to the cluster, not to a namespace: the title names none.
func (nodePoolsPage) title(_ env, count int) string {
	return sprintf("Node Pools [%d]", count)
}

func (nodePoolsPage) titles() []string { return nodePoolTitles }

func (nodePoolsPage) topics() []string { return []string{nomad.TopicNodePool} }

func (nodePoolsPage) fetch(e env) tea.Cmd {
	return fetchList(e.client.NodePools, func(items []nomad.NodePool) tea.Msg { return nodePoolsMsg(items) })
}

func (p nodePoolsPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	pools, ok := msg.(nodePoolsMsg)
	if !ok {
		return p, outcome{}, false
	}

	p.pools = pools

	return p, outcome{}, true
}

func (p nodePoolsPage) rows(env) []tableRow {
	rows := make([]tableRow, 0, len(p.pools))

	for _, pool := range p.pools {
		rows = append(rows, tableRow{cells: []string{pool.Name, pool.Scheduler, pool.Description}})
	}

	return rows
}

var nodePoolsKeys = []pageKey[nodePoolsPage]{
	{press: "enter", label: "Jobs", do: openInPool(func(pool string) page { return jobsPage{pool: pool} })},
	{press: "c", label: "Clients", do: openInPool(func(pool string) page { return nodesPage{pool: pool} })},
}

func (p nodePoolsPage) keys(e env) []keyHint { return hintsOf(p, e, nodePoolsKeys) }

func (p nodePoolsPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, nodePoolsKeys, k)
}

// openInPool opens a list of what is in the pool under the cursor.
func openInPool(open func(pool string) page) func(nodePoolsPage, env) (nodePoolsPage, outcome) {
	return func(p nodePoolsPage, e env) (nodePoolsPage, outcome) {
		pool, ok := pickedFrom(e, p.pools)
		if !ok {
			return p, outcome{}
		}

		return p, then(openMsg{open(pool.Name)})
	}
}
