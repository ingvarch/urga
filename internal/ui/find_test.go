package ui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// foundWeb is what a search for "web" finds, a match of each kind that opens
// differently.
func foundWeb() nomad.Found {
	return nomad.Found{Matches: []nomad.Match{
		{Kind: nomad.MatchJob, Name: "web", Namespace: "production", ID: "web"},
		{Kind: nomad.MatchGroup, Name: "frontend", Namespace: "production", JobID: "web", Group: "frontend"},
		{Kind: nomad.MatchTask, Name: "server", Namespace: "production", JobID: "web", Group: "frontend", Task: "server"},
		{Kind: nomad.MatchService, Name: "web-http", Namespace: "production", JobID: "web", Group: "frontend"},
		{Kind: nomad.MatchCommand, Name: "/bin/web", Namespace: "production", JobID: "web", Group: "frontend", Task: "server"},
		{Kind: nomad.MatchAlloc, Name: "web.frontend[0]", Namespace: "staging", ID: "bb19fb79-0d79-6a6d-97fa-319ec77d542e"},
		{Kind: nomad.MatchNode, Name: "web-node-01", ID: "03f874f0-dcec-5b67-2d83-791be144dcc4"},
		{Kind: nomad.MatchPool, Name: "web-pool", ID: "web-pool"},
		{Kind: nomad.MatchNamespace, Name: "staging", ID: "staging"},
		{Kind: nomad.MatchVariable, Name: "nomad/jobs/web", Namespace: "production", ID: "nomad/jobs/web"},
		{Kind: nomad.MatchHostVolume, Name: "web-data", Namespace: "production", ID: "7f3c1a2b-0000-0000-0000-000000000000"},
		{Kind: nomad.MatchPlugin, Name: "web-csi", ID: "web-csi"},
		{Kind: nomad.MatchEval, Name: "bb1c0000-0000-0000-0000-000000000000", ID: "bb1c0000-0000-0000-0000-000000000000"},
		{Kind: nomad.MatchDeployment, Name: "bb1d0000-0000-0000-0000-000000000000", ID: "bb1d0000-0000-0000-0000-000000000000"},
	}}
}

// finding is a cluster where a search for "web" finds foundWeb.
func finding() *fakeClient {
	return &fakeClient{jobs: twoJobs(), found: foundWeb()}
}

func TestFind_TheCommand(t *testing.T) {
	r := require.New(t)

	for _, line := range []string{"find web", "search web"} {
		client := finding()
		m := typeCommand(newTestModel(client), line)

		r.IsType(findPage{}, m.screen.page, line)
		r.Equal("web", client.findText)
		r.Equal(`Found "web" [14]`, m.title())
	}
}

func TestFind_TooShortToAsk(t *testing.T) {
	r := require.New(t)

	for _, line := range []string{"find w", "find"} {
		client := finding()
		m := newTestModel(client)
		m, _ = m.update(jobsMsg(twoJobs()))
		m = typeCommand(m, line)

		// The cluster refuses it; nothing is sent, and the screen stays.
		r.IsType(jobsPage{}, m.screen.page, line)
		r.Zero(client.findCalls, line)
		r.Contains(plain(statusLine(m)), "Type at least 2 characters to find.", line)
	}
}

func TestFind_TheRows(t *testing.T) {
	r := require.New(t)

	m := typeCommand(newTestModel(finding()), "find web")

	cells := [][]string{}
	for _, row := range m.rows() {
		cells = append(cells, row.cells)
	}

	// Where each is, in the job it is part of, or by the ID it has.
	r.Equal([][]string{
		{"job", "web", "production", ""},
		{"group", "frontend", "production", "web"},
		{"task", "server", "production", "web / frontend"},
		{"service", "web-http", "production", "web / frontend"},
		{"command", "/bin/web", "production", "web / frontend / server"},
		{"alloc", "web.frontend[0]", "staging", "bb19fb79"},
		{"node", "web-node-01", "-", "03f874f0"},
		{"pool", "web-pool", "-", ""},
		{"namespace", "staging", "-", ""},
		{"variable", "nomad/jobs/web", "production", ""},
		{"host volume", "web-data", "production", "7f3c1a2b"},
		{"plugin", "web-csi", "-", ""},
		{"eval", "bb1c0000-0000-0000-0000-000000000000", "-", ""},
		{"deployment", "bb1d0000-0000-0000-0000-000000000000", "-", ""},
	}, cells)
}

func TestFind_AsksOnce(t *testing.T) {
	r := require.New(t)

	client := finding()
	m := typeCommand(newTestModel(client), "find web")

	// A search is as the cluster was when it was asked; the screen is not
	// polled for another.
	for range 3 {
		var cmd tea.Cmd

		m, cmd = m.update(pollMsg{})
		m = playOut(m, cmd)
	}

	r.Equal(1, client.findCalls)
}

func TestFind_WhatTheClusterCutShort(t *testing.T) {
	r := require.New(t)

	client := finding()
	client.found.Truncated = []string{nomad.MatchJob, nomad.MatchAlloc}

	m := typeCommand(newTestModel(client), "find web")

	r.Contains(plain(statusLine(m)), "Not every job and alloc is shown: type more to narrow the search.")
}

// openMatch opens the match of a row of the search for "web".
func openMatch(t *testing.T, client *fakeClient, row int) Model {
	t.Helper()

	m := typeCommand(newTestModel(client), "find web")
	for range row {
		m, _ = m.update(key('j'))
	}

	m, cmd := m.update(enter())

	return playOut(m, cmd)
}

func TestFind_AJobOpensAlone(t *testing.T) {
	r := require.New(t)

	client := finding()
	client.jobs = append(twoJobs(), nomad.Job{ID: "web-api", Name: "web-api", Namespace: "production", Type: "service", Status: "running"})

	m := openMatch(t, client, 0)

	// In the list of jobs, with every key of a job, and no other job.
	r.IsType(jobsPage{}, m.screen.page)
	r.Equal("Job web (production) [1]", m.title())
	r.Equal("production", client.askedNamespace)
	r.Contains(m.hints(), hint{Key: "<v>", Description: "Versions"})
}

func TestFind_APartOfAJobOpensItsGroup(t *testing.T) {
	r := require.New(t)

	for _, row := range []int{1, 2, 4} {
		m := openMatch(t, finding(), row)

		allocs, ok := m.screen.page.(allocationsPage)
		r.True(ok, "row %d: %T", row, m.screen.page)
		r.Equal(allocationsPage{namespace: "production", jobID: "web", group: "frontend"}, allocationsPage{
			namespace: allocs.namespace, jobID: allocs.jobID, group: allocs.group,
		})
	}
}

func TestFind_AServiceOpensItsInstances(t *testing.T) {
	r := require.New(t)

	client := finding()
	m := openMatch(t, client, 3)

	r.IsType(serviceInstancesPage{}, m.screen.page)
	r.Equal("production", client.instancesNamespace)
	r.Equal("web-http", client.instancesName)
}

func TestFind_AnAllocIsReadFirst(t *testing.T) {
	r := require.New(t)

	client := finding()
	client.alloc = nomad.Alloc{ID: "bb19fb79-0d79-6a6d-97fa-319ec77d542e", Namespace: "staging", JobID: "web-staging", Status: "running"}

	m := openMatch(t, client, 5)

	tasks, ok := m.screen.page.(tasksPage)
	r.True(ok, "%T", m.screen.page)
	r.Equal("bb19fb79-0d79-6a6d-97fa-319ec77d542e", client.askedID)
	r.Equal("staging", tasks.namespace)
	r.Equal("web-staging", tasks.jobID)
}

func TestFind_ANodeOpensTheClient(t *testing.T) {
	r := require.New(t)

	m := openMatch(t, finding(), 6)

	client, ok := m.screen.page.(clientPage)
	r.True(ok, "%T", m.screen.page)
	r.Equal("03f874f0-dcec-5b67-2d83-791be144dcc4", client.nodeID)
}

func TestFind_APoolHasNothingToOpen(t *testing.T) {
	r := require.New(t)

	m := typeCommand(newTestModel(finding()), "find web")
	r.Contains(m.hints(), hint{Key: "<enter>", Description: "Open"})

	for range 7 {
		m, _ = m.update(key('j'))
	}

	r.NotContains(m.hints(), hint{Key: "<enter>", Description: "Open"})
}

func TestFind_ANamespaceSwitchesTheSession(t *testing.T) {
	r := require.New(t)

	client := finding()
	client.namespaces = twoNamespaces()

	m := typeCommand(newTestModel(client), "find web")
	m.namespaces = twoNamespaces()

	for range 8 {
		m, _ = m.update(key('j'))
	}

	m, cmd := m.update(enter())
	m = playOut(m, cmd)

	r.Equal("staging", m.namespace)
	r.IsType(jobsPage{}, m.screen.page)
	r.Equal("Jobs (staging) [2]", m.title())
}

func TestFind_TheRestOpenTheirScreens(t *testing.T) {
	r := require.New(t)

	m := openMatch(t, finding(), 9)
	r.Equal(variablePage{namespace: "production", path: "nomad/jobs/web"}.path, m.screen.page.(variablePage).path)

	m = openMatch(t, finding(), 10)
	volume, ok := m.screen.page.(volumePage)
	r.True(ok, "%T", m.screen.page)
	r.Equal(nomad.VolumeHost, volume.kind)
	r.Equal("7f3c1a2b-0000-0000-0000-000000000000", volume.id)

	m = openMatch(t, finding(), 11)
	r.Equal("web-csi", m.screen.page.(pluginPage).id)
}

func TestFind_AnEvalIsDescribed(t *testing.T) {
	r := require.New(t)

	client := finding()
	m := openMatch(t, client, 12)

	r.IsType(describePage{}, m.screen.page)
	r.Equal("bb1c0000-0000-0000-0000-000000000000", client.askedID)
}

func TestFind_ADeploymentIsReadFirst(t *testing.T) {
	r := require.New(t)

	client := finding()
	client.deployment = nomad.DeploymentDetail{Deployment: nomad.Deployment{
		ID: "bb1d0000-0000-0000-0000-000000000000", JobID: "web", Namespace: "production", Status: "running",
	}}

	m := openMatch(t, client, 13)

	deployment, ok := m.screen.page.(deploymentPage)
	r.True(ok, "%T", m.screen.page)
	r.Equal("production", deployment.namespace)
	r.Equal("web", deployment.jobID)
}

func TestFind_AFailureIsShownOnce(t *testing.T) {
	r := require.New(t)

	client := finding()
	client.err = errors.New("fuzzy search is disabled")

	m := typeCommand(newTestModel(client), "find web")
	r.Contains(plain(statusLine(m)), "fuzzy search is disabled")

	var cmd tea.Cmd

	m, cmd = m.update(pollMsg{})
	playOut(m, cmd)

	r.Equal(1, client.findCalls)
}
