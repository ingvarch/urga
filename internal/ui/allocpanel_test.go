package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// replacedCanary is the first allocation of twoAllocs as a full reading
// says it: a canary of version 7 that replaced one that failed, and was in
// turn replaced.
func replacedCanary() nomad.Alloc {
	alloc := restarted()
	alloc.NodeID = "a3e23694-4528-6395-3a51-1ffcbf3c2ba4"
	alloc.DesiredStatus = "run"
	alloc.JobVersion = 7
	alloc.Health = "checking"
	alloc.Canary = true
	alloc.Ports = []nomad.Port{
		{Label: "http", Address: "10.0.0.5:21659", To: 8080},
		{Label: "admin", Address: "10.0.0.5:22689"},
	}
	alloc.Reschedules = 2
	alloc.Previous = "4f2a1c9e-0000-0000-0000-000000000000"
	alloc.Next = "9a1b2c3d-0000-0000-0000-000000000000"
	alloc.FollowUp = "8b1c2d3e-0000-0000-0000-000000000000"

	return alloc
}

func TestAllocPanel(t *testing.T) {
	r := require.New(t)

	lines := allocPanel(replacedCanary(), nil, 120, 100)

	for i := range lines {
		lines[i] = plain(lines[i])
	}

	// What it is, where it listens, and what came before and after it; a
	// line of air before the tasks.
	r.Equal([]string{
		" Status running   Desired run   Client node-01   Version 7   Deployment checking, canary",
		" Ports http 10.0.0.5:21659->8080, admin 10.0.0.5:22689",
		" Reschedules 2   Previous 4f2a1c9e   Next 9a1b2c3d   Follow-up 8b1c2d3e",
		"",
	}, lines)
}

func TestAllocPanel_SaysOnlyWhatThereIs(t *testing.T) {
	r := require.New(t)

	alloc := twoAllocs()[0]
	alloc.DesiredStatus = "run"

	text := strings.Join(allocPanel(alloc, nil, 120, 100), "\n")

	// No deployment, no ports, nothing before or after it: no line for any
	// of it.
	r.NotContains(text, "Deployment")
	r.NotContains(text, "Ports")
	r.NotContains(text, "Previous")
	r.Contains(plain(text), "Version 0")
	r.Len(allocPanel(alloc, nil, 120, 100), 2)
}

// openCanary is the tasks of the first allocation, once the full reading of
// it came back.
func openCanary(t *testing.T, client *fakeClient) Model {
	t.Helper()

	client.alloc = replacedCanary()

	m := openTasks(t, client)

	return drain(m, m.fetch())
}

func TestTasks_ThePanelIsOverThem(t *testing.T) {
	r := require.New(t)

	m := openCanary(t, &fakeClient{jobs: twoJobs(), allocs: twoAllocs()})

	rows := lines(plain(m.render()))
	panel, header := -1, -1

	for i, row := range rows {
		if strings.Contains(row, "Ports http 10.0.0.5:21659->8080") {
			panel = i
		}

		if strings.Contains(row, "Name") && strings.Contains(row, "Restarts") {
			header = i
		}
	}

	r.Positive(panel)
	r.Greater(header, panel)
	r.Contains(taskLine(t, m, "server"), "running")
}

func TestTasks_OpenTheClient(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openCanary(t, client)
	m, cmd := m.update(key('c'))
	m = drain(m, cmd)

	// The machine it runs on, with what it runs and what it is doing.
	r.Equal(screenNode, m.screen.kind)
	r.Contains(plain(m.render()), "Client node-01")
	r.Equal("a3e23694-4528-6395-3a51-1ffcbf3c2ba4", client.askedNodeID)
}

func TestTasks_OpenTheAllocationItReplaced(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openCanary(t, client)
	r.True(offers(m, "p"))

	// The one it replaced is not in the list the tasks were opened from:
	// its tasks are what the cluster says of it.
	replaced := nomad.Alloc{
		ID: "4f2a1c9e-0000-0000-0000-000000000000", Namespace: "production", JobID: "web",
		Tasks: []nomad.Task{{Name: "old-server", State: "dead", Failed: true}},
	}
	client.alloc = replaced

	m, cmd := m.update(key('p'))
	m = drain(m, cmd)

	r.Equal(screenTasks, m.screen.kind)
	r.Contains(plain(m.render()), "Tasks (Allocation: 4f2a1c9e)")
	r.Equal("production", client.askedNamespace)
	r.Contains(taskLine(t, m, "old-server"), "dead")

	// Escape comes back to the one it was opened from.
	m, _ = m.update(escape())
	r.Contains(plain(m.render()), "Tasks (Allocation: af1f37df)")
}

func TestTasks_OpenTheAllocationThatReplacedIt(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openCanary(t, client)
	r.True(offers(m, "n"))

	client.alloc = nomad.Alloc{ID: "9a1b2c3d-0000-0000-0000-000000000000", Namespace: "production", JobID: "web"}

	m, cmd := m.update(key('n'))
	drain(m, cmd)

	r.Equal("9a1b2c3d-0000-0000-0000-000000000000", client.askedID)
}

func TestTasks_OpenTheFollowUpEvaluation(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := openCanary(t, client)
	r.True(offers(m, "f"))

	client.evaluation = nomad.EvaluationDetail{ID: "8b1c2d3e-0000-0000-0000-000000000000", Status: "pending"}

	m, cmd := m.update(key('f'))
	m = drain(m, cmd)

	// The evaluation that will place it again, the way the list of
	// evaluations opens one.
	r.Equal("8b1c2d3e-0000-0000-0000-000000000000", client.askedID)
	r.Equal("production", client.askedNamespace)
	r.Equal(screenDescribe, m.screen.kind)
	r.Contains(plain(m.render()), "Evaluation 8b1c2d3e")
}

func TestTasks_NowhereToGo(t *testing.T) {
	r := require.New(t)

	placed := restarted()
	placed.NodeID = "a3e23694-4528-6395-3a51-1ffcbf3c2ba4"

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), alloc: placed}
	m := openTasks(t, client)
	m = drain(m, m.fetch())

	// Nothing came before it, nothing after, nothing will place it again.
	r.False(offers(m, "p"))
	r.False(offers(m, "n"))
	r.False(offers(m, "f"))
	r.True(offers(m, "c"))
}

func TestTasks_ThePanelGivesWayOnAShortScreen(t *testing.T) {
	r := require.New(t)

	m := openCanary(t, &fakeClient{jobs: twoJobs(), allocs: twoAllocs()})
	m, _ = m.update(tea.WindowSizeMsg{Width: 120, Height: 16})

	// The tasks come first: what does not fit of the panel is left out.
	out := plain(m.render())
	r.Contains(out, "server")
	r.Contains(out, "sidecar")
	r.Len(lines(out), 16)
}

func TestTasks_ThePanelKeepsWhatFits(t *testing.T) {
	r := require.New(t)

	m := openCanary(t, &fakeClient{jobs: twoJobs(), allocs: twoAllocs()})
	m, _ = m.update(tea.WindowSizeMsg{Width: 120, Height: 18})

	// Its first lines, then the tasks.
	out := plain(m.render())
	r.Contains(out, "Status running")
	r.Contains(out, "Ports http")
	r.NotContains(out, "Reschedules")
	r.Contains(out, "sidecar")
	r.Len(lines(out), 18)
}

func TestFitPanel_LeavesTheHeadItWasGiven(t *testing.T) {
	r := require.New(t)

	// A head with room behind it: what the panel adds must not land in the
	// slice of whoever built the head.
	backing := []string{"status", "kept"}
	head := backing[:1]

	// An empty block adds no rows, only the line of air under the head.
	rows := fitPanel(head, panelBlock{}, 10)

	r.Equal([]string{"status", "kept"}, backing)
	r.Equal("status", rows[0])
}
