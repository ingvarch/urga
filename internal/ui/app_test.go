package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// fakeClient answers what the test puts in it and keeps what it was asked.
type fakeClient struct {
	jobs        []nomad.Job
	allocs      []nomad.Alloc
	deployments []nomad.Deployment
	namespaces  []nomad.Namespace
	services    []nomad.Service
	evaluations []nomad.Evaluation
	nodes       []nomad.Node
	variables   []nomad.Variable
	nodePools   []nomad.NodePool

	err error

	askedNamespace string
	askedJobID     string

	calls      int
	allocCalls int
}

func (f *fakeClient) Address() string { return "https://nmd.1ly.dev" }

func (f *fakeClient) Version(context.Context) (string, error) { return "1.11.1", nil }

func (f *fakeClient) Jobs(_ context.Context, namespace string) ([]nomad.Job, error) {
	f.askedNamespace = namespace
	f.calls++

	return f.jobs, f.err
}

func (f *fakeClient) Allocations(_ context.Context, namespace, jobID string) ([]nomad.Alloc, error) {
	f.askedNamespace, f.askedJobID = namespace, jobID
	f.allocCalls++

	return f.allocs, f.err
}

func (f *fakeClient) Deployments(_ context.Context, namespace string) ([]nomad.Deployment, error) {
	f.askedNamespace = namespace

	return f.deployments, f.err
}

func (f *fakeClient) Namespaces(context.Context) ([]nomad.Namespace, error) {
	return f.namespaces, f.err
}

func (f *fakeClient) Services(_ context.Context, namespace string) ([]nomad.Service, error) {
	f.askedNamespace = namespace

	return f.services, f.err
}

func (f *fakeClient) Evaluations(_ context.Context, namespace string) ([]nomad.Evaluation, error) {
	f.askedNamespace = namespace

	return f.evaluations, f.err
}

func (f *fakeClient) Nodes(context.Context) ([]nomad.Node, error) {
	return f.nodes, f.err
}

func (f *fakeClient) Variables(_ context.Context, namespace string) ([]nomad.Variable, error) {
	f.askedNamespace = namespace

	return f.variables, f.err
}

func (f *fakeClient) NodePools(context.Context) ([]nomad.NodePool, error) {
	return f.nodePools, f.err
}

func twoJobs() []nomad.Job {
	return []nomad.Job{
		{ID: "web", Name: "web", Namespace: "production", Type: "service", Status: "running", Running: 3, Desired: 3},
		{ID: "cron", Name: "cron", Namespace: "production", Type: "batch", Status: "dead"},
	}
}

func newTestModel(client Client) Model {
	m := New(client, Options{Namespace: "production", Version: "v-test", PollEvery: time.Millisecond})
	m, _ = m.update(tea.WindowSizeMsg{Width: 120, Height: 30})

	return m
}

func TestFetchJobs_AsksInTheNamespace(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs()}

	msg := newTestModel(client).fetch()()

	jobs, ok := msg.(jobsMsg)
	r.True(ok, "%T", msg)
	r.Len(jobs, 2)

	// The namespace of the session travels with the request.
	r.Equal("production", client.askedNamespace)
}

func TestFetchJobs_HandsTheErrorOver(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{err: errors.New("connection refused")}

	msg := newTestModel(client).fetch()()

	fail, ok := msg.(errMsg)
	r.True(ok, "%T", msg)
	r.ErrorContains(fail.err, "connection refused")
}

func TestModel_ShowsTheJobsItGets(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{})
	m, _ = m.update(jobsMsg(twoJobs()))

	out := plain(m.render())

	r.Contains(out, "Jobs (production) [2]")
	r.Contains(out, "web")
	r.Contains(out, "cron")
	r.Contains(out, "3/3")
}

func TestModel_KeepsTheRowsWhenTheClusterFails(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{})
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(errMsg{err: errors.New("connection refused")})

	out := plain(m.render())

	// The last good list stays on the screen with the failure under it. An
	// empty table reads as an empty cluster.
	r.Contains(out, "connection refused")
	r.Contains(out, "web")
}

func TestModel_ErrorGoesAwayOnTheNextAnswer(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{})
	m, _ = m.update(errMsg{err: errors.New("connection refused")})
	m, _ = m.update(jobsMsg(twoJobs()))

	r.NotContains(plain(m.render()), "connection refused")
}

func TestModel_QuitKeys(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{
		{Code: 'q', Text: "q"},
		{Code: 'c', Mod: tea.ModCtrl},
	} {
		t.Run(key.String(), func(t *testing.T) {
			r := require.New(t)

			_, cmd := newTestModel(&fakeClient{}).update(key)
			r.NotNil(cmd)
			r.IsType(tea.QuitMsg{}, cmd())
		})
	}
}

func TestModel_FillsTheScreen(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{})
	m, _ = m.update(jobsMsg(twoJobs()))

	rows := lines(m.render())

	// The screen is exactly the size of the terminal: every line as wide as
	// the window and as many lines as it is tall.
	r.Len(rows, 30)
	for i, row := range rows {
		r.LessOrEqual(ansi.StringWidth(row), 120, "line %d: %q", i, row)
	}
}

func TestModel_PollsTheCluster(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs()}
	m := newTestModel(client)

	_, cmd := m.update(pollMsg{})
	r.NotNil(cmd)

	// A poll asks the cluster and comes back as a message. Nothing writes to
	// the model from a goroutine.
	r.IsType(jobsMsg{}, cmd())
	r.Equal(1, client.calls)
}

func TestModel_SchedulesTheNextPollOnAnswer(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{})

	for _, answer := range []tea.Msg{jobsMsg(twoJobs()), errMsg{err: errors.New("boom")}} {
		_, cmd := m.update(answer)
		r.NotNil(cmd)

		// The next ask is scheduled when the last one is answered, so two
		// polls never run over each other.
		r.IsType(pollMsg{}, cmd())
	}
}

func TestModel_UsesTheAlternateScreen(t *testing.T) {
	r := require.New(t)

	// urga takes the whole terminal and gives it back untouched when it ends,
	// it does not scribble over what the user had in the scrollback.
	r.True(newTestModel(&fakeClient{}).View().AltScreen)
}

func TestModel_LeavesAMarginAroundTheScreen(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{})
	m, _ = m.update(jobsMsg(twoJobs()))

	rows := lines(m.render())

	// A line of air above everything, and nothing touches the left edge.
	r.Empty(strings.TrimSpace(rows[0]))
	r.True(strings.HasPrefix(rows[1], strings.Repeat(" ", headerPadX)+"Address:"), rows[1])

	box := rows[screenPadTop+headerHeight]
	r.True(strings.HasPrefix(box, strings.Repeat(" ", screenPadX)+"╭"), box)

	// The box keeps the same margin on the right.
	r.Equal(120-screenPadX, ansi.StringWidth(box))

	// The status line follows the header, not the box.
	last := rows[len(rows)-1]
	r.True(strings.HasPrefix(last, strings.Repeat(" ", headerPadX)+"q"), last)
}
