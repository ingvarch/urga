package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// changedEnv is the plan of a change of an environment variable of web: the
// task is replaced, and one group is short of memory.
func changedEnv() nomad.Plan {
	return nomad.Plan{
		Diff: []nomad.DiffLine{
			{Kind: nomad.DiffContext, Text: `group "web" {`},
			{Kind: nomad.DiffContext, Indent: 1, Text: `task "server" {`},
			{Kind: nomad.DiffDeleted, Indent: 3, Text: `V = "2"`},
			{Kind: nomad.DiffAdded, Indent: 3, Text: `V = "3"`},
		},
		Groups: []nomad.PlanGroup{
			{Name: "api", Ignore: 3},
			{Name: "web", Destructive: 2, Canary: 1, Ignore: 1},
		},
		Warnings: "1 warning:\n\n* Group \"web\" has warnings",
		Index:    42,
	}
}

// shortOfMemory is the same change on a cluster with no room for it.
func shortOfMemory() nomad.Plan {
	plan := changedEnv()
	plan.Failures = []nomad.PlacementFailure{
		{Group: "web", Unplaced: 1, NodesEvaluated: 1, NodesExhausted: 1, DimensionExhausted: map[string]int{"memory": 1}},
	}

	return plan
}

func TestPlanText(t *testing.T) {
	r := require.New(t)

	text := planText(shortOfMemory(), false)

	// What stops the submit comes first, then what the scheduler would do,
	// then the change itself.
	r.Equal([]string{
		"Placement failures",
		"",
		`Task group "web" failed to place 1 allocation:`,
		`  * Resources exhausted on 1 node`,
		`  * Dimension "memory" exhausted on 1 node`,
		"",
		"Warnings",
		"",
		"1 warning:",
		"",
		`* Group "web" has warnings`,
		"",
		"What will happen when you submit this job:",
		"",
		`  Task group "api"`,
		"    No changes - 3 allocations will remain unchanged",
		"",
		`  Task group "web"`,
		"    ● 2 allocations will be recreated (destructive update)",
		"    ● 1 canary allocation will be deployed",
		"    ● 1 allocation unchanged",
		"",
		"Changes",
		"",
		`  group "web" {`,
		`    task "server" {`,
		`-       V = "2"`,
		`+       V = "3"`,
	}, texts(text))

	// Each part in the colour of what it says.
	style := func(line string) string {
		for _, painted := range text {
			if painted.text == line && painted.style != nil {
				return opening(*painted.style)
			}
		}

		return ""
	}

	r.Equal(opening(styleError), style("Placement failures"))
	r.Equal(opening(styleWarn), style("Warnings"))
	r.Equal(opening(styleDestructive), style("    ● 2 allocations will be recreated (destructive update)"))
	r.Equal(opening(styleCanary), style("    ● 1 canary allocation will be deployed"))
	r.Equal(opening(styleMuted), style("    ● 1 allocation unchanged"))
	r.Equal(opening(styleMuted), style("    No changes - 3 allocations will remain unchanged"))
}

func TestPlanText_EveryUpdate(t *testing.T) {
	r := require.New(t)

	text := planText(nomad.Plan{Groups: []nomad.PlanGroup{
		{Name: "web", Place: 1, Stop: 2, Migrate: 1, InPlace: 3, Preemptions: 1},
	}}, false)

	r.Subset(texts(text), []string{
		"    ● 1 new allocation will be created",
		"    ● 2 allocations will be stopped",
		"    ● 1 allocation will be migrated to another node",
		"    ● 3 allocations will be updated in-place (no restart)",
		"    ● 1 allocation of another job will be preempted",
	})
}

func TestPlanText_NothingChanges(t *testing.T) {
	r := require.New(t)

	text := strings.Join(texts(planText(nomad.Plan{Groups: []nomad.PlanGroup{{Name: "web"}}}, false)), "\n")

	r.Contains(text, "No allocations affected")
	r.Contains(text, "Nothing in the job changes.")
	r.NotContains(text, "Placement failures")
	r.NotContains(text, "Warnings")

	// An empty plan: the scheduler returned nothing.
	text = strings.Join(texts(planText(nomad.Plan{}, false)), "\n")
	r.Contains(text, "No changes detected")
}

func TestPlanText_OfARevert(t *testing.T) {
	r := require.New(t)

	text := texts(planText(changedEnv(), true))

	r.Contains(text, "What will happen when you revert this job:")
}

// planned is the job list after web was edited and its plan came back.
func planned(t *testing.T, client *fakeClient) Model {
	t.Helper()

	m, editor := editModel(t, client)
	editor.replace = "job \"web\" {\n  type = \"batch\"\n}"

	m, cmd := m.update(key('e'))

	return follow(m, cmd, 6)
}

func TestPlan_ComesBeforeTheSubmit(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), spec: nomad.JobSource{Source: "job \"web\" {}"}, plan: changedEnv()}
	m := planned(t, client)

	// Saving the file requests a plan from the cluster and submits nothing
	// yet: a changed file can restart every allocation of the job.
	r.IsType(planPage{}, m.screen.page)
	r.Equal("job \"web\" {\n  type = \"batch\"\n}", client.plannedSource)
	r.Equal("production", client.askedNamespace)
	r.Zero(client.submitted)

	out := plain(m.render())
	r.Contains(out, "Plan (Job: web)")
	r.Contains(out, "2 allocations will be recreated")
}

func TestPlan_SubmitsAtItsIndex(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), spec: nomad.JobSource{Source: "job \"web\" {}"}, plan: changedEnv()}
	m := planned(t, client)

	m, cmd := m.update(key('y'))
	m = drain(m, cmd)

	// What was planned is what is sent, at the index it was planned at.
	r.Equal(1, client.submitted)
	r.Equal("job \"web\" {\n  type = \"batch\"\n}", client.submittedSource)
	r.Equal(uint64(42), client.submittedIndex)

	// The plan is spent: the screen goes back to the job list.
	r.IsType(jobsPage{}, m.screen.page)
	r.Contains(plain(m.render()), "Job web submitted")
}

func TestPlan_WhenTheJobChangedSinceThePlan(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		jobs:      twoJobs(),
		spec:      nomad.JobSource{Source: "job \"web\" {}"},
		plan:      changedEnv(),
		actionErr: nomad.ErrJobChanged,
	}
	m := planned(t, client)

	m, cmd := m.update(key('y'))
	m = drain(m, cmd)

	// The plan on the screen is not what submitting would do any more.
	// The file is still there to be planned again.
	r.IsType(planPage{}, m.screen.page)
	r.Contains(plain(m.render()), "web changed since this plan: r plans it again")
}

func TestPlan_PlansTheSameFileAgain(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), spec: nomad.JobSource{Source: "job \"web\" {}"}, plan: changedEnv()}
	m := planned(t, client)

	again := changedEnv()
	again.Groups = []nomad.PlanGroup{{Name: "web", InPlace: 3}}
	again.Index = 43
	client.plan = again

	m, cmd := m.update(key('r'))
	m = drain(m, cmd)

	r.Equal(2, client.planCalls)
	r.Equal("job \"web\" {\n  type = \"batch\"\n}", client.plannedSource)
	r.IsType(planPage{}, m.screen.page)
	r.Contains(strings.Join(m.text.lines, "\n"), "3 allocations will be updated in-place")

	// Submitted after a new plan, it is sent with the new index.
	m, cmd = m.update(key('y'))
	drain(m, cmd)
	r.Equal(uint64(43), client.submittedIndex)
}

func TestPlan_EscapeLeavesTheJobAsItIs(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), spec: nomad.JobSource{Source: "job \"web\" {}"}, plan: changedEnv()}
	m := planned(t, client)

	m, _ = m.update(escape())

	r.IsType(jobsPage{}, m.screen.page)
	r.Zero(client.submitted)
}

// buttons is the line of the plan with its buttons, and whether it is the
// last line inside the box.
func buttons(m Model) (line string, last bool) {
	rows := lines(plain(m.render()))

	for i, row := range rows {
		if strings.Contains(row, "Cancel") {
			return row, i+1 < len(rows) && strings.Contains(rows[i+1], "╰")
		}
	}

	return "", false
}

func TestPlan_AsksAtTheBottom(t *testing.T) {
	r := require.New(t)

	long := changedEnv()
	for i := 0; i < 100; i++ {
		long.Diff = append(long.Diff, nomad.DiffLine{Kind: nomad.DiffAdded, Indent: 3, Text: fmt.Sprintf("K%d = \"v\"", i)})
	}

	client := &fakeClient{jobs: twoJobs(), spec: nomad.JobSource{Source: "job \"web\" {}"}, plan: long}
	m := planned(t, client)

	// The question and its buttons stay at the foot of the plan, however
	// long it is and wherever it is scrolled to.
	line, last := buttons(m)
	r.True(last)
	r.Contains(line, "Submit web?")
	r.Contains(line, "Submit")

	m, _ = m.update(key('G'))
	_, last = buttons(m)
	r.True(last)
	r.Contains(plain(m.render()), "K99")

	// The cursor starts on cancel: enter out of habit sends nothing.
	r.Contains(m.render(), opening(styleButtonOn)+" Cancel ")
}

func TestPlan_EnterOnCancelLeavesTheJobAsItIs(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), spec: nomad.JobSource{Source: "job \"web\" {}"}, plan: changedEnv()}
	m := planned(t, client)

	// Over to submit and back.
	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyTab})
	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyLeft})

	m, cmd := m.update(enter())
	drain(m, cmd)

	r.IsType(jobsPage{}, m.screen.page)
	r.Zero(client.submitted)
}

func TestPlan_EnterOnSubmitSendsIt(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), spec: nomad.JobSource{Source: "job \"web\" {}"}, plan: changedEnv()}
	m := planned(t, client)

	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyRight})
	r.Contains(m.render(), opening(styleButtonOn)+" Submit ")

	m, cmd := m.update(enter())
	m = drain(m, cmd)

	r.Equal(1, client.submitted)
	r.Equal(uint64(42), client.submittedIndex)
	r.IsType(jobsPage{}, m.screen.page)
}

func TestPlan_WhatCannotBePlacedIsNotSent(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), spec: nomad.JobSource{Source: "job \"web\" {}"}, plan: shortOfMemory()}
	m := planned(t, client)

	// The cluster has no room for it: the button shows that, and neither
	// the button nor the key sends it.
	line, _ := buttons(m)
	r.Contains(line, "Can't submit: placement failed")
	r.False(offers(m, "y"))

	m, cmd := m.update(key('y'))
	m = drain(m, cmd)
	r.IsType(planPage{}, m.screen.page)

	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyRight})
	m, cmd = m.update(enter())
	drain(m, cmd)

	r.Zero(client.submitted)
}

func TestPlan_ARevertThatTheJobMovedPast(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), plan: nomad.Plan{To: 3, Version: 4}, actionErr: nomad.ErrJobChanged}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, cmd := m.update(key('u'))
	m = drain(m, cmd)

	m, cmd = m.update(key('y'))
	m = drain(m, cmd)

	r.IsType(planPage{}, m.screen.page)
	r.Contains(plain(m.render()), "web changed since this plan: r plans it again")

	// Planned again, it is the same revert: to the version it was asked for.
	client.plan = nomad.Plan{To: 3, Version: 5}

	m, cmd = m.update(key('r'))
	m = drain(m, cmd)

	r.NotNil(client.plannedTo)
	r.Equal(uint64(3), *client.plannedTo)

	client.actionErr = nil
	m, cmd = m.update(key('y'))
	drain(m, cmd)

	r.Equal(uint64(5), client.revertedFrom)
}

func TestPlan_ARevertSaysWhatItDoes(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), plan: nomad.Plan{To: 3, Version: 4}}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, cmd := m.update(key('u'))
	m = drain(m, cmd)

	// The question names what the key sends: a revert, not a submit.
	r.True(offers(m, "y"))

	out := plain(m.render())
	r.Contains(out, "Revert web to version 3?")
	r.NotContains(out, "Submit")
}

func TestPlan_TheHeaderSaysWhatAPlanCanDo(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), spec: nomad.JobSource{Source: "job \"web\" {}"}, plan: changedEnv()}
	m := planned(t, client)

	r.Equal([]hint{
		{Key: "<y>", Description: "Submit"},
		{Key: "<r>", Description: "Replan"},
		{Key: "<w>", Description: "Toggle Wrap"},
		{Key: "<ctrl-s>", Description: "Save"},
	}, m.hints())

	// With nothing to send, there is no key that sends.
	client.plan = shortOfMemory()
	m = planned(t, client)

	r.Equal([]hint{
		{Key: "<r>", Description: "Replan"},
		{Key: "<w>", Description: "Toggle Wrap"},
		{Key: "<ctrl-s>", Description: "Save"},
	}, m.hints())
}

func TestPlan_TheButtonsAreChosenTheWayTheButtonsOfAQuestionAre(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), spec: nomad.JobSource{Source: "job \"web\" {}"}, plan: changedEnv()}
	m := planned(t, client)

	cancel, submit := opening(styleButtonOn)+" Cancel ", opening(styleButtonOn)+" Submit "

	m, _ = m.update(key('l'))
	r.Contains(m.render(), submit)

	m, _ = m.update(key('h'))
	r.Contains(m.render(), cancel)

	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyTab})
	r.Contains(m.render(), submit)

	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	r.Contains(m.render(), cancel)

	// Those keys are the question's: help names them, the header does not.
	for _, h := range m.hints() {
		r.NotContains([]string{"<enter>", "<h>", "<l>", "<left>", "<right>", "<tab>", "<shift-tab>"}, h.Key)
	}
}

func TestPlan_ReadOnlySendsNothing(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), spec: nomad.JobSource{Source: "job \"web\" {}"}, plan: changedEnv()}
	m := planned(t, client)
	m.opts.ReadOnly = true

	m, cmd := m.update(key('y'))
	m = drain(m, cmd)
	r.Contains(plain(m.render()), "read-only: Submit is off")

	// The button that sends cannot be chosen, and enter cancels.
	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyRight})
	r.Contains(m.render(), opening(styleButtonOn)+" Cancel ")

	m, cmd = m.update(enter())
	m = drain(m, cmd)

	r.IsType(jobsPage{}, m.screen.page)
	r.Zero(client.submitted)
}

func TestPlan_PlannedAgainInPlaceOfTheOneBefore(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), spec: nomad.JobSource{Source: "job \"web\" {}"}, plan: changedEnv()}
	m := planned(t, client)
	m, _ = m.update(key('w'))
	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyTab})

	m, cmd := m.update(key('r'))
	m = drain(m, cmd)

	// Shown the way the one before was, with the same button chosen.
	r.True(m.text.wrap)
	r.Contains(m.render(), opening(styleButtonOn)+" Submit ")

	// Escape goes back to where the plan was asked from.
	m, _ = m.update(escape())
	r.IsType(jobsPage{}, m.screen.page)
	r.Empty(m.history)
}

func TestPlan_ASubmitThatFailsStaysOnThePlan(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		jobs:      twoJobs(),
		spec:      nomad.JobSource{Source: "job \"web\" {}"},
		plan:      changedEnv(),
		actionErr: errors.New("Unexpected response code: 500 (rpc error)"),
	}
	m := planned(t, client)

	m, cmd := m.update(key('y'))
	m = drain(m, cmd)

	r.IsType(planPage{}, m.screen.page)
	r.Contains(plain(m.render()), "rpc error")

	// Planned again, the error stays on screen until its timer ends.
	m, cmd = m.update(key('r'))
	m = drain(m, cmd)

	r.Equal(2, client.planCalls)
	r.Contains(plain(m.render()), "rpc error")
}

func TestPlan_WhatCameOfASubmitIsSaidAfterLeaving(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), spec: nomad.JobSource{Source: "job \"web\" {}"}, plan: changedEnv()}
	m := planned(t, client)

	// Sent, and the plan is left before the cluster answers.
	m, sent := m.update(key('y'))
	m, _ = m.update(escape())
	m = drain(m, sent)

	r.Equal(1, client.submitted)
	r.IsType(jobsPage{}, m.screen.page)
	r.Contains(plain(m.render()), "Job web submitted")
}

func TestPlan_DoesNotPoll(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), spec: nomad.JobSource{Source: "job \"web\" {}"}, plan: changedEnv()}
	m := planned(t, client)

	_, cmd := m.update(pollMsg{})
	r.Nil(cmd)
}

func TestPlan_Saved(t *testing.T) {
	r := require.New(t)

	dir := t.TempDir()
	t.Chdir(dir)

	client := &fakeClient{jobs: twoJobs(), spec: nomad.JobSource{Source: "job \"web\" {}"}, plan: changedEnv()}
	m := planned(t, client)

	m, cmd := m.update(ctrlKey('s'))
	drain(m, cmd)

	files, err := filepath.Glob(filepath.Join(dir, "urga-*.txt"))
	r.NoError(err)
	r.Len(files, 1)

	saved, err := os.ReadFile(files[0])
	r.NoError(err)
	r.Contains(string(saved), "What will happen when you submit this job:")
}

func TestPlan_ASwitchOfTheSessionLeavesThePlanAsItIs(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), spec: nomad.JobSource{Source: "job \"web\" {}"}, plan: changedEnv()}
	m := planned(t, client)
	m.namespaceOrder = []string{"production", "staging"}

	m, cmd := m.update(key('2'))
	m = playOut(m, cmd)

	r.Equal("staging", m.namespace)
	r.IsType(planPage{}, m.screen.page)
	r.Contains(plain(m.render()), "Plan (Job: web)")
	r.Zero(client.submitted)
}

func TestPlan_OfAnotherJobGoesOnTop(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), plan: nomad.Plan{To: 3, Version: 4}}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	// A revert of both jobs is requested before either plan arrives.
	m, web := m.update(key('u'))
	m, _ = m.update(key('j'))
	m, cron := m.update(key('u'))

	m = drain(m, web)
	m = drain(m, cron)
	r.Contains(plain(m.render()), "Revert (Job: cron, Version: 3)")

	// The plan of cron is not the plan of web planned again.
	m, _ = m.update(escape())
	r.Contains(plain(m.render()), "Revert (Job: web, Version: 3)")
}
