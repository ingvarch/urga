package ui

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// deployFaultsIDs are the IDs of the deployments the page keeps.
func deployFaultsIDs(p deploymentsPage) []string {
	ids := []string{}
	for _, d := range p.visible(env{}) {
		ids = append(ids, d.ID)
	}

	return ids
}

func TestDeploymentFaults_FailedOnlyWhenNewestOfItsJob(t *testing.T) {
	deployments := []nomad.Deployment{
		{ID: "web-3", JobID: "web", Namespace: "production", JobVersion: 3, Status: "failed"},
		{ID: "web-4", JobID: "web", Namespace: "production", JobVersion: 4, Status: "successful"},
		{ID: "api-2", JobID: "api", Namespace: "production", JobVersion: 2, Status: "failed"},
		{ID: "api-1", JobID: "api", Namespace: "production", JobVersion: 1, Status: "successful"},
		{ID: "web-s", JobID: "web", Namespace: "staging", JobVersion: 1, Status: "failed"},
	}

	got := deployFaultsIDs(deploymentsPage{deployments: deployments, fault: faultFailed})

	// The same job ID in another namespace is another job.
	require.Equal(t, []string{"api-2", "web-s"}, got)
}

func TestDeploymentFaults_PausedAndRunning(t *testing.T) {
	deployments := []nomad.Deployment{
		{ID: "a", Status: "successful"},
		{ID: "b", Status: "paused"},
		{ID: "c", Status: "running"},
		{ID: "d", Status: "failed"},
	}

	r := require.New(t)

	r.Equal([]string{"b"}, deployFaultsIDs(deploymentsPage{deployments: deployments, fault: faultPaused}))
	r.Equal([]string{"c"}, deployFaultsIDs(deploymentsPage{deployments: deployments, fault: faultRunning}))
	r.Equal([]string{"a", "b", "c", "d"}, deployFaultsIDs(deploymentsPage{deployments: deployments}))
}

func TestDeploymentFaults_KeysActOnTheOneUnderTheCursor(t *testing.T) {
	r := require.New(t)

	deployments := []nomad.Deployment{
		{ID: "dep-1", JobID: "web", Namespace: "production", Status: "successful"},
		{ID: "dep-2", JobID: "cron", Namespace: "production", Status: "paused"},
	}
	client := &fakeClient{deployments: deployments}

	m := newTestModel(client)
	m, _ = m.update(openMsg{deploymentsPage{deployments: deployments, fault: faultPaused}})

	r.True(offersLabel(m, "ctrl-s", "Resume"))

	asked, _ := m.update(ctrl('s'))
	r.Contains(plain(asked.render()), "Really resume the deployment of cron?")

	opened, cmd := m.update(enter())
	opened = drain(opened, cmd)

	r.Contains(opened.title(), "Deployment dep-2 (Job: cron)")
}

func TestDeploymentFaults_TitleSaysWhatTheListIsNarrowedTo(t *testing.T) {
	r := require.New(t)

	deployments := []nomad.Deployment{
		{ID: "a", JobID: "web", Namespace: "production", Status: "successful"},
		{ID: "b", JobID: "api", Namespace: "production", Status: "failed"},
	}

	m := newTestModel(&fakeClient{})
	m, _ = m.update(openMsg{deploymentsPage{deployments: deployments, fault: faultFailed}})

	r.Equal("Deployments (production, failed) [1]", m.title())
}
