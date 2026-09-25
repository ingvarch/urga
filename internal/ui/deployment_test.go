package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// waitingCanary is a deployment of web whose canary waits to be promoted,
// while api is done.
func waitingCanary() nomad.DeploymentDetail {
	return nomad.DeploymentDetail{
		Deployment: nomad.Deployment{
			ID: "5d1a2b3c-0000-0000-0000-000000000000", JobID: "web", Namespace: "production", JobVersion: 7,
			Status: "running", StatusDescription: "Deployment is running but requires manual promotion",
		},
		Groups: []nomad.DeploymentGroup{
			{Name: "api", DesiredTotal: 2, Placed: 2, Healthy: 2, ProgressDeadline: 10 * time.Minute},
			{
				Name: "web", DesiredTotal: 3, Placed: 1, DesiredCanaries: 1, PlacedCanaries: 1, AutoRevert: true,
				ProgressDeadline: 10 * time.Minute, RequireProgressBy: time.Now().Add(4*time.Minute + 30*time.Second),
			},
		},
	}
}

// canaryAllocs are the allocations of waitingCanary.
func canaryAllocs() []nomad.Alloc {
	return []nomad.Alloc{
		{ID: "9a1b2c3d-0000-0000-0000-000000000000", Namespace: "production", JobID: "web", TaskGroup: "web",
			NodeName: "node-01", Status: "running", Canary: true, Health: "checking"},
		{ID: "4f2a1c9e-0000-0000-0000-000000000000", Namespace: "production", JobID: "web", TaskGroup: "api",
			NodeName: "node-01", Status: "running", Health: "healthy"},
	}
}

func TestDeploymentPanel(t *testing.T) {
	r := require.New(t)

	// What the deployment is, then how far it got with each group.
	r.Equal([]string{
		" Status running   Job web   Version 7",
		" Deployment is running but requires manual promotion",
		"",
		" Group  Desired  Placed  Healthy  Unhealthy  Canaries  Promoted  Auto Revert  Deadline  Progress By",
		" api    2        2       2        0          -         -         no           10m       -",
		" web    3        1       0        0          1/1       no        yes          10m       in 4m",
		"",
	}, unstyled(t, deploymentPanel(waitingCanary(), 120, 100)))
}

func TestDeploymentPanel_ASystemJob(t *testing.T) {
	r := require.New(t)

	// A system job places one allocation on every client and has no
	// canaries.
	system := nomad.DeploymentDetail{
		Deployment: nomad.Deployment{ID: "d", JobID: "agent", Status: "successful", StatusDescription: "Deployment completed successfully"},
		Groups:     []nomad.DeploymentGroup{{Name: "agent", DesiredTotal: 12, Placed: 12, Healthy: 12}},
	}

	lines := unstyled(t, deploymentPanel(system, 120, 100))
	r.Contains(lines, " agent  12       12      12       0          -         -         no           -         -")
}

func TestDeploymentPanel_GroupsThatDoNotFit(t *testing.T) {
	r := require.New(t)

	many := waitingCanary()
	for _, name := range []string{"b1", "b2", "b3", "b4"} {
		many.Groups = append(many.Groups, nomad.DeploymentGroup{Name: name, DesiredTotal: 1})
	}

	// Room for the head, the table header and two groups: the rest are
	// counted, and the allocations under the panel keep their rows.
	lines := unstyled(t, deploymentPanel(many, 120, 8))
	r.Len(lines, 8)
	r.Equal("  + 4 more groups", lines[6])
}

// onDeployment is the deployment of web, opened from the list.
func onDeployment(t *testing.T, client *fakeClient) Model {
	t.Helper()

	client.deployments = []nomad.Deployment{waitingCanary().Deployment}
	client.deployment = waitingCanary()
	client.deploymentAllocs = canaryAllocs()

	m := newTestModel(client)
	m, _ = m.show(deploymentsView)
	m, _ = m.update(deploymentsMsg(client.deployments))

	m, cmd := m.update(enter())

	return drain(m, cmd)
}

func TestDeployments_EnterOpensTheDeployment(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onDeployment(t, client)

	// The deployment and its allocations, asked for in its namespace.
	r.IsType(deploymentPage{}, m.screen.page)
	r.Equal("5d1a2b3c-0000-0000-0000-000000000000", client.deploymentID)
	r.Equal("production", client.deploymentNamespace)

	out := plain(m.render())
	r.Contains(out, "Deployment 5d1a2b3c (Job: web) [2]")
	r.Contains(out, "requires manual promotion")

	rows := lines(out)
	panel, header := -1, -1

	for i, row := range rows {
		if strings.Contains(row, "Auto Revert") {
			panel = i
		}

		if strings.Contains(row, "Canary") && strings.Contains(row, "Health") {
			header = i
		}
	}

	// The groups above the allocations, as the panel of a client is above
	// its own.
	r.Positive(panel)
	r.Greater(header, panel)
	r.Contains(fileRow(t, m, 0), "yes")
	r.Contains(fileRow(t, m, 0), "checking")
}

func TestDeployment_ColumnsOfItsAllocations(t *testing.T) {
	r := require.New(t)

	m := onDeployment(t, &fakeClient{})

	r.Equal([]string{"ID", "TaskGroup", "Node", "Status", "Canary", "Health", "CPU", "MEM", "Age"}, m.screen.page.titles())
}

func TestDeployment_WatchesItsAllocationsAndItself(t *testing.T) {
	r := require.New(t)

	m := onDeployment(t, &fakeClient{})

	r.Equal([]string{nomad.TopicAllocation, nomad.TopicDeployment}, m.screen.page.topics())
}

func TestDeployment_TheKeysOfAllocations(t *testing.T) {
	r := require.New(t)

	m := onDeployment(t, &fakeClient{})

	m, _ = m.update(enter())

	r.IsType(tasksPage{}, m.screen.page)
	r.Contains(plain(m.render()), "Tasks (Allocation: 9a1b2c3d)")
}

func TestDeployment_AnAnswerForAnotherDeploymentIsDropped(t *testing.T) {
	r := require.New(t)

	m := onDeployment(t, &fakeClient{})

	other := waitingCanary()
	other.ID, other.StatusDescription = "other", "Deployment of another job"

	m, _ = m.update(deploymentMsg(other))

	// The deployment on the screen stays on it.
	out := plain(m.render())
	r.NotContains(out, "another job")
	r.Contains(out, "requires manual promotion")
}

func TestDeploymentPanel_ADeploymentThatIsOver(t *testing.T) {
	r := require.New(t)

	done := waitingCanary()
	done.Status, done.StatusDescription = "successful", "Deployment completed successfully"

	// The cluster keeps the deadline of the last step; nothing is due by it
	// any more.
	for _, line := range unstyled(t, deploymentPanel(done, 120, 100)) {
		r.NotContains(line, "in 4m")
	}
}

// pausedCron is a deployment of cron that someone paused.
func pausedCron() nomad.Deployment {
	return nomad.Deployment{
		ID: "7e8f9a0b-0000-0000-0000-000000000000", JobID: "cron", Namespace: "production", JobVersion: 3,
		Status: "paused", StatusDescription: "Deployment is paused",
	}
}

// onDeployments is the list of deployments opened by name: web waits for
// its canary, cron is paused.
func onDeployments(t *testing.T, client *fakeClient) Model {
	t.Helper()

	client.deployments = []nomad.Deployment{waitingCanary().Deployment, pausedCron()}

	m, cmd := runLine(newTestModel(client), "deployments")

	return drain(m, cmd)
}

func TestDeployments_TheList(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onDeployments(t, client)

	// Those of the namespace the session looks at.
	r.Equal("production", client.askedNamespace)
	r.Contains(plain(m.render()), "Deployments (production) [2]")
	r.Equal([]string{"ID", "JobID", "Namespace", "Version", "Status", "Description"}, m.screen.page.titles())
	r.Equal([]string{nomad.TopicDeployment}, m.screen.page.topics())

	r.Regexp(`^\s*5d1a2b3c\s+web\s+production\s+7\s+running\s+Deployment is running but requires manual promotion`, fileRow(t, m, 0))
	r.Regexp(`^\s*7e8f9a0b\s+cron\s+production\s+3\s+paused\s+Deployment is paused`, fileRow(t, m, 1))
}

func TestDeployments_TheKeysOfTheList(t *testing.T) {
	r := require.New(t)

	m := onDeployments(t, &fakeClient{})

	keys := func(m Model) []string {
		out := []string{}
		for _, b := range m.keys() {
			out = append(out, b.press+" "+b.label)
		}

		return out
	}

	// The label of the pause key names what it does to the deployment under
	// the cursor.
	r.Equal([]string{"enter Details", "d Describe", "p Promote", "f Fail", "ctrl+s Pause"}, keys(m))

	m, _ = m.update(down())
	r.Equal([]string{"enter Details", "d Describe", "p Promote", "f Fail", "ctrl+s Resume"}, keys(m))
}

func TestDeployments_DescribeTheOneUnderTheCursor(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{describe: "Deployment of cron, as the cluster describes it"}
	m := onDeployments(t, client)

	m, _ = m.update(down())
	m, cmd := m.update(key('d'))
	m = drain(m, cmd)

	r.IsType(describePage{}, m.screen.page)
	r.Equal("7e8f9a0b-0000-0000-0000-000000000000", client.askedID)
	r.Equal("production", client.askedNamespace)

	out := plain(m.render())
	r.Contains(out, "Deployment: 7e8f9a0b")
	r.Contains(out, "as the cluster describes it")
}

func TestDeployments_FollowTheSession(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onDeployments(t, client)
	m, _ = m.update(namespacesMsg(twoNamespaces()))

	m, cmd := m.update(key('2'))
	m = drain(m, cmd)

	// Opened by name, they are those of the namespace the session looks at.
	r.Equal("staging", client.askedNamespace)
	r.Contains(plain(m.render()), "Deployments (staging) [2]")
}

func TestDeployments_BackFromADeploymentToTheRowItWasOpenedFrom(t *testing.T) {
	r := require.New(t)

	m := onDeployments(t, &fakeClient{deploymentAllocs: canaryAllocs()})

	m, _ = m.update(down())
	m, cmd := m.update(enter())
	m = drain(m, cmd)
	r.Contains(plain(m.render()), "Deployment 7e8f9a0b (Job: cron)")

	// A list of deployments that arrives meanwhile is ignored by this screen.
	m, _ = m.update(deploymentsMsg([]nomad.Deployment{{ID: "another", JobID: "batch"}}))
	r.Contains(plain(m.render()), "Deployment 7e8f9a0b (Job: cron)")

	m, _ = m.update(escape())

	// The list still shows the rows it had, with the cursor where it was.
	r.Contains(plain(m.render()), "Deployments (production) [2]")
	r.Equal("7e8f9a0b", cursorName(m))
}

func TestDeployment_StaysInItsNamespace(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{changes: newChanges()}
	m := onDeployment(t, client)
	m.namespaceOrder = []string{"production", "staging"}

	m, cmd := m.update(key('2'))
	m = playOut(m, cmd)

	// The deployment lives in production: it and what it placed are asked
	// for, and watched, there.
	r.Equal("staging", m.namespace)
	r.Contains(plain(m.render()), "Deployment 5d1a2b3c (Job: web) [2]")
	r.Equal("production", client.deploymentNamespace)
	r.Equal("production", client.watchedNamespace)
}

func TestDeployment_NoPanelNorKeysOfItsOwnUntilItIsRead(t *testing.T) {
	r := require.New(t)

	// The cluster answers with the allocations, not yet with the
	// deployment.
	client := &fakeClient{deploymentAllocs: canaryAllocs()}
	m := onDeployments(t, client)

	m, cmd := m.update(enter())
	m = drain(m, cmd)

	// The columns of the allocations come right under the title of the box.
	title, header := -1, -1

	for i, row := range lines(m.render()) {
		if strings.Contains(row, "Deployment 5d1a2b3c (Job: web) [2]") {
			title = i
		}

		if strings.Contains(row, "Canary") && strings.Contains(row, "Health") {
			header = i
		}
	}

	r.Positive(title)
	r.Equal(title+1, header)

	for _, press := range []string{"p", "ctrl-p", "f", "ctrl-s"} {
		r.False(offers(m, press), press)
	}

	r.True(offers(m, "r"))
}

func TestDeployment_ReadsWhatItsAllocationsTake(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{use: map[string]nomad.ResourceUse{
		"9a1b2c3d-0000-0000-0000-000000000000": {CPUPercent: 42, CPUTicksAllowed: 500, MemoryPercent: 17, MemoryMBAllowed: 256},
	}}
	m := onDeployment(t, client)

	m = drain(m, m.fetchUsage())

	// The usage of each is requested in its own namespace.
	r.Equal("production", client.usageNamespace)
	r.Contains(fileRow(t, m, 0), "42%")
	r.Contains(fileRow(t, m, 0), "17%")
}

func TestDeployment_IsAskedWhereItLives(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{deployment: waitingCanary(), deploymentAllocs: canaryAllocs(), changes: newChanges()}
	m := onDeployments(t, client)

	// The session looks at every namespace; the deployment lives in
	// production.
	m, cmd := m.update(key('0'))
	m = drain(m, cmd)

	m, cmd = m.update(enter())
	drain(m, cmd)

	r.Equal("production", client.deploymentNamespace)

	m.update(m.watchScreen()())
	r.Equal("production", client.watchedNamespace)
}
