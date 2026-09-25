package ui

import (
	"context"
	"errors"
	"fmt"
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
	alloc       nomad.Alloc
	nodeAllocs  []nomad.Alloc
	nodeDetail  nomad.NodeDetail
	nodeMeta    []nomad.MetaEntry
	versions    []nomad.JobVersion
	diff        []nomad.DiffLine
	groups      []nomad.TaskGroup
	deployments []nomad.Deployment
	namespaces  []nomad.Namespace
	services    []nomad.Service
	evaluations []nomad.Evaluation
	evaluation  nomad.EvaluationDetail

	placementErr error
	nodes        []nomad.Node
	variables    []nomad.Variable
	variable     nomad.VariableDetail
	variablePath string
	nodePools    []nomad.NodePool
	servers      []nomad.Server
	changes      *fakeChanges
	server       nomad.Server
	raft         []nomad.RaftPeer

	// variableSpec is a variable as a file.
	variableSpec nomad.VariableSource

	// refusals are what the sends of an edit answer, in turn: the plan of a
	// job, a namespace, metadata and a variable.
	refusals []error

	describe      string
	spec          nomad.JobSource
	specErr       error
	logs          *nomad.LogStream
	usage         nomad.Usage
	use           map[string]nomad.ResourceUse
	usageCalls    int
	usageErr      error
	namespaceSpec string

	submitted       int
	submittedSource string
	submittedVars   nomad.JobVariables
	submittedIndex  uint64

	plan                nomad.Plan
	planCalls           int
	plannedSource       string
	plannedVars         nomad.JobVariables
	plannedTo           *uint64
	revertedFrom        uint64
	submittedNamespaces int

	stopped       int
	started       int
	restarted     int
	stoppedAllocs int
	drained       bool
	drainCalls    int
	eligible      bool
	eligibleCalls int
	promoted      int
	failed        int
	scaled        int
	scaledTo      int
	actionErr     error

	askedID     string
	askedTask   string
	askedGroup  string
	askedSource string
	logsClosed  bool

	err     error
	raftErr error

	askedNamespace   string
	askedJobID       string
	askedNodeID      string
	usageNamespace   string
	askedServer      string
	metaSpec         string
	metaSubmitted    string
	askedVersion     uint64
	watchedTopics    []string
	watchedNamespace string
	watchErr         error
	revertedTo       uint64
	diffErr          error

	calls      int
	allocCalls int

	// writes are the calls that change the cluster, in the order made.
	writes []string

	region          string
	regions         []string
	datacenters     []string
	usageDatacenter string

	// checks are the checks of any allocation, and who asked for them last.
	checks          []nomad.Check
	checksNamespace string
	checksAllocID   string
	checksCalls     int

	// restartedTask and signalledTask are the tasks restarted and signalled
	// on their own, signal what the last one was sent.
	restartedTask string
	signalledTask string
	signal        string

	// files are the directories of any allocation by path, and which one
	// was listed last.
	files          map[string][]nomad.File
	filesNamespace string
	filesAllocID   string
	filesPath      string

	// file is what a file opened says, and which one was opened last.
	file          *nomad.LogStream
	fileErr       error
	fileNamespace string
	fileAllocID   string
	filePath      string
	fileCalls     int

	// deployment is any deployment read, with its allocations, and which
	// one was asked for last.
	deployment          nomad.DeploymentDetail
	deploymentAllocs    []nomad.Alloc
	deploymentNamespace string
	deploymentID        string

	// promotedGroups are the groups promoted on their own, paused whether
	// each pause asked to pause or to go on.
	promotedGroups []string
	paused         []bool

	// logsByAlloc are the logs of each allocation, and logsOpened the
	// allocations whose log was opened, in order.
	logsByAlloc map[string]*nomad.LogStream
	logsOpened  []string

	// token is the token the session sends, as the cluster sees it.
	token      nomad.Token
	tokenCalls int

	// instances are the instances of any service, and which service was
	// asked for last; deletedRegistration the last one deleted.
	instances           []nomad.ServiceInstance
	instancesNamespace  string
	instancesName       string
	deletedRegistration string
}

// wrote keeps a call that changes the cluster.
func (f *fakeClient) wrote(method string) { f.writes = append(f.writes, method) }

func (f *fakeClient) Address() string { return "https://nomad.example.com" }

func (f *fakeClient) Region() string { return f.region }

func (f *fakeClient) Regions(context.Context) ([]string, error) { return f.regions, f.err }

func (f *fakeClient) Datacenters(context.Context) ([]string, error) { return f.datacenters, f.err }

func (f *fakeClient) Agent(context.Context) (nomad.Agent, error) {
	return nomad.Agent{Version: "1.11.1"}, nil
}

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

func (f *fakeClient) AllocationChecks(_ context.Context, namespace, allocID string) ([]nomad.Check, error) {
	f.checksNamespace, f.checksAllocID = namespace, allocID
	f.checksCalls++

	return f.checks, f.err
}

func (f *fakeClient) Files(_ context.Context, namespace, allocID, path string) ([]nomad.File, error) {
	f.filesNamespace, f.filesAllocID, f.filesPath = namespace, allocID, path

	return f.files[path], f.err
}

func (f *fakeClient) File(_ context.Context, namespace, allocID, path string) (*nomad.LogStream, error) {
	f.fileNamespace, f.fileAllocID, f.filePath = namespace, allocID, path
	f.fileCalls++

	return f.file, f.fileErr
}

func (f *fakeClient) ServiceInstances(_ context.Context, namespace, name string) ([]nomad.ServiceInstance, error) {
	f.instancesNamespace, f.instancesName = namespace, name

	return f.instances, f.err
}

func (f *fakeClient) DeleteServiceRegistration(_ context.Context, namespace, name, id string) error {
	f.wrote("DeleteServiceRegistration")
	f.askedNamespace, f.instancesName, f.deletedRegistration = namespace, name, id

	return f.actionErr
}

func (f *fakeClient) Token(context.Context) (nomad.Token, error) {
	f.tokenCalls++

	return f.token, nil
}

func (f *fakeClient) Allocation(_ context.Context, namespace, allocID string) (nomad.Alloc, error) {
	f.askedNamespace, f.askedID = namespace, allocID

	return f.alloc, f.err
}

func (f *fakeClient) Evaluation(_ context.Context, namespace, evalID string) (nomad.EvaluationDetail, error) {
	f.askedNamespace, f.askedID = namespace, evalID

	return f.evaluation, f.err
}

func (f *fakeClient) FailedPlacement(_ context.Context, namespace, jobID string) (nomad.EvaluationDetail, error) {
	f.askedNamespace, f.askedJobID = namespace, jobID

	return f.evaluation, f.placementErr
}

func (f *fakeClient) NodeAllocations(_ context.Context, nodeID string) ([]nomad.Alloc, error) {
	f.askedNodeID = nodeID

	return f.nodeAllocs, f.err
}

func (f *fakeClient) JobVersions(_ context.Context, namespace, jobID string) ([]nomad.JobVersion, error) {
	f.askedNamespace, f.askedJobID = namespace, jobID

	return f.versions, f.err
}

func (f *fakeClient) JobVersionDiff(_ context.Context, namespace, jobID string, version uint64) ([]nomad.DiffLine, error) {
	f.askedNamespace, f.askedJobID, f.askedVersion = namespace, jobID, version

	return f.diff, f.diffErr
}

func (f *fakeClient) PlanRevert(_ context.Context, namespace, jobID string, to *uint64) (nomad.Plan, error) {
	f.askedNamespace, f.askedJobID, f.plannedTo = namespace, jobID, to
	f.planCalls++

	return f.plan, f.err
}

func (f *fakeClient) RevertJobTo(_ context.Context, _, jobID string, version, from uint64) error {
	f.wrote("RevertJobTo")
	f.askedJobID, f.revertedTo, f.revertedFrom = jobID, version, from

	return f.actionErr
}

func (f *fakeClient) Node(_ context.Context, nodeID string) (nomad.Node, error) {
	f.askedNodeID = nodeID

	for _, node := range f.nodes {
		if node.ID == nodeID {
			return node, f.err
		}
	}

	return nomad.Node{}, f.err
}

func (f *fakeClient) NodeDetail(_ context.Context, nodeID string) (nomad.NodeDetail, error) {
	f.askedNodeID = nodeID

	return f.nodeDetail, f.err
}

func (f *fakeClient) NodeMeta(_ context.Context, nodeID string) ([]nomad.MetaEntry, error) {
	f.askedNodeID = nodeID

	return f.nodeMeta, f.err
}

func (f *fakeClient) NodeMetaSpec(_ context.Context, nodeID string) (string, error) {
	f.askedNodeID = nodeID

	return f.metaSpec, f.err
}

func (f *fakeClient) SubmitNodeMeta(_ context.Context, nodeID, source string) error {
	f.wrote("SubmitNodeMeta")
	f.askedNodeID, f.metaSubmitted = nodeID, source

	return f.refused(f.err)
}

func (f *fakeClient) DescribeJob(_ context.Context, namespace, jobID string) (string, error) {
	f.askedNamespace, f.askedID = namespace, jobID

	return f.describe, f.err
}

func (f *fakeClient) DescribeAllocation(_ context.Context, namespace, allocID string) (string, error) {
	f.askedNamespace, f.askedID = namespace, allocID

	return f.describe, f.err
}

func (f *fakeClient) DescribeDeployment(_ context.Context, namespace, id string) (string, error) {
	f.askedNamespace, f.askedID = namespace, id

	return f.describe, f.err
}

func (f *fakeClient) DescribeService(_ context.Context, namespace, name string) (string, error) {
	f.askedNamespace, f.askedID = namespace, name

	return f.describe, f.err
}

func (f *fakeClient) JobSpec(_ context.Context, namespace, jobID string) (nomad.JobSource, error) {
	f.askedNamespace, f.askedID = namespace, jobID

	if f.specErr != nil {
		return nomad.JobSource{}, f.specErr
	}

	return f.spec, f.err
}

func (f *fakeClient) Logs(_ context.Context, namespace, allocID, task, source string) (*nomad.LogStream, error) {
	f.askedNamespace, f.askedID, f.askedTask, f.askedSource = namespace, allocID, task, source

	if f.err != nil {
		return nil, f.err
	}

	// Each allocation writes a log of its own, when the test gives it one.
	if f.logsByAlloc != nil {
		f.logsOpened = append(f.logsOpened, allocID)

		stream, ok := f.logsByAlloc[allocID]
		if !ok {
			return nil, fmt.Errorf("no log for %s", allocID)
		}

		return stream, nil
	}

	stream := f.logs
	stream.OnClose = func() { f.logsClosed = true }

	return stream, nil
}

func (f *fakeClient) StartJob(_ context.Context, namespace, jobID string) error {
	f.wrote("StartJob")
	f.askedNamespace, f.askedID = namespace, jobID
	f.started++

	return f.actionErr
}

func (f *fakeClient) StopJob(_ context.Context, namespace, jobID string) error {
	f.wrote("StopJob")
	f.askedNamespace, f.askedID = namespace, jobID
	f.stopped++

	return f.actionErr
}

func (f *fakeClient) ScaleJob(_ context.Context, namespace, jobID, group string, count int) error {
	f.wrote("ScaleJob")
	f.askedNamespace, f.askedID, f.askedGroup = namespace, jobID, group
	f.scaled++
	f.scaledTo = count

	return f.actionErr
}

func (f *fakeClient) RestartAllocation(_ context.Context, namespace, allocID string) error {
	f.wrote("RestartAllocation")
	f.askedNamespace, f.askedID = namespace, allocID
	f.restarted++

	return f.actionErr
}

func (f *fakeClient) RestartTask(_ context.Context, namespace, allocID, task string) error {
	f.wrote("RestartTask")
	f.askedNamespace, f.askedID = namespace, allocID
	f.restartedTask = task

	return f.actionErr
}

func (f *fakeClient) SignalTask(_ context.Context, namespace, allocID, task, signal string) error {
	f.wrote("SignalTask")
	f.askedNamespace, f.askedID = namespace, allocID
	f.signalledTask, f.signal = task, signal

	return f.actionErr
}

func (f *fakeClient) StopAllocation(_ context.Context, namespace, allocID string) error {
	f.wrote("StopAllocation")
	f.askedNamespace, f.askedID = namespace, allocID
	f.stoppedAllocs++

	return f.actionErr
}

func (f *fakeClient) TaskGroups(_ context.Context, namespace, jobID string) ([]nomad.TaskGroup, error) {
	f.askedNamespace, f.askedID = namespace, jobID

	return f.groups, f.err
}

func (f *fakeClient) PlanJob(_ context.Context, namespace, source string, vars nomad.JobVariables) (nomad.Plan, error) {
	f.askedNamespace, f.plannedSource, f.plannedVars = namespace, source, vars
	f.planCalls++

	return f.plan, f.refused(f.err)
}

func (f *fakeClient) SubmitJob(_ context.Context, namespace, source string, vars nomad.JobVariables, index uint64) error {
	f.wrote("SubmitJob")
	f.askedNamespace, f.submittedSource, f.submittedVars = namespace, source, vars
	f.submittedIndex = index
	f.submitted++

	return f.actionErr
}

func (f *fakeClient) NamespaceSpec(_ context.Context, name string) (string, error) {
	f.askedID = name

	return f.namespaceSpec, f.err
}

func (f *fakeClient) SubmitNamespace(_ context.Context, source string) error {
	f.wrote("SubmitNamespace")
	f.submittedSource = source
	f.submittedNamespaces++

	return f.refused(f.actionErr)
}

func (f *fakeClient) DrainNode(_ context.Context, nodeID string, drain bool) error {
	f.wrote("DrainNode")
	f.askedID, f.drained = nodeID, drain
	f.drainCalls++

	return f.actionErr
}

func (f *fakeClient) SetNodeEligible(_ context.Context, nodeID string, eligible bool) error {
	f.wrote("SetNodeEligible")
	f.askedID, f.eligible = nodeID, eligible
	f.eligibleCalls++

	return f.actionErr
}

func (f *fakeClient) PromoteDeployment(_ context.Context, namespace, deploymentID string) error {
	f.wrote("PromoteDeployment")
	f.askedNamespace, f.askedID = namespace, deploymentID
	f.promoted++

	return f.actionErr
}

func (f *fakeClient) PromoteGroups(_ context.Context, namespace, deploymentID string, groups []string) error {
	f.wrote("PromoteGroups")
	f.askedNamespace, f.askedID = namespace, deploymentID
	f.promotedGroups = append(f.promotedGroups, groups...)

	return f.actionErr
}

func (f *fakeClient) PauseDeployment(_ context.Context, namespace, deploymentID string, pause bool) error {
	f.wrote("PauseDeployment")
	f.askedNamespace, f.askedID = namespace, deploymentID
	f.paused = append(f.paused, pause)

	return f.actionErr
}

func (f *fakeClient) FailDeployment(_ context.Context, namespace, deploymentID string) error {
	f.wrote("FailDeployment")
	f.askedNamespace, f.askedID = namespace, deploymentID
	f.failed++

	return f.actionErr
}

func (f *fakeClient) Usage(_ context.Context, datacenter string) (nomad.Usage, error) {
	f.usageDatacenter = datacenter

	return f.usage, f.err
}

func (f *fakeClient) AllocationUsage(_ context.Context, namespace, allocID string) (nomad.ResourceUse, error) {
	f.usageCalls++
	f.usageNamespace = namespace

	if f.usageErr != nil {
		return nomad.ResourceUse{}, f.usageErr
	}

	use, ok := f.use[allocID]
	if !ok {
		return nomad.ResourceUse{}, errors.New("no stats")
	}

	return use, nil
}

func (f *fakeClient) NodeUsage(_ context.Context, nodeID string) (nomad.ResourceUse, error) {
	f.usageCalls++

	use, ok := f.use[nodeID]
	if !ok {
		return nomad.ResourceUse{}, errors.New("no stats")
	}

	return use, nil
}

func (f *fakeClient) Deployment(_ context.Context, namespace, id string) (nomad.DeploymentDetail, error) {
	f.deploymentNamespace, f.deploymentID = namespace, id

	return f.deployment, f.err
}

func (f *fakeClient) DeploymentAllocations(_ context.Context, namespace, id string) ([]nomad.Alloc, error) {
	f.deploymentNamespace, f.deploymentID = namespace, id

	return f.deploymentAllocs, f.err
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

func (f *fakeClient) Variable(_ context.Context, namespace, path string) (nomad.VariableDetail, error) {
	f.askedNamespace, f.variablePath = namespace, path

	return f.variable, f.err
}

func (f *fakeClient) VariableSpec(_ context.Context, namespace, path string) (nomad.VariableSource, error) {
	f.askedNamespace, f.variablePath = namespace, path

	return f.variableSpec, f.err
}

func (f *fakeClient) SubmitVariable(_ context.Context, namespace, path, source string, index uint64) error {
	f.wrote("SubmitVariable")
	f.askedNamespace, f.variablePath = namespace, path
	f.submittedSource, f.submittedIndex = source, index

	return f.refused(f.actionErr)
}

// refused is the next of the refusals, or otherwise what the call answers.
func (f *fakeClient) refused(otherwise error) error {
	if len(f.refusals) == 0 {
		return otherwise
	}

	err := f.refusals[0]
	f.refusals = f.refusals[1:]

	return err
}

func (f *fakeClient) NodePools(context.Context) ([]nomad.NodePool, error) {
	return f.nodePools, f.err
}

func (f *fakeClient) Events(_ context.Context, namespace string, topics []string) (*nomad.Changes, error) {
	f.watchedNamespace, f.watchedTopics = namespace, topics

	if f.watchErr != nil {
		return nil, f.watchErr
	}

	if f.changes == nil {
		return nil, errors.New("no stream")
	}

	return f.changes.stream(), nil
}

func (f *fakeClient) Servers(context.Context) ([]nomad.Server, error) {
	return f.servers, f.err
}

func (f *fakeClient) Server(_ context.Context, name string) (nomad.Server, error) {
	f.askedServer = name

	return f.server, f.err
}

func (f *fakeClient) RaftPeers(context.Context) ([]nomad.RaftPeer, error) {
	return f.raft, f.raftErr
}

func twoJobs() []nomad.Job {
	return []nomad.Job{
		{ID: "web", Name: "web", Namespace: "production", Type: "service", Status: "running", Running: 3, Desired: 3},
		{ID: "cron", Name: "cron", Namespace: "production", Type: "batch", Status: "dead"},
	}
}

func sizeMsg() tea.WindowSizeMsg {
	return tea.WindowSizeMsg{Width: 120, Height: 30}
}

func newTestModel(client Client) Model {
	m := New(client, Options{Namespace: "production", Version: "v-test", PollEvery: time.Millisecond})
	m, _ = m.update(tea.WindowSizeMsg{Width: 120, Height: 30})

	return m
}

// answered is what an ask of the screen came back with, without the label
// that ties it to the ask.
func answered(t *testing.T, msg tea.Msg) tea.Msg {
	t.Helper()

	answer, ok := msg.(answerMsg)
	require.True(t, ok, "%T", msg)

	return answer.msg
}

func TestFetchJobs_AsksInTheNamespace(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs()}

	msg := answered(t, newTestModel(client).fetch()())

	jobs, ok := msg.(jobsMsg)
	r.True(ok, "%T", msg)
	r.Len(jobs, 2)

	// The namespace of the session travels with the request.
	r.Equal("production", client.askedNamespace)
}

func TestFetchJobs_HandsTheErrorOver(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{err: errors.New("connection refused")}

	msg := answered(t, newTestModel(client).fetch()())

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
	r.IsType(jobsMsg{}, answered(t, cmd()))
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
	r.True(strings.HasPrefix(last, strings.Repeat(" ", headerPadX)+"<:>"), last)
}

func TestModel_ReadsWhatTheClusterIsUsing(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{usage: nomad.Usage{CPUPercent: 15, MemoryPercent: 31}}
	m := newTestModel(client)

	m, cmd := m.update(usageMsg{usage: client.usage})
	r.NotNil(cmd, "the next reading is scheduled")

	head := plain(renderHeader(m.headerData(), 120))
	r.Contains(head, "CPU:")
	r.Contains(head, "15%")
	r.Contains(head, "31%")
}

// clipboardOf is what a command puts on the clipboard.
func clipboardOf(cmd tea.Cmd) string {
	return fmt.Sprintf("%s", cmd())
}

// fakeChanges is the cluster saying when things change, without a cluster.
type fakeChanges struct {
	c      chan nomad.Change
	errs   chan error
	closed bool
}

func newChanges() *fakeChanges {
	return &fakeChanges{c: make(chan nomad.Change), errs: make(chan error, 1)}
}

func (f *fakeChanges) stream() *nomad.Changes {
	return nomad.NewChanges(f.c, f.errs, func() { f.closed = true })
}
