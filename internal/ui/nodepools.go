package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// nodePoolsPage is the pools the nodes of the cluster are grouped in.
type nodePoolsPage struct {
	noKeys

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
