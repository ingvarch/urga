package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestIsFull(t *testing.T) {
	for _, tc := range []struct {
		node nomad.Node
		full bool
	}{
		{nomad.Node{MaxAllocs: 3, Allocs: 3}, true},
		{nomad.Node{MaxAllocs: 3, Allocs: 4}, true},
		{nomad.Node{MaxAllocs: 3, Allocs: 2}, false},
		{nomad.Node{Allocs: 2}, false},
	} {
		require.Equal(t, tc.full, isFull(tc.node), "%+v", tc.node)
	}
}

func TestNodeRows_AFullClientTakesNoNewWorkAndIsPaintedLikeOneThatDoesNot(t *testing.T) {
	r := require.New(t)

	ready := nomad.Node{Status: "ready", Eligibility: "eligible"}

	full, below, none := ready, ready, ready
	full.MaxAllocs, full.Allocs = 3, 3
	below.MaxAllocs, below.Allocs = 3, 2

	rows := nodeRows([]nomad.Node{full, below, none}, nil)

	r.Equal(colorAttention, rows[0].color)
	r.Nil(rows[1].color)
	r.Nil(rows[2].color)
}

func TestNodes_TheListAsksForTheLimitsOfWhatItReads(t *testing.T) {
	r := require.New(t)

	full := nomad.Node{ID: "node-1", Name: "server-01", Status: "ready", Eligibility: "eligible", MaxAllocs: 2, Allocs: 2}
	client := &fakeClient{nodes: []nomad.Node{full}}

	m, cmd := runLine(newTestModel(client), "nodes")
	m = drain(m, cmd)

	r.Equal([]string{""}, client.lists["limits"])
	r.Equal(colorAttention, m.list.table.rows[0].color)
}

func TestNodes_ThePoolListAsksForLimitsToo(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{poolNodes: []nomad.Node{{ID: "node-1", Status: "ready", Eligibility: "eligible"}}}

	cmd := nodesPage{pool: "default"}.fetch(newTestModel(client).env())
	drain(newTestModel(client), cmd)

	r.Equal([]string{""}, client.lists["limits"])
}

// limitedCluster holds one client with an allocation limit, and allocs on it.
func limitedCluster(limit int, allocs []nomad.Alloc) *fakeClient {
	nodes := busyClient()
	nodes[0].MaxAllocs = limit

	return &fakeClient{nodes: nodes, nodeAllocs: allocs, use: map[string]nomad.ResourceUse{"node-1": {}}}
}

func limitedClient(t *testing.T, limit int, allocs []nomad.Alloc) Model {
	t.Helper()

	m, _ := nodeModelOf(limitedCluster(limit, allocs))
	m, cmd := m.update(enter())

	return drain(m, cmd)
}

func TestClient_DoesNotCountAllocationsItHasNotRead(t *testing.T) {
	r := require.New(t)

	// The list carries the limit, and the allocations are not read yet.
	m, client := nodeModelOf(limitedCluster(3, clientAllocs()))
	m, _ = m.update(enter())
	r.IsType(clientPage{}, m.screen.page)

	r.NotContains(bodyOf(m), "Allocations 0 of 3")
	r.Contains(bodyOf(m), "Allocations n/a of 3")

	// Nor when the cluster does not answer for them.
	client.allocsErr = errTest
	m = drain(m, m.fetch())

	r.Contains(m.flash.text, errTest.Error())
	r.NotContains(bodyOf(m), "Allocations 0 of 3")
	r.Contains(bodyOf(m), "Allocations n/a of 3")
}

func TestNodes_AListThatCannotBeReadShowsTheErrorAndAsksForNoLimits(t *testing.T) {
	r := require.New(t)

	m, client := nodeModel(t, readyNode())
	client.nodesErr = errTest

	m = drain(m, m.fetch())

	r.Contains(m.flash.text, errTest.Error())
	r.Empty(client.lists["limits"])
}

func TestClient_ShowsHowManyAllocationsItHoldsOfItsLimit(t *testing.T) {
	r := require.New(t)

	running, ended := clientAllocs()[0], clientAllocs()[0]
	ended.Status = "complete"

	stopped := running
	stopped.DesiredStatus = "stop"

	m := limitedClient(t, 3, []nomad.Alloc{running, running, ended, stopped})

	r.Contains(bodyOf(m), "Allocations 2 of 3")
}

func TestClient_WithoutALimitShowsNoAllocationCount(t *testing.T) {
	m, _ := onClient(t)

	require.NotContains(t, bodyOf(m), "Allocations ")
}

func TestClient_ThePanelOfALimitedClientCountsItsExtraRow(t *testing.T) {
	r := require.New(t)

	m := limitedClient(t, 3, clientAllocs())
	m, _ = m.update(reading(29))

	// The charts get what the extra row leaves, so the tall ones need a row
	// more than without a limit, and a box too small for the two rows and
	// one more draws no panel.
	for _, tc := range []struct {
		width, box, panel int
		charts            bool
	}{
		{width: 120, box: 40, panel: 19, charts: true},
		{width: 120, box: 25, panel: 19, charts: true},
		{width: 120, box: 24, panel: 13, charts: true},
		{width: 120, box: 19, panel: 13, charts: true},
		{width: 120, box: 18, panel: 3},
		{width: 120, box: 9, panel: 3},
		{width: 120, box: 8, panel: 0},
		{width: 120, box: 3, panel: 0},
		{width: 20, box: 40, panel: 3},
	} {
		sized, _ := m.update(tea.WindowSizeMsg{Width: tc.width, Height: tc.box + screenPadTop + headerHeight + statusHeight})
		_, content := sized.body(sized.innerWidth())
		body := plain(content)

		r.Equal(tc.panel, sized.panelHeight(), "%dx%d", tc.width, tc.box)
		r.Equal(tc.panel, drawnPanel(sized), "%dx%d", tc.width, tc.box)
		r.Equal(max(tc.box-3-tc.panel, 1), sized.list.table.height, "%dx%d", tc.width, tc.box)

		// A screen this narrow cuts the row.
		if tc.width == 120 {
			r.Equal(tc.panel > 0, strings.Contains(body, "Allocations 1 of 3"), "%dx%d", tc.width, tc.box)
		}

		r.Equal(tc.charts, strings.Contains(body, "50%"), "%dx%d", tc.width, tc.box)
	}
}

func TestOverview_DoesNotAskForLimits(t *testing.T) {
	client := &fakeClient{}

	overviewAnswered(client)

	require.Empty(t, client.lists["limits"])
}
