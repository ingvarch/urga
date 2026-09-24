package ui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// opened is what a key opens from the row under the cursor. A list is up the
// moment the key is pressed; a description, a job file or the diff of a
// version arrives as an answer, so its request is run.
func opened(m Model, press tea.KeyPressMsg) Model {
	next, cmd := m.handleKey(press)
	if len(next.history) > len(m.history) || cmd == nil {
		return next
	}

	if answer, ok := cmd().(describeMsg); ok {
		next, _ = next.update(answer)
	}

	return next
}

// openAndBack opens what a key opens and escapes back to the list.
func openAndBack(t *testing.T, m Model, press tea.KeyPressMsg) Model {
	t.Helper()

	next := opened(m, press)
	require.Greater(t, len(next.history), len(m.history), "%s opens no screen", press.String())

	back, _ := next.update(escape())

	return back
}

// cursorName is the first cell of the row under the cursor.
func cursorName(m Model) string {
	row, ok := m.table.selected()
	if !ok {
		return ""
	}

	return row.cells[0]
}

func TestBack_ComesBackToTheRowADescriptionWasAskedFrom(t *testing.T) {
	client := &fakeClient{
		jobs:        twoJobs(),
		allocs:      twoAllocs(),
		deployments: []nomad.Deployment{{ID: "dep-1", JobID: "web"}, {ID: "dep-2", JobID: "cron"}},
		services:    []nomad.Service{{Name: "api"}, {Name: "web"}},
		describe:    "{}",
		spec:        nomad.JobSource{Source: `job "cron" {}`},
	}

	tests := []struct {
		name  string
		open  string
		rows  tea.Msg
		press tea.KeyPressMsg
	}{
		{name: "describe a job", open: "jobs", rows: jobsMsg(client.jobs), press: key('d')},
		{name: "the file of a job", open: "jobs", rows: jobsMsg(client.jobs), press: key('h')},
		{name: "describe an allocation", open: "allocations", rows: allocsMsg(client.allocs), press: key('d')},
		{name: "describe a deployment", open: "deployments", rows: deploymentsMsg(client.deployments), press: key('d')},
		{name: "describe a service", open: "services", rows: servicesMsg(client.services), press: key('d')},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := require.New(t)

			m, _ := runLine(newTestModel(client), test.open)
			m, _ = m.update(test.rows)
			m, _ = m.update(down())

			left := cursorName(m)

			m = openAndBack(t, m, test.press)

			// The cursor is where it was left, not on the first row.
			r.Equal(1, m.table.cursor)
			r.Equal(left, cursorName(m))
		})
	}
}

func TestBack_ComesBackToTheRowOfEveryKeyThatOpensAScreen(t *testing.T) {
	// The keys that edit write their file before anything else happens.
	t.Setenv("TMPDIR", t.TempDir())

	for name, m := range everyScreen(t) {
		if len(m.table.rows) < 2 || m.readsAsText() {
			continue
		}

		m, _ = m.update(down())
		left := cursorName(m)

		for _, h := range m.hints() {
			next := opened(m, keyOf(h.Key))
			if len(next.history) <= len(m.history) {
				continue
			}

			back, _ := next.update(escape())

			require.Equal(t, m.screen.kind, back.screen.kind, "%s %s", name, h.Key)
			require.Equal(t, left, cursorName(back), "the %s screen after %s and escape", name, h.Key)
		}
	}
}

func TestBack_ComesBackToTheRowOfAClient(t *testing.T) {
	r := require.New(t)

	nodes := []nomad.Node{
		{ID: "node-1", Name: "nomad-01", Status: "ready"},
		{ID: "node-2", Name: "nomad-02", Status: "ready"},
	}

	m, _ := runLine(newTestModel(&fakeClient{nodes: nodes}), "clients")
	m, _ = m.update(nodesMsg(nodes))
	m, _ = m.update(down())

	m = openAndBack(t, m, enter())

	r.Equal("node-2", cursorName(m))
}

func TestBack_KeepsTheFilterAndTheOrderOfTheList(t *testing.T) {
	r := require.New(t)

	jobs := []nomad.Job{
		{ID: "web-b", Name: "web-b", Namespace: "production"},
		{ID: "api", Name: "api", Namespace: "production"},
		{ID: "web-a", Name: "web-a", Namespace: "production"},
	}

	m := newTestModel(&fakeClient{jobs: jobs, describe: "{}"})
	m, _ = m.update(jobsMsg(jobs))

	// Only the web jobs, in the order of their name.
	m, _ = m.update(key('/'))
	m = typeIn(m, "web")
	m, _ = m.update(enter())
	m, _ = m.update(key('I'))
	m, _ = m.update(down())
	r.Equal("web-b", cursorName(m))

	m = openAndBack(t, m, key('d'))

	// The list reads as it did: a row number means nothing in a list that
	// lost its filter or its order.
	r.Equal("web", m.filter)
	r.Equal([]string{"web-a", "web-b"}, []string{m.table.rows[0].cells[0], m.table.rows[1].cells[0]})
	r.Equal("web-b", cursorName(m))
}

func TestBack_KeepsTheWindowWhereItWas(t *testing.T) {
	r := require.New(t)

	jobs := manyJobs(40)

	m := newTestModel(&fakeClient{jobs: jobs, describe: "{}"})
	m, _ = m.update(tea.WindowSizeMsg{Width: 120, Height: 20})
	m, _ = m.update(jobsMsg(jobs))

	for range 25 {
		m, _ = m.update(down())
	}

	top := m.table.top
	r.Positive(top, "the window has moved down the list")

	m = openAndBack(t, m, key('d'))

	r.Equal(fmt.Sprintf("job-%02d", 25), cursorName(m))
	r.Equal(top, m.table.top)
}

func TestBack_ComesBackToTheRowOfEveryScreenOfAClient(t *testing.T) {
	r := require.New(t)

	allocs := []nomad.Alloc{
		{ID: "a1111111-0000-0000-0000-000000000000", Namespace: "production", JobID: "web", Status: "running"},
		{ID: "b2222222-0000-0000-0000-000000000000", Namespace: "production", JobID: "api", Status: "running"},
	}

	client := &fakeClient{nodes: busyClient(), nodeAllocs: allocs, nodeDetail: clientDetail(), nodeMeta: clientMeta()}

	m, _ := runLine(newTestModel(client), "clients")
	m, _ = m.update(nodesMsg(client.nodes))
	m, cmd := m.update(enter())
	m = drain(m, cmd)
	r.True(m.screen.isClient())

	m, _ = m.update(down())
	left := cursorName(m)

	// What the machine happened to do, what it can run, what it lends out,
	// what it is built from and what it carries.
	for _, press := range []tea.KeyPressMsg{key('e'), ctrlKey('d'), ctrlKey('h'), key('a'), key('m')} {
		back := openAndBack(t, m, press)

		r.True(back.screen.isClient(), press.String())
		r.Equal(left, cursorName(back), press.String())
	}
}

func TestBack_ComesBackToTheVersionADiffWasAskedFrom(t *testing.T) {
	r := require.New(t)

	m, _ := onVersions(t)
	m, _ = m.update(down())
	left := cursorName(m)

	m = openAndBack(t, m, enter())

	r.Equal(screenJobVersions, m.screen.kind)
	r.Equal(left, cursorName(m))
}
