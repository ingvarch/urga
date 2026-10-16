package ui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func manyJobs(n int) []nomad.Job {
	jobs := make([]nomad.Job, 0, n)
	for i := range n {
		id := fmt.Sprintf("job-%02d", i)
		jobs = append(jobs, nomad.Job{ID: id, Name: id, Namespace: "production", Type: "service", Status: "running", Running: 1, Desired: 1})
	}

	return jobs
}

func TestCursor_StaysInTheListWhenItShrinks(t *testing.T) {
	r := require.New(t)

	jobs := manyJobs(20)

	m := newTestModel(&fakeClient{jobs: jobs})
	m, _ = m.update(tea.WindowSizeMsg{Width: 120, Height: 14})
	m, _ = m.update(jobsMsg(jobs))

	// The cursor is on the last row of a list longer than the window.
	m, _ = m.update(key('G'))
	r.Equal(19, m.list.table.cursor)

	// The list shrinks under it, which is what a filter, a namespace switch
	// or a cluster with fewer jobs does.
	m, _ = m.update(key('/'))
	m = typeIn(m, "job-03")

	// The row that is left is on the screen. A cursor past the end scrolls
	// the window past it and the table looks empty.
	out := plain(m.render())
	r.Contains(out, "job-03")
	r.Less(m.list.table.cursor, len(m.list.table.rows))
}

func TestCursor_ShrinkingClusterKeepsTheRowsVisible(t *testing.T) {
	r := require.New(t)

	jobs := manyJobs(20)

	m := newTestModel(&fakeClient{jobs: jobs})
	m, _ = m.update(tea.WindowSizeMsg{Width: 120, Height: 14})
	m, _ = m.update(jobsMsg(jobs))
	m, _ = m.update(key('G'))

	// The cluster answers with a shorter list than the one on the screen.
	m, _ = m.update(jobsMsg(jobs[:2]))

	out := plain(m.render())
	r.Contains(out, "job-00")
	r.Contains(out, "job-01")
}

func TestCursor_StaysOnItsResourceWhenTheListChanges(t *testing.T) {
	r := require.New(t)

	job := func(id string) nomad.Job { return nomad.Job{ID: id, Name: id, Namespace: "production"} }
	selected := func(m Model) string {
		row, _ := m.list.table.selected()

		return row.cells[0]
	}

	m := newTestModel(&fakeClient{})
	m, _ = m.update(jobsMsg([]nomad.Job{job("cron"), job("web")}))
	m, _ = m.update(key('j'))
	r.Equal("web", selected(m))

	// A job that appears above does not move the cursor to another job: a
	// key pressed next acts on the one that was chosen.
	m, _ = m.update(jobsMsg([]nomad.Job{job("api"), job("cron"), job("web")}))
	r.Equal("web", selected(m))

	// When the job is gone, the cursor is on no row: the one that now has
	// its number is not the one that was chosen.
	m, _ = m.update(jobsMsg([]nomad.Job{job("api"), job("batch"), job("cron")}))
	_, onRow := m.list.table.selected()
	r.False(onRow)
}

// firstCell is the first cell of the row under the cursor.
func firstCell(m Model) string {
	row, ok := m.list.table.selected()
	if !ok {
		return ""
	}

	return row.cells[0]
}

func TestCursor_ATokenKeepsItsSecretWhenANewOneComesAbove(t *testing.T) {
	r := require.New(t)

	const (
		first  = "aaaa0000-0000-0000-0000-000000000000"
		second = "bbbb0000-0000-0000-0000-000000000000"
		newer  = "0000cccc-0000-0000-0000-000000000000"
	)

	token := func(id, name string) nomad.ACLObject {
		return nomad.ACLObject{Kind: nomad.ACLToken, ID: id, Name: name, Type: "client"}
	}

	client := &fakeClient{secret: "s3cr3t-0000"}
	m := onACL(t, client, "tokens")
	m, _ = m.update(aclMsg{kind: nomad.ACLToken, objects: []nomad.ACLObject{token(first, "deployer"), token(second, "auditor")}})

	// The token that was under the cursor is gone, so a move puts it on the
	// first row, and another on the second.
	m, _ = m.update(key('j'))
	m, _ = m.update(key('j'))

	// A token that sorts above the one under the cursor must not move the
	// secret that a key copies to another token.
	m, _ = m.update(aclMsg{kind: nomad.ACLToken, objects: []nomad.ACLObject{
		token(newer, "newest"), token(first, "deployer"), token(second, "auditor"),
	}})

	m, cmd := m.update(key('c'))
	m = playOut(m, cmd)

	r.Equal(nomad.ACLToken+"/"+second, client.aclOf)
	r.Contains(plain(statusLine(m)), "Copied the secret of the token auditor.")
}

// pageCase is a page with a row chosen, the answer of the next poll with a
// row above it, and what the chosen row starts with.
type pageCase struct {
	open func(t *testing.T) Model
	poll tea.Msg
	want string
}

func variableRowCase() pageCase {
	row := func(path string) nomad.Variable { return nomad.Variable{Path: path, Namespace: "default"} }

	return pageCase{
		open: func(*testing.T) Model {
			m, _ := newTestModel(&fakeClient{}).show(variablesView)
			m, _ = m.update(variablesMsg([]nomad.Variable{row("apps/b"), row("apps/c")}))

			return m
		},
		poll: variablesMsg([]nomad.Variable{row("apps/a"), row("apps/b"), row("apps/c")}),
		want: "apps/c",
	}
}

func namespaceRowCase() pageCase {
	return pageCase{
		open: func(*testing.T) Model {
			m := typeCommand(newTestModel(&fakeClient{}), "ns")
			m, _ = m.update(namespacesMsg([]nomad.Namespace{{Name: "production"}, {Name: "staging"}}))

			return m
		},
		poll: namespacesMsg([]nomad.Namespace{{Name: "alpha"}, {Name: "production"}, {Name: "staging"}}),
		want: "staging",
	}
}

func versionRowCase() pageCase {
	return pageCase{
		open: func(t *testing.T) Model {
			m, _ := onVersions(t)

			return m
		},
		poll: versionsMsg{jobID: "web", versions: append([]nomad.JobVersion{{Version: 4}}, threeVersions()...)},
		want: "2",
	}
}

func deploymentRowCase() pageCase {
	return pageCase{
		open: func(t *testing.T) Model { return onDeployments(t, &fakeClient{}) },
		poll: deploymentsMsg([]nomad.Deployment{
			{ID: "0000aaaa-0000-0000-0000-000000000000", JobID: "new", Namespace: "production"},
			waitingCanary().Deployment, pausedCron(),
		}),
		want: "7e8f9a0b",
	}
}

func instanceRowCase() pageCase {
	added := []nomad.ServiceInstance{{ID: "reg-0", Service: "served", Namespace: "default"}}

	return pageCase{
		open: func(t *testing.T) Model { return onServed(t, &fakeClient{}) },
		poll: instancesMsg(append(added, servedInstances()...)),
		want: "10.0.0.7:24011",
	}
}

func groupRowCase() pageCase {
	return pageCase{
		open: func(*testing.T) Model {
			m := newTestModel(&fakeClient{jobs: twoJobs()})
			m, _ = m.update(jobsMsg(twoJobs()))
			m, _ = m.update(key('t'))
			m, _ = m.update(taskGroupsMsg(twoGroups()))

			return m
		},
		poll: taskGroupsMsg(append([]nomad.TaskGroup{{Name: "alpha", JobID: "web"}}, twoGroups()...)),
		want: "backend",
	}
}

func taskRowCase() pageCase {
	alloc := twoAllocs()[0]
	alloc.Tasks = append([]nomad.Task{{Name: "alpha", State: "running"}}, alloc.Tasks...)

	return pageCase{
		open: func(t *testing.T) Model {
			m, _ := onAllocations(t)
			m, _ = m.update(enter())

			return m
		},
		poll: allocMsg(alloc),
		want: "sidecar",
	}
}

func variableValueCase() pageCase {
	detail := func(items map[string]string) nomad.VariableDetail {
		return nomad.VariableDetail{Variable: nomad.Variable{Path: "apps/web", Namespace: "default"}, Items: items}
	}

	return pageCase{
		open: func(t *testing.T) Model {
			return onVariable(t, &fakeClient{}, detail(map[string]string{"B": "1", "C": "2"}))
		},
		poll: variableMsg(detail(map[string]string{"A": "0", "B": "1", "C": "2"})),
		want: "C",
	}
}

func TestCursor_StaysOnItsResourceOnEveryPageThatActs(t *testing.T) {
	cases := map[string]pageCase{
		"variables":         variableRowCase(),
		"namespaces":        namespaceRowCase(),
		"versions":          versionRowCase(),
		"deployments":       deploymentRowCase(),
		"service instances": instanceRowCase(),
		"task groups":       groupRowCase(),
		"tasks":             taskRowCase(),
		"variable values":   variableValueCase(),
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			m := c.open(t)
			m, _ = m.update(key('j'))
			r.Contains(firstCell(m), c.want)

			m, _ = m.update(c.poll)

			// The row that was chosen is still under the cursor.
			r.Contains(firstCell(m), c.want)
		})
	}
}

func TestCursor_IsOnNoRowWhenItsResourceLeavesTheView(t *testing.T) {
	r := require.New(t)

	m, client := onAllocations(t)
	m, _ = m.update(allocsMsg(troubledAllocs()))
	m, _ = m.update(key('!'))
	m, _ = m.update(key('j'))
	m, _ = m.update(key('j'))
	r.Equal("bbbb2222", shortID(m.list.table.ids[m.list.table.cursor]))

	// The next answer finds it fine, so the filter drops it from the view.
	mended := troubledAllocs()
	mended[1].Tasks = nil
	m, _ = m.update(allocsMsg(mended))

	r.Len(m.list.table.rows, 2)
	r.Equal(-1, m.list.table.cursor)

	// A key that acts on a row has none to act on, and does not slide to
	// the one that has its number.
	m, _ = m.update(ctrlKey('k'))
	r.NotEqual(overlayConfirm, m.overlay)
	r.Zero(client.stoppedAllocs)

	// The next move puts the cursor on a row again.
	m, _ = m.update(key('j'))
	r.Equal(0, m.list.table.cursor)
}

func TestTable_KeepsItsWindowWhileTheCursorIsOnNoRow(t *testing.T) {
	r := require.New(t)

	rows := make([]tableRow, 20)
	for i := range rows {
		rows[i] = tableRow{cells: []string{fmt.Sprint(i)}}
	}

	table := newTableModel([]string{"ID"})
	table.setSize(20, 5)
	table.show(rows, table.sort)
	table.move(12)
	r.Equal(8, table.top)

	table.leave()
	table.show(rows, table.sort)
	table.show(rows, table.sort)

	r.Equal(8, table.top)
}

func TestCursor_ACursorOnNoRowDrawsTheQuestionInTheMiddle(t *testing.T) {
	r := require.New(t)

	m, _ := onAllocations(t)
	m, _ = m.update(allocsMsg(troubledAllocs()))
	m, _ = m.update(key('!'))
	m, _ = m.update(space())
	m, _ = m.update(key('j'))
	m, _ = m.update(key('j'))
	r.Equal("bbbb2222", shortID(m.list.table.ids[m.list.table.cursor]))
	m, _ = m.update(space())

	mended := troubledAllocs()
	mended[1].Tasks = nil
	m, _ = m.update(allocsMsg(mended))
	r.Equal(noRow, m.list.table.cursor)

	m, _ = m.update(ctrlKey('k'))
	r.Equal(overlayConfirm, m.overlay)
	r.Equal(-1, m.cursorLine())

	lines := strings.Split(plain(m.render()), "\n")
	at := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, "Really stop") })
	r.Positive(at)
	r.InDelta(len(lines)/2, at, 2)
}
