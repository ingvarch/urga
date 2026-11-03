package ui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func threeNamespaces() []nomad.Namespace {
	return []nomad.Namespace{{Name: "default"}, {Name: "production"}, {Name: "staging"}}
}

func TestNamespaceKeys_SwitchTheSession(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), namespaces: threeNamespaces()}
	m := newTestModel(client)
	m, _ = m.update(namespacesMsg(threeNamespaces()))

	// The keys are in the header, numbered from one; zero selects every
	// namespace.
	head := headerOf(m)
	r.Contains(head, "<0> all")
	r.Contains(head, "<1> default")
	r.Contains(head, "<3> staging")

	m, cmd := m.update(key('3'))
	r.Equal("staging", m.namespace)

	m = drain(m, cmd)
	r.Equal("staging", client.askedNamespace)

	// Zero selects every namespace, and its key is highlighted.
	m, _ = m.update(key('0'))
	r.Equal(nomad.AllNamespaces, m.namespace)
	r.Contains(renderHeader(m.headerData(), m.width-2*headerPadX), styleKey.Render(pad("all", len("production"))))
}

func TestNamespaceKeys_KeepTheirOrder(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{namespaces: threeNamespaces()})
	m, _ = m.update(namespacesMsg(threeNamespaces()))

	m, _ = m.update(key('3'))
	r.Equal("staging", m.namespace)

	// The cluster answers with the list in another order. The keys do not
	// move: a number means the same namespace as a moment ago.
	m, _ = m.update(namespacesMsg([]nomad.Namespace{{Name: "staging"}, {Name: "default"}, {Name: "production"}}))

	head := headerOf(m)
	r.Contains(head, "<1> default")
	r.Contains(head, "<3> staging")

	m, _ = m.update(key('1'))
	r.Equal("default", m.namespace)
}

func TestNamespaceKeys_MarkTheOneInUse(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{namespaces: threeNamespaces()})
	m, _ = m.update(namespacesMsg(threeNamespaces()))
	m, _ = m.update(key('2'))

	r.Contains(headerOf(m), "<2> production")

	// The one in use is the only one highlighted; the rest are muted.
	head := renderHeader(m.headerData(), m.width-2*headerPadX)
	r.Contains(head, styleKey.Render("production"))
	r.Contains(head, styleMuted.Render(pad("default", len("production"))))
	r.NotContains(head, styleKey.Render(pad("default", len("production"))))
}

func TestNamespaceKeys_OnlyNine(t *testing.T) {
	r := require.New(t)

	many := make([]nomad.Namespace, 0, 12)
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"} {
		many = append(many, nomad.Namespace{Name: name})
	}

	m := newTestModel(&fakeClient{namespaces: many})
	m, _ = m.update(namespacesMsg(many))

	// There are only nine keys; the rest of the namespaces are reached
	// through the command line.
	head := headerOf(m)
	r.Contains(head, "<9> i")
	r.NotContains(head, "<10>")
	r.NotContains(head, " j")
}

func TestNamespaceKeys_NothingToSwitchTo(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{})

	// A key that points at no namespace leaves the session where it is.
	m, cmd := m.update(key('5'))

	r.Equal("production", m.namespace)
	r.Nil(cmd)
}

func TestNamespaceKeys_DropTheAnswerAskedBefore(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{jobs: twoJobs(), namespaces: threeNamespaces()})
	m, _ = m.update(namespacesMsg(threeNamespaces()))

	// The request for the jobs of production still runs when the session
	// switches.
	late := m.fetch()

	m, _ = m.update(key('3'))
	m, _ = m.update(late())

	// They must not show under the name of staging.
	out := plain(m.render())
	r.Contains(out, "Jobs (staging) [0]")
	r.NotContains(out, "cron")
}

func TestNamespaceState_KeysAndNames(t *testing.T) {
	r := require.New(t)

	s := namespaceState{namespace: "staging", namespaces: threeNamespaces()}
	s.rememberNamespaces(threeNamespaces())

	r.Equal([]string{"default", "production", "staging"}, s.namespaceOrder)

	keys := s.namespaceColumnData()
	r.Equal("<0>", keys[0].Key)
	r.Equal("<3>", keys[3].Key)
	r.True(keys[3].Active)
	r.False(keys[0].Active)

	r.True(s.knowsNamespace("production"))
	r.False(s.knowsNamespace("nowhere"))
}

func TestNamespaceKey_AScreenOpenedForAJobStaysInItsNamespace(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), changes: newChanges()}
	m := newTestModel(client)
	m.namespaceOrder = []string{"production", "staging"}
	m, _ = m.update(jobsMsg(client.jobs))

	m, cmd := m.update(enter())
	m = playOut(m, cmd)
	r.Contains(plain(m.render()), "Allocations (Job: web)")

	// The job is in production: the requests about it, and the stream that
	// watches it, stay there.
	m, cmd = m.update(key('2'))
	m = playOut(m, cmd)

	r.Equal("staging", m.namespace)
	r.Contains(plain(m.render()), "Allocations (Job: web)")
	r.Equal("production", client.askedNamespace)
	r.Equal("production", client.watchedNamespace)
}

func TestCommand_ANamespaceGivenLeavesTheScreenBelowWhereItIs(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), namespaces: threeNamespaces(), changes: newChanges()}
	m := newTestModel(client)
	m, _ = m.update(namespacesMsg(threeNamespaces()))
	m, _ = m.update(jobsMsg(client.jobs))

	m, cmd := m.update(enter())
	m = playOut(m, cmd)
	r.Contains(plain(m.render()), "Allocations (Job: web)")

	// The namespace is the session's to switch; the allocations of a job in
	// production stay there when the list opens again under the jobs.
	m = typeCommand(m, "jobs staging")
	m, cmd = m.update(escape())
	m = playOut(m, cmd)

	r.Contains(plain(m.render()), "Allocations (Job: web)")
	r.Equal("production", client.askedNamespace)
	r.Equal("production", client.watchedNamespace)
}

func TestNamespaces_OneThatIsGoneLosesItsNumberKey(t *testing.T) {
	r := require.New(t)

	m := onNamespaces(t, &fakeClient{}, &fakeEditor{})
	r.Equal([]string{"production", "staging"}, m.namespaceOrder)

	// The cluster no longer names production.
	m, _ = m.update(namespacesMsg([]nomad.Namespace{{Name: "staging"}}))

	r.Equal([]string{"staging"}, m.namespaceOrder)
}

func TestNamespaceKeys_EveryListOfTheNamespaceLeftIsEmptied(t *testing.T) {
	tests := []struct {
		name  string
		page  page
		read  tea.Msg
		title string
	}{
		{"jobs", jobsPage{}, jobsMsg(twoJobs()), "Jobs (staging) [0]"},
		{"dead jobs", jobsPage{fault: faultDead}, jobsMsg(deadJobsFixture()), "Jobs (staging, dead) [0]"},
		{"allocations", allocationsPage{}, allocsMsg(twoAllocs()), "Allocations (staging) [0]"},
		{"failed allocations", allocationsPage{fault: faultFailedOrLost}, allocsMsg{{ID: "a1", JobID: "web", Namespace: "production", Status: "failed"}}, "Allocations (staging, failed or lost) [0]"},
		{"tasks", namespaceTasksPage{}, allocsMsg(twoAllocs()), "Tasks (staging) [0]"},
		{"deployments", deploymentsPage{}, deploymentsMsg{{ID: "d1", JobID: "web", Namespace: "production", Status: "running"}}, "Deployments (staging) [0]"},
		{"failed deployments", deploymentsPage{fault: faultFailed}, deploymentsMsg{{ID: "d1", JobID: "web", Namespace: "production", Status: "failed"}}, "Deployments (staging, failed) [0]"},
		{"evaluations", evaluationsPage{}, evaluationsMsg{{ID: "e1", JobID: "web", Namespace: "production", Status: "complete"}}, "Evaluations (staging) [0]"},
		{"blocked evaluations", evaluationsPage{fault: faultBlocked}, evaluationsMsg{{ID: "e1", JobID: "web", Namespace: "production", Status: "blocked"}}, "Evaluations (staging, blocked) [0]"},
		{"services", servicesPage{}, servicesMsg{{Name: "web", Namespace: "production"}}, "Services (staging) [0]"},
		{"scaling policies", scalingPoliciesPage{}, scalingPoliciesMsg(twoPolicies()), "Scaling Policies (staging) [0]"},
		{"variables", variablesPage{}, variablesMsg{webVariable().Variable}, "Variables (staging) [0]"},
		{"volumes", volumesPage{}, volumesMsg(twoVolumes()), "Volumes (staging) [0]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := require.New(t)

			m := newTestModel(&fakeClient{err: errors.New("connection refused")})
			m.namespaceOrder = []string{"production", "staging"}
			m, _ = m.update(openMsg{tt.page})
			m, _ = m.update(tt.read)
			r.NotEmpty(m.rows())

			m, _ = m.update(key('2'))

			r.Empty(m.rows())
			r.Contains(plain(m.render()), tt.title)
		})
	}
}

func TestCommand_ANamespaceGivenEmptiesTheListOfTheNamespaceLeft(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m, _ := onVariables(t, client, "production")
	m, _ = m.update(namespacesMsg(threeNamespaces()))
	client.err = errors.New("connection refused")

	m = typeCommand(m, "variables staging")

	out := plain(m.render())
	r.Contains(out, "Variables (staging) [0]")
	r.NotContains(out, "nomad/jobs/web")
}

func TestNamespaceKeys_AListUnderTheScreenIsEmptiedToo(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), changes: newChanges()}
	m := newTestModel(client)
	m.namespaceOrder = []string{"production", "staging", "dev"}
	m, _ = m.update(jobsMsg(client.jobs))

	m, cmd := m.update(enter())
	m = playOut(m, cmd)
	r.Contains(plain(m.render()), "Allocations (Job: web)")

	m, cmd = m.update(key('3'))
	m = playOut(m, cmd)

	client.err = errors.New("connection refused")
	m, cmd = m.update(escape())
	m = drain(m, cmd)

	out := plain(m.render())
	r.Contains(out, "Jobs (dev) [0]")
	r.NotContains(out, "cron")
}

func TestNamespaceKeys_TheClientsKeepTheirRows(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{err: errors.New("connection refused")})
	m.namespaceOrder = []string{"production", "staging"}
	m, _ = m.update(openMsg{nodesPage{}})
	m, _ = m.update(nodesMsg(busyClient()))
	r.NotEmpty(m.rows())

	m, _ = m.update(key('2'))

	r.NotEmpty(m.rows())
}

func TestNamespaceSwitch_AListLeftComesBackLikeOneEnteredFresh(t *testing.T) {
	stagingJobs := []nomad.Job{
		{ID: "api", Name: "api", Namespace: "staging", Type: "service", Status: "running"},
		{ID: "db", Name: "db", Namespace: "staging", Type: "service", Status: "running"},
	}

	moves := []struct {
		name string
		move func(Model) Model
	}{
		{name: "the cursor", move: func(m Model) Model { m, _ = m.update(down()); return m }},
		{name: "the filter", move: func(m Model) Model {
			m, _ = m.update(key('/'))
			m = typeIn(m, "web")
			m, _ = m.update(enter())

			return m
		}},
		{name: "the sort", move: func(m Model) Model { m, _ = m.update(key('N')); return m }},
	}

	// A number key switches with the list under another screen; a command
	// switches with the list open, and puts its screen on top.
	switches := []struct {
		name  string
		leave func(Model) Model
	}{
		{name: "a number key", leave: func(m Model) Model {
			m, cmd := m.update(enter())
			m = playOut(m, cmd)
			m, cmd = m.update(key('2'))

			return playOut(m, cmd)
		}},
		{name: "a command", leave: func(m Model) Model { return typeCommand(m, "variables staging") }},
	}

	for _, sw := range switches {
		for _, test := range moves {
			t.Run(test.name+" after "+sw.name, func(t *testing.T) {
				r := require.New(t)

				client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), changes: newChanges()}
				m := newTestModel(client)
				m, _ = m.update(namespacesMsg(threeNamespaces()))
				m.namespaceOrder = []string{"production", "staging"}
				m, _ = m.update(jobsMsg(client.jobs))
				m = test.move(m)

				m = sw.leave(m)

				client.jobs = stagingJobs
				m, cmd := m.update(escape())
				m = drain(m, cmd)

				r.Contains(plain(m.render()), "Jobs (staging) [2]")
				r.Zero(m.list.table.cursor)
				r.Empty(m.list.filter)
				r.Equal(newSortState(), m.list.sort)
			})
		}
	}
}
