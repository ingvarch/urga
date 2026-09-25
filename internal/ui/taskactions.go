package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// taskRef is a task of an allocation, and the namespace the allocation
// lives in, which is where it is asked.
type taskRef struct {
	namespace, allocID, name string
}

// taskRuns says the task under the cursor runs: its client restarts or
// signals nothing else.
func taskRuns(p tasksPage, e env) bool {
	task, ok := p.picked(e)

	return ok && task.State == statusRunning
}

// restartTask restarts the task under the cursor in place, once it is asked
// about. The rest of the allocation keeps running.
func restartTask(p tasksPage, e env) (tasksPage, outcome) {
	task, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	client, namespace, allocID := e.client, p.namespace, p.allocID

	return p, then(askMsg{
		question: fmt.Sprintf("Really restart task %s of the allocation %s?", task.Name, shortID(allocID)),
		apply: act(fmt.Sprintf("Task %s restarted.", task.Name), func(ctx context.Context) error {
			return client.RestartTask(ctx, namespace, allocID, task.Name)
		}),
	})
}

// askSignal asks which signal to send to the task under the cursor.
func askSignal(p tasksPage, e env) (tasksPage, outcome) {
	task, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	return p, then(signalMsg{namespace: p.namespace, allocID: p.allocID, name: task.Name})
}

// askForSignal puts up the line that asks which signal to send to a task. It
// offers the one a task most often reloads on.
func (m Model) askForSignal(task taskRef) (Model, tea.Cmd) {
	m.overlay = overlaySignal
	m.prompt = promptModel{
		prefix: fmt.Sprintf("signal %s with: ", task.name),
		text:   "SIGHUP",
		task:   task,
	}
	m.layout()

	return m, nil
}

// signalTask sends the signal typed to the task, when it is a signal and the
// question that follows is answered yes.
func (m Model) signalTask(task taskRef, typed string) (Model, tea.Cmd) {
	signal, err := signalName(typed)
	if err != nil {
		return m.fail(err), nil
	}

	client := m.client

	return m.ask(
		fmt.Sprintf("Really send %s to task %s of the allocation %s?", signal, task.name, shortID(task.allocID)),
		act(fmt.Sprintf("Sent %s to task %s.", signal, task.name), func(ctx context.Context) error {
			return client.SignalTask(ctx, task.namespace, task.allocID, task.name, signal)
		}),
	)
}

// signals are the names a client knows. For any other name it sends SIGINT
// instead, which stops most tasks.
var signals = []string{
	"SIGNULL", "SIGABRT", "SIGALRM", "SIGBUS", "SIGCONT", "SIGFPE", "SIGHUP",
	"SIGILL", "SIGINT", "SIGIO", "SIGIOT", "SIGKILL", "SIGPIPE", "SIGPROF",
	"SIGQUIT", "SIGSEGV", "SIGSTOP", "SIGSYS", "SIGTERM", "SIGTRAP", "SIGTSTP",
	"SIGTTIN", "SIGTTOU", "SIGUSR1", "SIGUSR2", "SIGWINCH", "SIGXCPU", "SIGXFSZ",
}

// signalName is a signal as the client names it, SIGHUP, whether it was
// typed hup, sighup or SIGHUP.
func signalName(typed string) (string, error) {
	name := strings.ToUpper(strings.TrimSpace(typed))
	if !strings.HasPrefix(name, "SIG") {
		name = "SIG" + name
	}

	if !slices.Contains(signals, name) {
		return "", fmt.Errorf("%q is not a signal", typed)
	}

	return name, nil
}
