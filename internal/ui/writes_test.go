package ui

import (
	"fmt"
	"reflect"
	"runtime"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// clientWrites are the methods of Client that change the cluster, and
// clientReads the ones that only ask it. Every method is in one of the two:
// a key that reaches a write is a key read-only mode has to take away.
var (
	clientWrites = []string{
		"SubmitJob", "SubmitNamespace", "SubmitNodeMeta", "SubmitVariable",
		"StartJob", "StopJob", "LaunchJob", "RevertJobTo", "ScaleJob",
		"RestartAllocation", "RestartAllTasks", "StopAllocation", "RestartTask", "SignalTask", "RunCommand", "PromoteGroups", "PauseDeployment", "DeleteServiceRegistration",
		"DrainNode", "SetNodeEligible", "RenewNodeIdentity",
		"PromoteDeployment", "FailDeployment",
		"DetachVolume", "ReleaseClaim", "DispatchJob", "EvaluateJob", "RescheduleFailed",
		"TagVersion", "UntagVersion", "MarkStable", "SetAllocHealth", "ReleaseLock",
		"SubmitACL", "CreateACL", "DeleteACL", "SubmitScheduler",
		"DeleteVariable", "CreateNamespace", "DeleteNamespace",
		"PurgeJob", "PurgeNode", "CollectNode", "CollectGarbage",
	}

	clientReads = []string{
		"Address", "SendsTokenInClear", "Region", "Agent", "Regions", "Datacenters", "AllocationChecks", "Files", "File", "Deployment", "JobDeployments", "DeploymentAllocations", "Token", "ServiceInstances",
		"Jobs", "NextLaunch", "JobSpec", "JobMeta", "JobVersions", "JobVersionDiff", "TaskGroups", "PlanJob", "PlanRevert", "JobTarget", "JobCounts",
		"DescribeJob", "DescribeAllocation", "DescribeDeployment", "DescribeService",
		"Allocations", "NodeAllocations", "Allocation", "Logs", "LogBefore", "AgentLogs",
		"Usage", "AllocationUsage", "TaskUsage", "NodeUsage",
		"Node", "NodeIdentity", "WithAllocLimits", "NodeDetail", "NodeMeta", "NodeMetaSpec", "Nodes", "NodePools", "PoolJobs", "PoolNodes", "BatchQueueJobs", "BatchQueueStatus",
		"NamespaceSpec", "Namespaces", "Deployments", "Services", "Evaluations", "Evaluation", "EvaluationAllocations", "FailedPlacement", "NewestEvaluation", "Variables", "Variable", "VariableSpec",
		"Servers", "Server", "RaftPeers", "Events", "Failures", "LatestIndex",
		"Volumes", "Volume", "DescribeVolume", "Plugins", "Plugin", "DescribePlugin",
		"Find", "DispatchForm", "ServerHealth",
		"ACLObjects", "DescribeACL", "TokenSecret",
		"Scheduler", "SchedulerSpec",
		"ScalingPolicies", "DescribeScalingPolicy", "GroupScaling", "Feed",
	}
)

func TestClient_EveryMethodIsAReadOrAWrite(t *testing.T) {
	iface := reflect.TypeFor[Client]()

	for i := range iface.NumMethod() {
		name := iface.Method(i).Name
		write, read := slices.Contains(clientWrites, name), slices.Contains(clientReads, name)

		if write == read {
			t.Errorf("%s must be in exactly one of clientWrites and clientReads", name)
		}
	}

	// A name left behind by a method that is gone checks nothing.
	for _, name := range slices.Concat(clientWrites, clientReads) {
		if _, ok := iface.MethodByName(name); !ok {
			t.Errorf("%s is listed but Client has no such method", name)
		}
	}
}

func TestFake_RecordsEveryWrite(t *testing.T) {
	for _, name := range clientWrites {
		fake := &fakeClient{}

		method := reflect.ValueOf(fake).MethodByName(name)
		require.True(t, method.IsValid(), name)

		args := make([]reflect.Value, method.Type().NumIn())
		for i := range args {
			args[i] = reflect.Zero(method.Type().In(i))
		}

		method.Call(args)

		// The test of the keys counts what the fake kept. A write the fake
		// does not keep is a key that changes the cluster unnoticed.
		require.Equal(t, []string{name}, fake.writes, name)
	}
}

// playOut runs what a key started the way the program does, and answers
// what it asks the way someone who means it would. Timers and stream waits
// are held while it runs, so each answers at once: a timer with what it
// would send, which nothing takes, and a stream wait with what came, or
// with nothing when nothing came. Every other command is waited for until
// it answers.
func playOut(m Model, cmd tea.Cmd) Model {
	defer timersHeld.Store(timersHeld.Swap(true))
	defer streamsHeld.Store(streamsHeld.Swap(true))

	queue := []tea.Cmd{cmd}

	// A plan is answered when the key opened it. One that was on the screen
	// before the key is not sent.
	_, planned := m.screen.page.(planPage)

	for range 300 {
		if len(queue) == 0 {
			if _, onPlan := m.screen.page.(planPage); planned && onPlan && m.overlay == overlayNone {
				return m
			}

			next, answered, acted := answer(m)
			if !acted {
				return m
			}

			m, queue = next, append(queue, answered)

			continue
		}

		run := queue[0]
		queue = queue[1:]

		switch msg := answerOf(run, slowest).(type) {
		case nil, heldStream:
		case tea.BatchMsg:
			queue = append(queue, msg...)
		default:
			var next tea.Cmd
			m, next = m.update(msg)
			queue = append(queue, next)
		}
	}

	return m
}

// slowest is how long playOut waits for a command to answer. One that takes
// longer panics with the name of the command, which ends the run of the
// package; the test would otherwise hang.
const slowest = 5 * time.Second

// answerOf is what cmd answers. It panics when cmd takes longer than wait.
func answerOf(cmd tea.Cmd, wait time.Duration) tea.Msg {
	msg, ok := answerIn(cmd, wait)
	if !ok {
		panic(fmt.Sprintf("%s did not answer in %s", runtime.FuncForPC(reflect.ValueOf(cmd).Pointer()).Name(), wait))
	}

	return msg
}

// answerIn is what cmd answers, and false when it takes longer than wait.
func answerIn(cmd tea.Cmd, wait time.Duration) (tea.Msg, bool) {
	if cmd == nil {
		return nil, true
	}

	answered := make(chan tea.Msg, 1)

	go func() { answered <- cmd() }()

	select {
	case msg := <-answered:
		return msg, true
	case <-time.After(wait):
		return nil, false
	}
}

func TestPlayOut_TakesAnAnswerThatComesLate(t *testing.T) {
	m := loadedModel(t)

	late := func() tea.Msg {
		time.Sleep(80 * time.Millisecond)

		return sayMsg("the answer came late")
	}

	m = playOut(m, late)

	// A loaded machine can take longer than a moment for what is no timer.
	require.Contains(t, plain(m.render()), "the answer came late")
}

func TestPlayOut_WaitsForSlowWorkUntilItAnswers(t *testing.T) {
	m := loadedModel(t)

	slow := func() tea.Msg {
		time.Sleep(300 * time.Millisecond)

		return sayMsg("the slow work is done")
	}

	m = playOut(m, slow)

	require.Contains(t, plain(m.render()), "the slow work is done")
}

func TestAnswerOf_PanicsOnWorkThatNeverAnswers(t *testing.T) {
	never := make(chan struct{})
	t.Cleanup(func() { close(never) })

	stuck := func() tea.Msg {
		<-never

		return nil
	}

	require.PanicsWithValue(t, runtime.FuncForPC(reflect.ValueOf(stuck).Pointer()).Name()+" did not answer in 10ms",
		func() { answerOf(stuck, 10*time.Millisecond) })
}

// answer says yes to a question and to a plan, and answers a line with what
// it offers, or with a word when it offers nothing.
func answer(m Model) (Model, tea.Cmd, bool) {
	if _, onPlan := m.screen.page.(planPage); onPlan && m.overlay == overlayNone {
		next, cmd := m.update(key('y'))

		return next, cmd, true
	}

	switch m.overlay {
	case overlayConfirm:
		next, cmd := m.update(key('y'))

		return next, cmd, true

	case overlayAnswer:
		if m.prompt.text == "" {
			m = typeIn(m, "answer")
		}

		next, cmd := m.update(enter())

		return next, cmd, true
	}

	return m, nil, false
}

// within is what a command answers, unless it takes longer than a moment.
func within(cmd tea.Cmd, wait time.Duration) tea.Msg {
	msg, _ := answerIn(cmd, wait)

	return msg
}

func TestBindings_TheKeysThatChangeTheCluster(t *testing.T) {
	// Saving a screen writes where urga runs, editing writes in the temp dir.
	t.Chdir(t.TempDir())
	tempDir(t)

	// A key the page does not offer now does nothing when pressed. It is
	// checked on a screen that offers it, and some screen has to.
	marked, pressed := map[string]bool{}, map[string]bool{}

	for name, open := range everyScreen(t) {
		for _, k := range open.pageKeys() {
			of := fmt.Sprintf("%T %s", open.screen.page, k.press)
			if k.writes {
				marked[of] = true
			}

			if !k.offered {
				continue
			}

			pressed[of] = true

			fake, ok := open.client.(*fakeClient)
			require.True(t, ok)

			fake.writes = nil
			shell := &fakeShell{}

			// Enter on a task follows its output.
			fake.logs = &nomad.LogStream{Lines: make(chan string)}

			m := open.quiet()
			m.opts.Editor = &fakeEditor{replace: "changed"}
			m.opts.Shell = shell

			m, cmd := m.pressPage(k.press)
			playOut(m, cmd)

			wrote := len(fake.writes) > 0 || shell.opened != (shellCommand{})

			// Read-only takes away the keys marked as writes. A key that
			// writes without the mark stays, and changes the cluster.
			if wrote != k.writes {
				t.Errorf("%s on the %s screen: marked writes=%t, changed the cluster: %t %v",
					k.press, name, k.writes, wrote, fake.writes)
			}
		}
	}

	for of := range marked {
		if !pressed[of] {
			t.Errorf("%s changes the cluster and no screen offers it", of)
		}
	}
}

func TestBindings_AKeyLabelsItsWorkAWriteOnlyWhenTheWorkWrites(t *testing.T) {
	// Editing writes its file in the temp dir.
	t.Chdir(t.TempDir())
	tempDir(t)

	for name, open := range everyScreen(t) {
		for _, k := range open.pageKeys() {
			if !k.offered {
				continue
			}

			fake, ok := open.client.(*fakeClient)
			require.True(t, ok)

			fake.writes = nil
			fake.logs = &nomad.LogStream{Lines: make(chan string)}

			m := open.quiet()

			_, out, pressed := m.screen.page.press(k.press, m.env())
			require.True(t, pressed, "%s on the %s screen", k.press, name)

			runOnce(out.cmd)

			// What a key only reads is dropped once its screen is left. A
			// write that a question, a line or the editor comes before is
			// labelled when it is sent.
			if wrote := len(fake.writes) > 0; wrote != out.writes {
				t.Errorf("%s on the %s screen: labelled a write: %t, changed the cluster: %t %v",
					k.press, name, out.writes, wrote, fake.writes)
			}
		}
	}
}

// runOnce runs a command and the commands of a batch it returns, and takes
// none of their answers.
func runOnce(cmd tea.Cmd) {
	if batch, ok := within(cmd, 200*time.Millisecond).(tea.BatchMsg); ok {
		for _, c := range batch {
			runOnce(c)
		}
	}
}
