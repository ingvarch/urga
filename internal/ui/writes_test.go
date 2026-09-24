package ui

import (
	"reflect"
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
		"SubmitJob", "SubmitNamespace", "SubmitNodeMeta",
		"StartJob", "StopJob", "RevertJob", "RevertJobTo", "ScaleJob",
		"RestartAllocation", "StopAllocation",
		"DrainNode", "SetNodeEligible",
		"PromoteDeployment", "FailDeployment",
	}

	clientReads = []string{
		"Address", "Region", "Agent", "Regions", "Datacenters",
		"Jobs", "JobSpec", "JobVersions", "JobVersionDiff", "TaskGroups",
		"DescribeJob", "DescribeAllocation", "DescribeDeployment", "DescribeService",
		"Allocations", "NodeAllocations", "Allocation", "Logs",
		"Usage", "AllocationUsage", "NodeUsage",
		"Node", "NodeDetail", "NodeMeta", "NodeMetaSpec", "Nodes", "NodePools",
		"NamespaceSpec", "Namespaces", "Deployments", "Services", "Evaluations", "Evaluation", "Variables",
		"Servers", "Server", "RaftPeers", "Events",
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
// what it asks the way someone who means it would. A command that waits,
// like a timer, is left waiting.
func playOut(m Model, cmd tea.Cmd) Model {
	queue := []tea.Cmd{cmd}

	for range 300 {
		if len(queue) == 0 {
			next, answered, acted := answer(m)
			if !acted {
				return next
			}

			m, queue = next, append(queue, answered)

			continue
		}

		run := queue[0]
		queue = queue[1:]

		msg := within(run, 50*time.Millisecond)
		if msg == nil {
			continue
		}

		if batch, ok := msg.(tea.BatchMsg); ok {
			queue = append(queue, batch...)

			continue
		}

		var next tea.Cmd
		m, next = m.update(msg)
		queue = append(queue, next)
	}

	return m
}

// answer says yes to a question and gives the scale line a count.
func answer(m Model) (Model, tea.Cmd, bool) {
	switch m.overlay {
	case overlayConfirm:
		next, cmd := m.update(key('y'))

		return next, cmd, true

	case overlayScale:
		m, _ = m.update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
		m = typeIn(m, "5")
		next, cmd := m.update(enter())

		return next, cmd, true
	}

	return m, nil, false
}

// within is what a command answers, unless it takes longer than a moment.
func within(cmd tea.Cmd, wait time.Duration) tea.Msg {
	if cmd == nil {
		return nil
	}

	answered := make(chan tea.Msg, 1)

	go func() { answered <- cmd() }()

	select {
	case msg := <-answered:
		return msg
	case <-time.After(wait):
		return nil
	}
}

func TestBindings_TheKeysThatChangeTheCluster(t *testing.T) {
	// Saving a screen writes a file where urga runs, editing one in TMPDIR.
	t.Chdir(t.TempDir())
	t.Setenv("TMPDIR", t.TempDir())

	for name, open := range everyScreen(t) {
		for _, b := range open.screen.bindings() {
			fake, ok := open.client.(*fakeClient)
			require.True(t, ok)

			fake.writes = nil
			shell := &fakeShell{}

			// Enter on a task follows its output.
			fake.logs = &nomad.LogStream{Lines: make(chan string)}

			m := open.quiet()
			m.opts.Editor = &fakeEditor{replace: "changed"}
			m.opts.Shell = shell

			m, cmd := b.do(m)
			playOut(m, cmd)

			wrote := len(fake.writes) > 0 || shell.opened != (shellCommand{})

			// Read-only takes away the keys marked as writes. A key that
			// writes without the mark stays, and changes the cluster.
			if wrote != b.writes {
				t.Errorf("%s on the %s screen: marked writes=%t, changed the cluster: %t %v",
					b.press, name, b.writes, wrote, fake.writes)
			}
		}
	}
}
