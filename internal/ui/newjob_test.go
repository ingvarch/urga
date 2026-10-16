package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// newJobModel is the job list of a session in namespace, with an editor that
// saves edited in place of the template.
func newJobModel(client *fakeClient, namespace, edited string) (Model, *fakeEditor) {
	editor := &fakeEditor{replace: edited}

	m := New(client, Options{Namespace: namespace, Version: "v-test", Editor: editor, PollEvery: time.Millisecond})
	m, _ = m.update(sizeMsg())
	m, _ = m.update(jobsMsg(twoJobs()))

	return m, editor
}

// editedTemplate is the template with something typed into it: a file saved
// as it was sent nothing.
var editedTemplate = strings.Replace(nomad.JobTemplate, "nginx:1.27", "nginx:1.28", 1)

// plannedNew presses n, saves the edited template and returns the plan that opens.
func plannedNew(t *testing.T, client *fakeClient, namespace string) Model {
	t.Helper()

	m, _ := newJobModel(client, namespace, editedTemplate)

	m, cmd := m.update(key('n'))

	return follow(m, cmd, 6)
}

func TestNewJob_OpensTheTemplateAndPlansWhatIsSaved(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), plan: nomad.Plan{Groups: []nomad.PlanGroup{{Name: "web", Place: 1}}},
		target: nomad.JobTarget{ID: "web", Namespace: "production", Type: "service"}}
	m, editor := newJobModel(client, "production", editedTemplate)

	m, cmd := m.update(key('n'))
	m = follow(m, cmd, 6)

	r.Equal([]string{nomad.JobTemplate}, editor.seen)
	r.IsType(planPage{}, m.screen.page)
	r.Equal(editedTemplate, client.plannedSource)
	r.Equal("production", client.targetAskedIn)
	r.Equal("production", client.askedNamespace)
	r.Zero(client.submitted)
	r.Contains(plain(m.render()), "Plan (Job: web)")

	// A job that does not exist yet is submitted at index 0.
	m, cmd = m.update(key('y'))
	drain(m, cmd)

	r.Equal(1, client.submitted)
	r.Equal(editedTemplate, client.submittedSource)
	r.Equal("production", client.askedNamespace)
	r.Zero(client.submittedIndex)
}

func TestNewJob_GoesToTheNamespaceTheFileNames(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), plan: nomad.Plan{Index: 0},
		target: nomad.JobTarget{ID: "web", Namespace: "staging", Type: "service"}}
	m := plannedNew(t, client, "production")

	r.Equal("staging", client.askedNamespace)

	m, cmd := m.update(key('y'))
	drain(m, cmd)

	r.Equal("staging", client.askedNamespace)
	r.Equal(1, client.submitted)

	// A new job has no counts to keep.
	r.Nil(client.openedWith)
	r.Nil(client.submittedOpened)
}

func TestNewJob_FromAllNamespacesGoesToTheOneTheClientDecided(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), plan: nomad.Plan{Index: 0},
		target: nomad.JobTarget{ID: "web", Namespace: nomad.DefaultNamespace, Type: "service"}}
	m := plannedNew(t, client, "")

	r.Equal("", client.targetAskedIn)
	r.Equal(nomad.DefaultNamespace, client.askedNamespace)

	m, cmd := m.update(key('y'))
	drain(m, cmd)

	r.Equal(nomad.DefaultNamespace, client.askedNamespace)
}

func TestNewJob_ThatExistsIsNotSent(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), plan: changedEnv(), target: nomad.JobTarget{ID: "web", Namespace: "production", Type: "service"}}
	m := plannedNew(t, client, "production")

	out := plain(m.render())
	r.Contains(out, "A job named web exists already: a new one cannot replace it.")
	r.NotContains(out, "Changed after the file was read")

	line, _ := buttons(m)
	r.Contains(line, "Can't submit: job exists")
	r.False(offers(m, "y"))

	m, cmd := m.update(key('y'))
	m = drain(m, cmd)
	r.IsType(planPage{}, m.screen.page)

	// The button that sends cannot be chosen, and enter cancels.
	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyTab})
	r.Contains(m.render(), opening(styleButtonOn)+" Cancel ")

	m, cmd = m.update(enter())
	m = drain(m, cmd)

	r.Zero(client.submitted)
}

func TestNewJob_AReplanKeepsWhereTheFileWasPlanned(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), plan: nomad.Plan{Index: 0}, target: nomad.JobTarget{ID: "web", Namespace: "staging", Type: "service"}}
	m := plannedNew(t, client, "production")

	m, cmd := m.update(key('r'))
	m = follow(m, cmd, 3)

	r.Equal(2, client.planCalls)
	r.Equal("staging", client.askedNamespace)

	m, cmd = m.update(key('y'))
	drain(m, cmd)
	r.Zero(client.submittedIndex)
}

func TestNewJob_ReadOnlyOffersNoNew(t *testing.T) {
	r := require.New(t)

	m, editor := newJobModel(&fakeClient{jobs: twoJobs()}, "production", editedTemplate)
	r.True(offers(m, "n"))

	m.opts.ReadOnly = true
	r.False(offers(m, "n"))

	m, cmd := m.update(key('n'))
	m = drain(m, cmd)

	r.Contains(plain(m.render()), "read-only: New is off")
	r.Empty(editor.seen)
}

func TestNewJob_AFileTheClusterDoesNotAcceptOpensAgain(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), refusals: []error{errors.New("the job was not accepted")}}
	m, editor := newJobModel(client, "production", editedTemplate)
	editor.edits = []string{editedTemplate}

	m, cmd := m.update(key('n'))
	m = follow(m, cmd, 8)

	// The edit is not lost: the editor opens on it again with the reason.
	r.IsType(jobsPage{}, m.screen.page)
	r.Len(editor.seen, 2)
	r.Contains(editor.seen[1], "Not saved:")
	r.Contains(editor.seen[1], editedTemplate)
}

func TestNewJob_APlanTheClusterRefusesOpensTheEditorAgain(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), target: nomad.JobTarget{ID: "web", Namespace: "staging", Type: "service"},
		refusals: []error{nil, errors.New("namespace staging does not exist")}}
	m, editor := newJobModel(client, "production", editedTemplate)
	editor.edits = []string{editedTemplate}

	m, cmd := m.update(key('n'))
	m = follow(m, cmd, 8)

	r.IsType(jobsPage{}, m.screen.page)
	r.Len(editor.seen, 2)
	r.Contains(editor.seen[1], "Not saved: namespace staging does not exist.")
	r.Contains(editor.seen[1], editedTemplate)
}

func TestNewJob_TheWatchOpensForAJobThatRunsItself(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), plan: nomad.Plan{Index: 0}, target: nomad.JobTarget{ID: "web", Namespace: "staging", Type: "batch"}}
	m := plannedNew(t, client, "production")

	m, cmd := m.update(key('y'))
	m = drain(m, cmd)

	r.Equal(deployWatchPage{namespace: "staging", jobID: "web", jobType: "batch"}, m.screen.page)
}

func TestNewJob_SubmitsAtIndexZeroWhateverThePlanIndexIs(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := New(client, Options{Version: "v-test"})

	_, out := submitPlan(planPage{planState: planState{fresh: true, index: 7, jobID: "web"}}, m.env())
	out.cmd()

	r.Equal(1, client.submitted)
	r.Zero(client.submittedIndex)
}

func TestNewJob_ThatLaunchesOthersOpensNoWatch(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), plan: nomad.Plan{Index: 0},
		target: nomad.JobTarget{ID: "cron", Namespace: "production", Type: "batch", Launches: true}}
	m := plannedNew(t, client, "production")

	m, cmd := m.update(key('y'))
	m = drain(m, cmd)

	r.IsType(jobsPage{}, m.screen.page)
	r.Contains(plain(m.render()), "Job cron submitted")
}
