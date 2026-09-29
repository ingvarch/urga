package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// twoPolicies are the policies of two groups of web, one of them off.
func twoPolicies() []nomad.ScalingPolicy {
	return []nomad.ScalingPolicy{
		{ID: "pol-1", Namespace: "staging", Job: "web", Group: "frontend", Type: "horizontal", Enabled: true},
		{ID: "pol-2", Namespace: "staging", Job: "web", Group: "cache", Type: "horizontal"},
	}
}

// frontendScaling is a group the autoscaler keeps in 1-5: scaled by hand,
// after a report that failed.
func frontendScaling() nomad.GroupScaling {
	three := 3

	return nomad.GroupScaling{
		Count: 3, Running: 3, Healthy: 2,
		Policy: &nomad.GroupPolicy{ID: "pol-1", Enabled: true, Min: 1, Max: 5},
		Events: []nomad.ScalingEvent{
			{At: time.Now().Add(-5 * time.Minute), From: 2, To: &three, Message: "scaled up by hand"},
			{At: time.Now().Add(-2 * time.Hour), From: 2, Message: "no metrics from nomad-apm", Error: true},
		},
	}
}

// onScaling is the list of scaling policies, opened by name.
func onScaling(t *testing.T, client *fakeClient) Model {
	t.Helper()

	m, _ := sessionModel(t, client)

	return typeCommand(m, "scaling")
}

// historyOf is the scaling history of frontend, opened from the task groups
// of web.
func historyOf(t *testing.T, scaling nomad.GroupScaling) (Model, *fakeClient) {
	t.Helper()

	client := &fakeClient{jobs: twoJobs(), groups: twoGroups(), groupScaling: scaling}
	m := openTaskGroups(t, client)

	m, cmd := m.update(key('a'))

	return playOut(m, cmd), client
}

func TestScaling_ListsThePoliciesOfTheNamespace(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{scalingPolicies: twoPolicies()}
	m, cfg := sessionModel(t, client)
	m = typeCommand(m, "scaling")

	r.Equal("production", client.askedNamespace)
	r.Equal("Scaling Policies (production) [2]", m.title())
	r.Equal([]string{"Job", "Group", "Namespace", "Type", "Enabled"}, m.screen.page.titles())

	rows := m.screen.page.rows(m.env())
	r.Equal([]string{"web", "frontend", "staging", "horizontal", "yes"}, rows[0].cells)

	// A policy that is off is muted.
	r.Nil(rows[0].color)
	r.Equal(colorMuted, rows[1].color)

	// The next run opens it again. A policy changes with its job.
	r.Equal("scaling", cfg.Screen)
	r.Equal([]string{nomad.TopicJob}, m.screen.page.topics())
}

func TestScaling_EnterOpensTheHistoryOfTheGroup(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{scalingPolicies: twoPolicies(), groupScaling: frontendScaling(), changes: newChanges()}
	m := onScaling(t, client)

	m, cmd := m.update(enter())
	m = playOut(m, cmd)

	// Asked and watched where the policy lives, whatever the session looks
	// at. A scale writes a version of the job.
	r.Equal("staging", client.askedNamespace)
	r.Equal("web", client.askedJobID)
	r.Equal("frontend", client.askedGroup)
	r.Equal("staging", client.watchedNamespace)
	r.Equal([]string{nomad.TopicJob}, client.watchedTopics)
	r.Equal("Scaling (Job: web, Group: frontend, 3 in 1-5) [2]", m.title())
}

func TestScaling_DescribesThePolicy(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{scalingPolicies: twoPolicies(), describe: "{}"}
	m := onScaling(t, client)

	m, cmd := m.update(key('d'))
	m = playOut(m, cmd)

	r.IsType(describePage{}, m.screen.page)
	r.Equal("Scaling policy: web/frontend", m.title())
	r.Equal("staging", client.askedNamespace)
	r.Equal("pol-1", client.askedID)
}

func TestScaling_TaskGroupsOpenTheHistoryOfAGroup(t *testing.T) {
	r := require.New(t)

	m, client := historyOf(t, frontendScaling())

	r.IsType(scalingPage{}, m.screen.page)
	r.Equal("production", client.askedNamespace)
	r.Equal("web", client.askedJobID)
	r.Equal("frontend", client.askedGroup)
}

func TestScaling_TheHistoryNamesTheBoundsOfThePolicy(t *testing.T) {
	r := require.New(t)

	off, none := frontendScaling(), frontendScaling()
	off.Policy.Enabled = false
	none.Policy = nil

	for want, scaling := range map[string]nomad.GroupScaling{
		"Scaling (Job: web, Group: frontend, 3 in 1-5) [2]":      frontendScaling(),
		"Scaling (Job: web, Group: frontend, 3 in 1-5, off) [2]": off,
		"Scaling (Job: web, Group: frontend, 3) [2]":             none,
	} {
		m, _ := historyOf(t, scaling)
		r.Equal(want, m.title())
	}
}

func TestScaling_TheHistoryNamesNoCountBeforeTheClusterAnswers(t *testing.T) {
	// A count of 0 would read as a group that runs nothing.
	require.Equal(t, "Scaling (Job: web, Group: frontend) [0]", scalingPage{jobID: "web", group: "frontend"}.title(env{}, 0))
}

func TestScaling_TheHistoryListsWhatWasDoneNewestFirst(t *testing.T) {
	r := require.New(t)

	m, _ := historyOf(t, frontendScaling())

	r.Equal([]string{"Change", "Message", "Age"}, m.screen.page.titles())

	rows := m.screen.page.rows(m.env())
	r.Equal([]string{"2 → 3", "scaled up by hand", "5m"}, rows[0].cells)
	r.Nil(rows[0].color)

	// A report that left the count, and failed.
	r.Equal([]string{"-", "no metrics from nomad-apm", "2h"}, rows[1].cells)
	r.Equal(colorDead, rows[1].color)
}

func TestScaling_TheHistoryDescribesThePolicy(t *testing.T) {
	r := require.New(t)

	m, client := historyOf(t, frontendScaling())
	client.describe = "{}"
	r.Contains(m.hints(), hint{Key: "<d>", Description: "Describe"})

	m, cmd := m.update(key('d'))
	m = playOut(m, cmd)

	r.Equal("Scaling policy: web/frontend", m.title())
	r.Equal("production", client.askedNamespace)
	r.Equal("pol-1", client.askedID)

	// A group without a policy has none to describe.
	none := frontendScaling()
	none.Policy = nil

	m, _ = historyOf(t, none)
	r.NotContains(m.hints(), hint{Key: "<d>", Description: "Describe"})
}

// scaledGroups are the groups of web: frontend held in 1-5, backend without a
// policy, cache with its policy off.
func scaledGroups() []nomad.TaskGroup {
	groups := twoGroups()
	groups[0].Policy = &nomad.GroupPolicy{ID: "pol-1", Enabled: true, Min: 1, Max: 5}

	return append(groups, nomad.TaskGroup{Name: "cache", JobID: "web", Count: 2, Policy: &nomad.GroupPolicy{ID: "pol-2", Min: 2, Max: 4}})
}

func TestTaskGroups_ShowTheBoundsOfTheirPolicy(t *testing.T) {
	r := require.New(t)

	m := openTaskGroups(t, &fakeClient{jobs: twoJobs()})
	m, _ = m.update(taskGroupsMsg(scaledGroups()))

	titles := m.screen.page.titles()
	r.Equal("Scaling", titles[len(titles)-1])

	for i, want := range []string{"1-5", "-", "2-4 off"} {
		cells := m.screen.page.rows(m.env())[i].cells
		r.Equal(want, cells[len(cells)-1])
	}
}

func TestScale_WarnsWhenTheAutoscalerHoldsTheGroup(t *testing.T) {
	r := require.New(t)

	for i, want := range []string{
		"Really scale frontend of web from 3 to 4? The autoscaler keeps it in 1-5 and may change it back.",
		"Really scale backend of web from 1 to 4?",
		// A policy that is off does not move the count.
		"Really scale cache of web from 2 to 4?",
	} {
		m := openTaskGroups(t, &fakeClient{jobs: twoJobs()})
		m, _ = m.update(taskGroupsMsg(scaledGroups()))

		for range i {
			m, _ = m.update(key('j'))
		}

		m, _ = m.update(key('s'))
		m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyBackspace})
		m = typeIn(m, "4")
		m, _ = m.update(enter())

		r.Equal(want, m.confirm.question)
	}
}
