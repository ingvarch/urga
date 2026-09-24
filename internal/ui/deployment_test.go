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
	// counted, and the tasks keep their rows.
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
	m, _ = m.show(screenDeployments)
	m, _ = m.update(deploymentsMsg(client.deployments))

	m, cmd := m.update(enter())

	return drain(m, cmd)
}

func TestDeployments_EnterOpensTheDeployment(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onDeployment(t, client)

	// The deployment and its allocations, asked for in its namespace.
	r.Equal(screenAllocations, m.screen.kind)
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

	// The groups over the allocations, the way a client sits over its own.
	r.Positive(panel)
	r.Greater(header, panel)
	r.Contains(fileRow(t, m, 0), "yes")
	r.Contains(fileRow(t, m, 0), "checking")
}

func TestDeployment_ColumnsOfItsAllocations(t *testing.T) {
	r := require.New(t)

	m := onDeployment(t, &fakeClient{})

	r.Equal([]string{"ID", "TaskGroup", "Node", "Status", "Canary", "Health", "CPU", "MEM", "Age"}, m.screen.titles())
}

func TestDeployment_WatchesItsAllocationsAndItself(t *testing.T) {
	r := require.New(t)

	m := onDeployment(t, &fakeClient{})

	r.Equal([]string{nomad.TopicAllocation, nomad.TopicDeployment}, m.screen.topics())
}

func TestDeployment_TheKeysOfAllocations(t *testing.T) {
	r := require.New(t)

	m := onDeployment(t, &fakeClient{})

	m, _ = m.update(enter())

	r.Equal(screenTasks, m.screen.kind)
	r.Equal("9a1b2c3d-0000-0000-0000-000000000000", m.screen.allocID)
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
