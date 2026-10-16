package ui

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// commandLine types a command line, runs it and follows what it asks for.
func commandLine(m Model, line string) Model {
	m, _ = m.update(key(':'))
	m = typeIn(m, line)
	m, cmd := m.update(enter())

	return follow(m, cmd, 4)
}

// runModel is the job list of a session in production, with a client that
// plans a new job called web.
func runModel(readOnly bool) (Model, *fakeClient) {
	client := &fakeClient{jobs: twoJobs(), plan: nomad.Plan{Index: 0},
		target: nomad.JobTarget{ID: "web", Namespace: "production", Type: "service"}}
	m, _ := newJobModel(client, "production", "")
	m.opts.ReadOnly = readOnly

	return m, client
}

func TestRun_PlansTheFileAndSubmitsItAtIndexZero(t *testing.T) {
	r := require.New(t)

	dir := t.TempDir()
	t.Chdir(dir)
	r.NoError(os.WriteFile("web.nomad.hcl", []byte(nomad.JobTemplate), 0o600))

	m, client := runModel(false)
	m = commandLine(m, "run web.nomad.hcl")

	r.IsType(planPage{}, m.screen.page)
	r.Equal(nomad.JobTemplate, client.plannedSource)
	r.Equal("production", client.askedNamespace)
	r.Equal("production", client.targetAskedIn)

	m, cmd := m.update(key('y'))
	drain(m, cmd)

	r.Equal(1, client.submitted)
	r.Equal(nomad.JobTemplate, client.submittedSource)
	r.Zero(client.submittedIndex)
}

func TestRun_ThatExistsIsNotSent(t *testing.T) {
	r := require.New(t)

	t.Chdir(t.TempDir())
	r.NoError(os.WriteFile("web.nomad.hcl", []byte(nomad.JobTemplate), 0o600))

	m, client := runModel(false)
	client.plan = nomad.Plan{Index: 7}
	m = commandLine(m, "run web.nomad.hcl")

	r.IsType(planPage{}, m.screen.page)
	r.Contains(plain(m.render()), "A job named web exists already: a new one cannot replace it.")
	r.False(offers(m, "y"))
}

func TestRun_TheFileGoesToTheNamespaceItNames(t *testing.T) {
	r := require.New(t)

	t.Chdir(t.TempDir())
	r.NoError(os.WriteFile("web.nomad.hcl", []byte(nomad.JobTemplate), 0o600))

	m, client := runModel(false)
	client.target.Namespace = "staging"
	commandLine(m, "run web.nomad.hcl")

	r.Equal("staging", client.askedNamespace)
}

func TestRun_ExpandsTheHomeDirectory(t *testing.T) {
	r := require.New(t)

	home := t.TempDir()
	// The home directory, where each system reads it from: Windows from
	// USERPROFILE, the others from HOME.
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	r.NoError(os.WriteFile(filepath.Join(home, "web.nomad.hcl"), []byte(nomad.JobTemplate), 0o600))

	m, client := runModel(false)
	m = commandLine(m, "run ~/web.nomad.hcl")

	r.IsType(planPage{}, m.screen.page)
	r.Equal(nomad.JobTemplate, client.plannedSource)
}

func TestRun_SaysWhyItPlansNothing(t *testing.T) {
	t.Run("a file that is not there", func(t *testing.T) {
		r := require.New(t)

		t.Chdir(t.TempDir())

		m, client := runModel(false)
		m = commandLine(m, "run web.nomad.hcl")

		r.IsType(jobsPage{}, m.screen.page)
		r.Contains(plain(m.render()), "cannot read web.nomad.hcl: "+syscall.ENOENT.Error())
		r.Zero(client.planCalls)
	})

	t.Run("no home directory", func(t *testing.T) {
		r := require.New(t)

		t.Setenv("HOME", "")
		t.Setenv("USERPROFILE", "")

		m, client := runModel(false)
		m = commandLine(m, "run ~/web.nomad.hcl")

		r.IsType(jobsPage{}, m.screen.page)
		r.Contains(plain(m.render()), "cannot read ~/web.nomad.hcl:")
		r.Zero(client.planCalls)
	})

	t.Run("no path", func(t *testing.T) {
		r := require.New(t)

		m, client := runModel(false)
		m = commandLine(m, "run")

		r.IsType(jobsPage{}, m.screen.page)
		r.Contains(plain(m.render()), ":run needs a job file")
		r.Zero(client.planCalls)
	})

	t.Run("read-only mode", func(t *testing.T) {
		r := require.New(t)

		t.Chdir(t.TempDir())
		r.NoError(os.WriteFile("web.nomad.hcl", []byte(nomad.JobTemplate), 0o600))

		m, client := runModel(true)
		m = commandLine(m, "run web.nomad.hcl")

		r.IsType(jobsPage{}, m.screen.page)
		r.Contains(plain(m.render()), "read-only: :run is off")
		r.Zero(client.planCalls)
		r.Empty(client.plannedSource)
	})
}
