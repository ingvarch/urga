package ui

import tea "charm.land/bubbletea/v2"

// The lists a region, a datacenter or a cluster is picked from. What they
// list is the session's: they show it, and enter asks the session to move.
type (
	regionsPage     struct{}
	datacentersPage struct{}
	clustersPage    struct{}
)

// choiceTitles are the columns of a list a region or a datacenter is
// picked from.
var choiceTitles = []string{"Name", ""}

// choiceRows are the names to pick from, the one in use marked.
func choiceRows(names []string, inUse string) []tableRow {
	rows := make([]tableRow, 0, len(names))

	for _, name := range names {
		row := tableRow{cells: []string{name, ""}}

		if name == inUse {
			row.cells[1] = "in use"
			row.color = colorTitle
		}

		rows = append(rows, row)
	}

	return rows
}

// switchTo moves the session to the choice under the cursor. The one in use
// has nothing to switch, and the list goes back to where it was opened from.
func switchTo(e env, choices []string, inUse string, move func(string) tea.Msg) outcome {
	choice, ok := pickedFrom(e, choices)
	if !ok {
		return outcome{}
	}

	if choice == inUse {
		return then(backMsg{})
	}

	return then(move(choice))
}

func (regionsPage) title(_ env, count int) string { return sprintf("Regions [%d]", count) }
func (regionsPage) titles() []string              { return choiceTitles }
func (regionsPage) topics() []string              { return nil }
func (regionsPage) fetch(e env) tea.Cmd           { return fetchRegions(e.client) }
func (p regionsPage) take(tea.Msg) (page, bool)   { return p, false }
func (regionsPage) rows(e env) []tableRow         { return choiceRows(e.regions, e.region) }

var regionKeys = []pageKey[regionsPage]{
	{press: "enter", label: "Switch", do: func(p regionsPage, e env) (regionsPage, outcome) {
		return p, switchTo(e, e.regions, e.region, func(region string) tea.Msg { return switchRegionMsg(region) })
	}},
}

func (p regionsPage) keys(e env) []keyHint { return hintsOf(p, e, regionKeys) }

func (p regionsPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, regionKeys, k)
}

func (datacentersPage) title(_ env, count int) string { return sprintf("Datacenters [%d]", count) }
func (datacentersPage) titles() []string              { return choiceTitles }
func (datacentersPage) topics() []string              { return nil }
func (datacentersPage) fetch(e env) tea.Cmd           { return fetchDatacenters(e.client) }
func (p datacentersPage) take(tea.Msg) (page, bool)   { return p, false }
func (datacentersPage) rows(e env) []tableRow {
	return choiceRows(e.datacenters, orEvery(e.datacenter))
}

// datacenterKeys narrow the screen the list was opened from to the
// datacenter under the cursor, or to every one of them.
var datacenterKeys = []pageKey[datacentersPage]{
	{press: "enter", label: "Switch", do: func(p datacentersPage, e env) (datacentersPage, outcome) {
		return p, switchTo(e, e.datacenters, orEvery(e.datacenter), func(choice string) tea.Msg {
			if choice == everyDatacenter {
				return narrowMsg("")
			}

			return narrowMsg(choice)
		})
	}},
}

func (p datacentersPage) keys(e env) []keyHint { return hintsOf(p, e, datacenterKeys) }

func (p datacentersPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, datacenterKeys, k)
}

func (clustersPage) title(_ env, count int) string { return sprintf("Clusters [%d]", count) }
func (clustersPage) titles() []string              { return choiceTitles }
func (clustersPage) topics() []string              { return nil }
func (clustersPage) fetch(env) tea.Cmd             { return nil }
func (p clustersPage) take(tea.Msg) (page, bool)   { return p, false }
func (clustersPage) rows(e env) []tableRow         { return choiceRows(e.clusters, e.cluster) }

var clusterKeys = []pageKey[clustersPage]{
	{press: "enter", label: "Switch", do: func(p clustersPage, e env) (clustersPage, outcome) {
		return p, switchTo(e, e.clusters, e.cluster, func(name string) tea.Msg { return switchClusterMsg(name) })
	}},
}

func (p clustersPage) keys(e env) []keyHint { return hintsOf(p, e, clusterKeys) }

func (p clustersPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, clusterKeys, k)
}
