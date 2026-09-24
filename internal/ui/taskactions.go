package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// taskRuns says the task under the cursor runs: its client restarts or
// signals nothing else.
func taskRuns(m Model) bool {
	task, ok := selectedOf(m, screenTasks, m.tasks())

	return ok && task.State == statusRunning
}

// restartTask restarts the task under the cursor in place, once it is asked
// about. The rest of the allocation keeps running.
func restartTask(m Model) (Model, tea.Cmd) {
	task, ok := selectedOf(m, screenTasks, m.tasks())
	if !ok {
		return m, nil
	}

	client, screen := m.client, m.screen

	return m.ask(
		fmt.Sprintf("Really restart task %s of the allocation %s?", task.Name, shortID(screen.allocID)),
		act(fmt.Sprintf("Task %s restarted.", task.Name), func(ctx context.Context) error {
			return client.RestartTask(ctx, screen.namespace, screen.allocID, task.Name)
		}),
	)
}

// askSignal asks which signal to send to the task under the cursor. The line
// offers the one a task most often reloads on.
func askSignal(m Model) (Model, tea.Cmd) {
	task, ok := selectedOf(m, screenTasks, m.tasks())
	if !ok {
		return m, nil
	}

	m.overlay = overlaySignal
	m.prompt = promptModel{
		prefix: fmt.Sprintf("signal %s with: ", task.Name),
		text:   "SIGHUP",
		task:   task.Name,
	}
	m.layout()

	return m, nil
}

// signalTask sends the signal typed to the task, when it is a signal and the
// question that follows is answered yes.
func (m Model) signalTask(task, typed string) (Model, tea.Cmd) {
	signal, err := signalName(typed)
	if err != nil {
		return m.fail(err), nil
	}

	client, screen := m.client, m.screen

	return m.ask(
		fmt.Sprintf("Really send %s to task %s of the allocation %s?", signal, task, shortID(screen.allocID)),
		act(fmt.Sprintf("Sent %s to task %s.", signal, task), func(ctx context.Context) error {
			return client.SignalTask(ctx, screen.namespace, screen.allocID, task, signal)
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
