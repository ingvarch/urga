package ui

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// troubledCluster has something wrong on most lines of the overview, and a
// healthy resource of each kind beside it.
func troubledCluster() *fakeClient {
	restarting := nomad.Task{Name: "server", Restarts: 1, LastRestart: time.Now().Add(-10 * time.Minute)}
	replaced := allocFaultsAlloc("replaced", statusFailed)
	replaced.Next = "newer"

	return &fakeClient{
		changes: newChanges(),
		jobs: []nomad.Job{
			{ID: "crashed", Namespace: "production", Type: "service", Status: "dead", Datacenters: []string{"dc1"}},
			{ID: "late", Namespace: "production", Type: "service", Status: "dead", Datacenters: []string{"dc2"}},
			{ID: "web", Namespace: "production", Type: "service", Status: "running", Running: 1, Desired: 1},
		},
		allocs: []nomad.Alloc{
			allocFaultsAlloc("failed", statusFailed),
			allocFaultsAlloc("lost", statusLost),
			replaced,
			allocFaultsAlloc("restarting", statusRunning, restarting),
			allocFaultsAlloc("healthy", statusRunning, nomad.Task{Name: "server"}),
		},
		nodes: []nomad.Node{
			{ID: "down", Datacenter: "dc1", Status: "down", Eligibility: "ineligible"},
			{ID: "down-far", Datacenter: "dc2", Status: "down", Eligibility: "ineligible"},
			{ID: "draining", Datacenter: "dc1", Status: "ready", Eligibility: "ineligible", Drain: true},
			{ID: "cordoned", Datacenter: "dc1", Status: "ready", Eligibility: "ineligible"},
			{ID: "ok", Datacenter: "dc1", Status: "ready", Eligibility: "eligible"},
		},
		evaluations: []nomad.Evaluation{
			{ID: "e1", JobID: "web", Namespace: "production", Status: "blocked"},
			{ID: "e2", JobID: "api", Namespace: "production", Status: "blocked"},
			{ID: "e3", JobID: "db", Namespace: "production", Status: "failed"},
			{ID: "e4", JobID: "cron", Namespace: "production", Status: "complete"},
		},
		deployments: []nomad.Deployment{
			{ID: "d1", JobID: "web", Namespace: "production", JobVersion: 2, Status: "failed"},
			{ID: "d2", JobID: "api", Namespace: "production", JobVersion: 1, Status: "running"},
			{ID: "d3", JobID: "db", Namespace: "production", JobVersion: 1, Status: "running"},
			{ID: "d4", JobID: "cron", Namespace: "production", JobVersion: 1, Status: "successful"},
		},
	}
}

// overviewLines are the lines of the overview as it reads, in order.
var overviewLines = [][2]string{
	{"Jobs", "dead"},
	{"Allocations", "failed or lost"},
	{"Allocations", "restarting"},
	{"Allocations", "OOM killed"},
	{"Clients", "down"},
	{"Clients", "disconnected"},
	{"Clients", "draining"},
	{"Clients", "ineligible"},
	{"Evaluations", "blocked"},
	{"Evaluations", "failed"},
	{"Deployments", "failed"},
	{"Deployments", "paused"},
	{"Deployments", "running"},
}

// overviewOpen is the overview open on a cluster, before it answered.
func overviewOpen(client *fakeClient) Model {
	m := newTestModel(client)
	m, _ = m.update(openMsg{overviewPage{}})

	return m
}

// overviewAnswered is the overview after the cluster answered every list.
func overviewAnswered(client *fakeClient) Model {
	m := overviewOpen(client)

	return drain(m, m.fetch())
}

// overviewWithCounts is the overview with a count above zero on its first
// two lines: a dead job and a failed allocation.
func overviewWithCounts(client *fakeClient) Model {
	dead := nomad.Job{ID: "crashed", Namespace: "production", Type: "service", Status: "dead"}
	failed := nomad.Alloc{ID: "failed-1", Namespace: "production", JobID: "web", TaskGroup: "frontend", Status: statusFailed}

	m := typeCommand(newTestModel(client), "overview")

	for _, part := range []tea.Msg{
		partMsg[nomad.Job]{namespace: "production", at: time.Now(), items: append([]nomad.Job{dead}, client.jobs...)},
		partMsg[nomad.Alloc]{namespace: "production", at: time.Now(), items: append([]nomad.Alloc{failed}, client.allocs...)},
	} {
		m, _ = m.update(part)
	}

	return m
}

// overviewCells are the lines of the overview as the table shows them.
func overviewCells(m Model) []string {
	out := []string{}

	for _, row := range m.list.table.rows {
		out = append(out, strings.Join(row.cells, "|"))
	}

	return out
}

// overviewCounts are the Count cells, in the order of the lines.
func overviewCounts(m Model) []string { return cellsOf(m.list.table, 2) }

func TestOverview_OpensFromTheCommandLine(t *testing.T) {
	r := require.New(t)

	for _, word := range []string{"overview", "ov"} {
		m := typeCommand(newTestModel(&fakeClient{}), word)

		r.Equal(overviewView, m.screen.view, word)
		r.Equal("Overview (production)", m.title(), word)
	}
}

func TestOverview_AsksForEachListInTheNamespaceOfTheSession(t *testing.T) {
	r := require.New(t)

	client := troubledCluster()
	m := overviewOpen(client)
	drain(m, m.fetch())

	r.Equal(map[string][]string{
		"jobs":        {"production"},
		"allocations": {"production/"},
		"nodes":       {""},
		"evaluations": {"production"},
		"deployments": {"production"},
	}, client.lists)
}

func TestOverview_WatchesWhatItCounts(t *testing.T) {
	r := require.New(t)

	r.Equal([]string{
		nomad.TopicJob, nomad.TopicAllocation, nomad.TopicNode, nomad.TopicEvaluation, nomad.TopicDeployment,
	}, overviewPage{}.topics())

	client := troubledCluster()
	m := overviewOpen(client)

	m.update(m.watchScreen()())
	r.Equal("production", client.watchedNamespace)
	r.Equal(overviewPage{}.topics(), client.watchedTopics)
}

func TestOverview_NothingIsCountedBeforeTheClusterAnswers(t *testing.T) {
	r := require.New(t)

	m := overviewOpen(troubledCluster())

	r.Len(m.list.table.rows, len(overviewLines))

	for _, count := range overviewCounts(m) {
		r.Equal("n/a", count)
	}

	_, offered := m.offeredKey("enter")
	r.False(offered)
}

func TestOverview_CountsWhatNeedsAttention(t *testing.T) {
	r := require.New(t)

	m := overviewAnswered(troubledCluster())

	r.Equal([]string{
		"Jobs|dead|2",
		"Allocations|failed or lost|2",
		"Allocations|restarting|1",
		"Allocations|OOM killed|0",
		"Clients|down|2",
		"Clients|disconnected|0",
		"Clients|draining|1",
		"Clients|ineligible|1",
		"Evaluations|blocked|2",
		"Evaluations|failed|1",
		"Deployments|failed|1",
		"Deployments|paused|0",
		"Deployments|running|2",
	}, overviewCells(m))
}

func TestOverview_ACountIsTheRowsOfTheListItOpens(t *testing.T) {
	r := require.New(t)

	m := overviewAnswered(troubledCluster())
	page := m.screen.page.(overviewPage)
	e := m.env()

	r.Len(overviewRows, len(overviewLines))

	for i, row := range overviewRows {
		list := row.list(page.read)

		r.Equal(overviewLines[i][1], row.fault.String())
		r.Equal(sprintf("%d", len(list.rows(e))), overviewCounts(m)[i], row.resource+" "+row.fault.String())

		switch opened := list.(type) {
		case jobsPage:
			r.Equal("Jobs", row.resource)
			r.Equal(row.fault, opened.fault)
		case allocationsPage:
			r.Equal("Allocations", row.resource)
			r.Equal(row.fault, opened.fault)
		case nodesPage:
			r.Equal("Clients", row.resource)
			r.Equal(row.fault, opened.fault)
		case evaluationsPage:
			r.Equal("Evaluations", row.resource)
			r.Equal(row.fault, opened.fault)
		case deploymentsPage:
			r.Equal("Deployments", row.resource)
			r.Equal(row.fault, opened.fault)
		default:
			r.Failf("unknown list", "%T", list)
		}
	}
}

func TestOverview_ALineTakesTheColorOfWhatItCounts(t *testing.T) {
	r := require.New(t)

	m := overviewAnswered(troubledCluster())
	page := m.screen.page.(overviewPage)
	e := m.env()

	for i, line := range m.rows() {
		counted := overviewRows[i].list(page.read).rows(e)

		if len(counted) == 0 {
			r.Equal(colorSpent, line.color, overviewLines[i])

			continue
		}

		for _, row := range counted {
			r.Equal(counted[0].color, row.color, overviewLines[i])
		}

		r.Equal(counted[0].color, line.color, overviewLines[i])
	}
}

func TestOverview_ARefusalShowsInItsLinesAndTheRestStays(t *testing.T) {
	r := require.New(t)

	client := troubledCluster()
	client.nodesErr = forbidden(t)

	m := overviewAnswered(client)

	counts := overviewCounts(m)
	for i, line := range overviewLines {
		if line[0] == "Clients" {
			r.Equal("unknown: Permission denied", counts[i], line[1])
			r.Equal(colorSpent, m.rows()[i].color)

			continue
		}

		r.NotContains(counts[i], "unknown", line[1])
		r.NotEqual("n/a", counts[i], line[1])
	}

	// Five answers arrive one after another and each one clears the status
	// line, so a refusal is shown on its lines and nowhere else.
	r.NotEqual(flashErr, m.flash.level)
}

func TestOverview_AnErrorGoesAwayWithTheNextAnswer(t *testing.T) {
	r := require.New(t)

	client := troubledCluster()
	client.allocsErr = errors.New("connection refused")

	m := overviewAnswered(client)

	r.Equal("unknown: connection refused", overviewCounts(m)[1])
	r.Equal("2", overviewCounts(m)[0])

	client.allocsErr = nil
	m = drain(m, m.fetch())

	r.Equal([]string{"2", "1", "0"}, overviewCounts(m)[1:4])
}

func TestOverview_ACountOfAnotherNamespaceIsNotShown(t *testing.T) {
	r := require.New(t)

	m := overviewAnswered(troubledCluster())
	m.namespaceOrder = []string{"production", "staging"}

	// The answers are those of production, and the session moves on.
	m, _ = m.update(key('2'))

	r.Equal("staging", m.namespace)
	r.Equal("Overview (staging)", m.title())

	for i, line := range overviewLines {
		if line[0] == "Clients" {
			r.NotEqual("n/a", overviewCounts(m)[i], line[1])

			continue
		}

		r.Equal("n/a", overviewCounts(m)[i], line[0]+" "+line[1])
	}
}

func TestOverview_FollowsTheDatacenter(t *testing.T) {
	r := require.New(t)

	m := overviewAnswered(troubledCluster())
	before := overviewCounts(m)

	m.datacenter = "dc1"
	m.layout()

	after := overviewCounts(m)

	// A job and a client of another datacenter drop out; allocations,
	// evaluations and deployments are not narrowed by datacenter.
	for i, line := range overviewLines {
		switch line {
		case overviewLines[0], overviewLines[4]:
			r.NotEqual(before[i], after[i], line)
		default:
			r.Equal(before[i], after[i], line)
		}
	}

	r.Equal("1", after[0])
	r.Equal("1", after[4])
}

func TestOverview_ANarrowTerminalKeepsTheCounts(t *testing.T) {
	r := require.New(t)

	client := troubledCluster()
	client.nodesErr = forbidden(t)

	m := overviewAnswered(client)
	m, _ = m.update(tea.WindowSizeMsg{Width: 40, Height: 30})

	// The words of a line may be cut, a number never is.
	out := lines(m.render())
	header := slices.IndexFunc(out, func(line string) bool { return strings.Contains(line, "Resource") })
	r.NotEqual(-1, header)

	for i, line := range overviewLines {
		if line[0] == "Clients" {
			continue
		}

		fields := strings.Fields(strings.Trim(out[header+1+i], "│ "))
		r.Equal(overviewCounts(m)[i], fields[len(fields)-1], line)
	}
}

func TestOverview_ToggleFaultsKeepsTheLinesThatNeedAttention(t *testing.T) {
	r := require.New(t)

	m := overviewAnswered(troubledCluster())
	m, _ = m.update(key('!'))

	kept := []string{}

	for _, row := range m.list.table.rows {
		kept = append(kept, row.cells[0]+" "+row.cells[1])
	}

	// Running deployments are yellow, and a zero is grey: neither needs
	// attention.
	r.Equal([]string{
		"Jobs dead",
		"Allocations failed or lost",
		"Allocations restarting",
		"Clients down",
		"Clients draining",
		"Clients ineligible",
		"Evaluations blocked",
		"Evaluations failed",
		"Deployments failed",
	}, kept)
}

func TestOverview_TheCursorStaysOnItsLine(t *testing.T) {
	r := require.New(t)

	m := overviewAnswered(troubledCluster())
	m.list.table.cursor = 6
	r.Equal("Clients|draining|1", overviewCells(m)[6])

	m, _ = m.update(key('!'))

	row, ok := m.list.table.selected()
	r.True(ok)
	r.Equal("Clients|draining", row.cells[0]+"|"+row.cells[1])
}

// overviewAt is the overview with the cursor on a line.
func overviewAt(t *testing.T, m Model, resource, state string) Model {
	t.Helper()

	i := slices.Index(overviewLines, [2]string{resource, state})
	require.GreaterOrEqual(t, i, 0, "no line %s %s", resource, state)

	m.list.table.cursor = i

	return m
}

func TestOverview_EnterOpensWhatTheLineCounts(t *testing.T) {
	r := require.New(t)

	m := overviewAt(t, overviewAnswered(troubledCluster()), "Allocations", "restarting")

	opened, _ := m.handleKey(enter())

	page, ok := opened.screen.page.(allocationsPage)
	r.True(ok)
	r.Equal(faultRestarting, page.fault)
	r.Equal("Allocations (production, restarting) [1]", opened.title())
	r.Len(opened.list.table.rows, 1)
}

func TestOverview_EnterIsOfferedOnlyOnACount(t *testing.T) {
	r := require.New(t)

	answered := overviewAnswered(troubledCluster())

	_, offered := overviewAt(t, answered, "Jobs", "dead").offeredKey("enter")
	r.True(offered)

	_, offered = overviewAt(t, answered, "Allocations", "OOM killed").offeredKey("enter")
	r.False(offered, "a zero")

	unread := overviewAt(t, overviewOpen(troubledCluster()), "Jobs", "dead")
	_, offered = unread.offeredKey("enter")
	r.False(offered, "n/a")

	switched := answered
	switched.namespaceOrder = []string{"production", "staging"}
	switched, _ = switched.update(key('2'))
	switched = overviewAt(t, switched, "Jobs", "dead")

	r.Equal("n/a", overviewCounts(switched)[slices.Index(overviewLines, [2]string{"Jobs", "dead"})])
	_, offered = switched.offeredKey("enter")
	r.False(offered, "n/a with the lists of another namespace")

	// The refused answer carries clients, so only its error keeps enter off
	// the line.
	refused, _ := overviewAnswered(troubledCluster()).update(
		partMsg[nomad.Node]{items: troubledCluster().nodes, err: forbidden(t)})
	refused = overviewAt(t, refused, "Clients", "down")

	r.Equal("unknown: Permission denied", overviewCounts(refused)[slices.Index(overviewLines, [2]string{"Clients", "down"})])
	_, offered = refused.offeredKey("enter")
	r.False(offered, "unknown")
}

func TestOverview_EscapeComesBackToTheLine(t *testing.T) {
	r := require.New(t)

	m := overviewAt(t, overviewAnswered(troubledCluster()), "Evaluations", "blocked")
	counts := overviewCounts(m)

	back := openAndBack(t, m, enter())

	r.IsType(overviewPage{}, back.screen.page)
	r.Equal("Evaluations", cursorName(back))
	r.Equal(counts, overviewCounts(back))
	r.Equal("blocked", back.list.table.rows[back.list.table.cursor].cells[1])
}

func TestOverview_AnOpenedListIsNotWhatTheNextRunOpens(t *testing.T) {
	r := require.New(t)

	m, cfg := sessionModel(t, troubledCluster())
	m = typeCommand(m, "overview")
	m = drain(m, m.fetch())

	opened, _ := m.handleKey(enter())
	r.IsType(jobsPage{}, opened.screen.page)

	r.Equal("overview", cfg.Of("").Screen)
}

func TestOverview_RestartsAreJudgedAtTheTimeOfTheRead(t *testing.T) {
	r := require.New(t)

	now := time.Now()
	restarted := func(id string, ago time.Duration) nomad.Alloc {
		return allocFaultsAlloc(id, statusRunning,
			nomad.Task{Name: "server", Restarts: 1, LastRestart: now.Add(-ago)})
	}

	m := overviewOpen(&fakeClient{changes: newChanges()})
	m, _ = m.update(partMsg[nomad.Alloc]{
		namespace: "production", at: now.Add(-2 * time.Hour),
		items: []nomad.Alloc{restarted("aaaa1111", 2*time.Hour+10*time.Minute), restarted("bbbb2222", 3*time.Hour+30*time.Minute)},
	})

	i := slices.Index(overviewLines, [2]string{"Allocations", "restarting"})
	r.Equal("1", overviewCounts(m)[i])

	opened, _ := overviewAt(t, m, "Allocations", "restarting").handleKey(enter())

	r.Len(opened.list.table.rows, 1)
	r.Equal("aaaa1111", opened.list.table.rows[0].cells[0][:8])
}
