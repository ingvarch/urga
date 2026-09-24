package ui

import (
	"context"
	"fmt"
	"image/color"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// jobLogLimit is how many logs one screen reads at once: each one is a
// stream kept open to a client.
const jobLogLimit = 20

// tagColours tell the allocations of a log apart, in the order they are
// read.
var tagColours = []color.Color{
	colorTitle, colorLabel, colorPending, colorCanary, colorBlue, colorAttention, colorAccent, colorActive,
}

// Messages of the logs of a job.
type (
	// logScopeMsg is the allocations a key asked about: which task each of
	// them runs is what the logs are read from.
	logScopeMsg struct {
		scope  screen
		allocs []nomad.Alloc
	}

	// jobLogOpenedMsg is the log of one allocation, opened for a reading.
	jobLogOpenedMsg struct {
		reading int
		allocID string
		stream  *nomad.LogStream
		err     error
	}

	// jobLogAllocsMsg is the allocations read again, for a reading.
	jobLogAllocsMsg struct {
		reading int
		allocs  []nomad.Alloc
	}
)

// logChoice is a task and the allocations that run it.
type logChoice struct {
	job, group, task string
	allocs           []nomad.Alloc
}

// logPick is the question which task to read: what it was asked about, and
// the answers.
type logPick struct {
	title   string
	scope   screen
	choices []logChoice
}

// jobLogsState is the log of one task, read from every allocation that runs
// it.
type jobLogsState struct {
	// reading counts the times the logs were opened: a stream opened for an
	// earlier time is let go of.
	reading int

	// streams are the logs open now, by the allocation they read; tags say
	// which allocation a line came from.
	streams map[*nomad.LogStream]string
	tags    map[string]tag

	// all are the allocations that run the task, the newest first, and
	// allocs the ones read: no more than the limit.
	all    []nomad.Alloc
	allocs []nomad.Alloc
}

// stop closes every stream, which stops the requests behind them. What
// they still send is for no one.
func (l *jobLogsState) stop() {
	for stream := range l.streams {
		stream.Close()
	}

	l.streams = nil
	l.reading++
}

// jobLogs reads the logs of the job under the cursor.
func jobLogs(m Model) (Model, tea.Cmd) {
	job, ok := selectedOf(m, screenJobs, m.jobs)
	if !ok {
		return m, nil
	}

	return m.askLogScope(screen{kind: screenAllocations, namespace: job.Namespace, jobID: job.ID})
}

// groupLogs reads the logs of the task group under the cursor.
func groupLogs(m Model) (Model, tea.Cmd) {
	group, ok := selectedOf(m, screenTaskGroups, m.groups)
	if !ok {
		return m, nil
	}

	return m.askLogScope(screen{kind: screenAllocations, namespace: m.screen.namespace, jobID: m.screen.jobID, taskGroup: group.Name})
}

// listLogs reads the logs of the allocations of the list on the screen.
func listLogs(m Model) (Model, tea.Cmd) {
	return m.askLogScope(m.screen)
}

// askLogScope reads the allocations the logs are to come from: which of
// them run, and which tasks.
func (m Model) askLogScope(scope screen) (Model, tea.Cmd) {
	return m, askedFor(m.asked, request(allocsOf(m.client, scope), func(allocs []nomad.Alloc) tea.Msg {
		return logScopeMsg{scope: scope, allocs: allocs}
	}))
}

// allocsOf reads the allocations of a list: of a deployment, of a client, of
// a job, or of the namespace.
func allocsOf(client Client, s screen) func(ctx context.Context) ([]nomad.Alloc, error) {
	return func(ctx context.Context) ([]nomad.Alloc, error) {
		switch {
		case s.deploymentID != "":
			return client.DeploymentAllocations(ctx, s.namespace, s.deploymentID)
		case s.nodeID != "":
			return client.NodeAllocations(ctx, s.nodeID)
		}

		return client.Allocations(ctx, s.namespace, s.jobID)
	}
}

// choices are the tasks that run in the allocations of a list, each with
// the allocations that run it, the newest first.
func choices(scope screen, allocs []nomad.Alloc) []logChoice {
	byTask := map[[3]string]*logChoice{}

	for _, alloc := range allocs {
		if alloc.Status != statusRunning || !inScope(scope, alloc) {
			continue
		}

		for _, task := range alloc.Tasks {
			key := [3]string{alloc.JobID, alloc.TaskGroup, task.Name}
			if byTask[key] == nil {
				byTask[key] = &logChoice{job: alloc.JobID, group: alloc.TaskGroup, task: task.Name}
			}

			byTask[key].allocs = append(byTask[key].allocs, alloc)
		}
	}

	out := make([]logChoice, 0, len(byTask))
	for _, choice := range byTask {
		slices.SortFunc(choice.allocs, func(a, b nomad.Alloc) int { return b.Created.Compare(a.Created) })
		out = append(out, *choice)
	}

	slices.SortFunc(out, func(a, b logChoice) int {
		return strings.Compare(a.job+"/"+a.group+"/"+a.task, b.job+"/"+b.group+"/"+b.task)
	})

	return out
}

// inScope says the allocation belongs to what the logs were asked of.
func inScope(scope screen, alloc nomad.Alloc) bool {
	return (scope.jobID == "" || alloc.JobID == scope.jobID) &&
		(scope.taskGroup == "" || alloc.TaskGroup == scope.taskGroup)
}

// scopeName names what the logs were asked of, for a question or a warning.
func scopeName(scope screen) string {
	switch {
	case scope.taskGroup != "":
		return scope.taskGroup
	case scope.jobID != "":
		return scope.jobID
	case scope.label != "":
		return scope.label
	case scope.deploymentID != "":
		return "deployment " + shortID(scope.deploymentID)
	}

	return "the namespace"
}

// showLogScope reads the only task there is, or asks which one.
func (m Model) showLogScope(msg logScopeMsg) (Model, tea.Cmd) {
	found := choices(msg.scope, msg.allocs)

	switch len(found) {
	case 0:
		return m.warn(fmt.Sprintf("%s has no allocation running", scopeName(msg.scope))), nil
	case 1:
		return m.openJobLogs(msg.scope, found[0])
	}

	m.logPick = logPick{title: pickTitle(msg.scope), scope: msg.scope, choices: found}

	return m.push(screen{kind: screenLogTasks, namespace: msg.scope.namespace, jobID: msg.scope.jobID})
}

// pickTitle says what the question of which task is about.
func pickTitle(scope screen) string {
	switch {
	case scope.jobID != "":
		return fmt.Sprintf("Logs of which task? (Job: %s)", scope.jobID)
	case scope.nodeID != "":
		return fmt.Sprintf("Logs of which task? (Client: %s)", scope.label)
	}

	return "Logs of which task?"
}

// logTaskTitles are the columns of the question of which task.
var logTaskTitles = []string{"Task", "Allocations"}

// logTaskRows are the tasks to pick from. The job is named when there are
// tasks of several.
func logTaskRows(found []logChoice) []tableRow {
	jobs := map[string]bool{}
	for _, choice := range found {
		jobs[choice.job] = true
	}

	rows := make([]tableRow, 0, len(found))

	for _, choice := range found {
		name := choice.group + "/" + choice.task
		if len(jobs) > 1 {
			name = choice.job + "/" + name
		}

		rows = append(rows, tableRow{cells: []string{name, fmt.Sprintf("%d running", len(choice.allocs))}})
	}

	return rows
}

var logTaskBindings = []binding{{press: "enter", label: "Logs", do: pickLogTask}}

// pickLogTask reads the task under the cursor.
func pickLogTask(m Model) (Model, tea.Cmd) {
	choice, ok := selectedOf(m, screenLogTasks, m.logPick.choices)
	if !ok {
		return m, nil
	}

	return m.openJobLogs(m.logPick.scope, choice)
}

// openJobLogs follows what the task writes to stdout in every allocation
// that runs it, on top of the screen it was asked from: escape closes them
// all and goes back.
func (m Model) openJobLogs(scope screen, choice logChoice) (Model, tea.Cmd) {
	next := scope
	next.kind, next.jobID, next.taskGroup = screenJobLogs, choice.job, choice.group
	next.task, next.source = choice.task, nomad.LogStdout

	m = m.stackText(next, textModel{})
	m.logs = logState{following: true}

	return m.readJobLogs(choice.allocs)
}

// readJobLogs opens a stream for each of the allocations, the newest first
// and no more than the limit.
func (m Model) readJobLogs(allocs []nomad.Alloc) (Model, tea.Cmd) {
	m.jobLogs.stop()

	read := allocs[:min(len(allocs), jobLogLimit)]

	m.jobLogs = jobLogsState{
		reading: m.jobLogs.reading,
		streams: map[*nomad.LogStream]string{},
		tags:    map[string]tag{},
		all:     allocs,
		allocs:  read,
	}

	client, s, reading := m.client, m.screen, m.jobLogs.reading
	cmds := make([]tea.Cmd, 0, len(read))

	for i, alloc := range read {
		style := lipgloss.NewStyle().Foreground(tagColours[i%len(tagColours)])
		m.jobLogs.tags[alloc.ID] = tag{text: shortID(alloc.ID) + " │ ", style: style}

		id := alloc.ID
		cmds = append(cmds, func() tea.Msg {
			stream, err := client.Logs(context.Background(), s.namespace, id, s.task, s.source)

			return jobLogOpenedMsg{reading: reading, allocID: id, stream: stream, err: err}
		})
	}

	return m, tea.Batch(cmds...)
}

// openedJobLog keeps a stream that was opened for the reading on the
// screen, and lets go of one opened for another.
func (m Model) openedJobLog(msg jobLogOpenedMsg) (Model, tea.Cmd) {
	if msg.reading != m.jobLogs.reading || m.screen.kind != screenJobLogs {
		if msg.stream != nil {
			msg.stream.Close()
		}

		return m, nil
	}

	if msg.err != nil {
		return m.jobLogLine(msg.allocID, "could not read: "+msg.err.Error(), &styleError), nil
	}

	m.jobLogs.streams[msg.stream] = msg.allocID

	return m, waitForStream(msg.stream)
}

// appendJobLog puts what an allocation wrote at the end, behind the tag of
// the allocation.
func (m Model) appendJobLog(stream *nomad.LogStream, allocID, chunk string) (Model, tea.Cmd) {
	for _, line := range strings.Split(strings.TrimSuffix(chunk, "\n"), "\n") {
		m = m.jobLogLine(allocID, line, nil)
	}

	return m, waitForStream(stream)
}

// endJobLog lets go of a stream that ended, and says so: the others go on.
func (m Model) endJobLog(stream *nomad.LogStream, allocID string) Model {
	delete(m.jobLogs.streams, stream)

	return m.jobLogLine(allocID, "stopped", &styleMuted)
}

// jobLogLine adds one line of an allocation, in a style of its own when it
// is not something the task wrote.
func (m Model) jobLogLine(allocID, line string, style *lipgloss.Style) Model {
	at := len(m.text.lines)

	if m.text.stamps == nil {
		m.text.stamps = map[int]time.Time{}
	}

	if m.text.tags == nil {
		m.text.tags = map[int]tag{}
	}

	m.text.lines = append(m.text.lines, line)
	m.text.stamps[at] = time.Now()
	m.text.tags[at] = m.jobLogs.tags[allocID]

	if style != nil {
		if m.text.paint == nil {
			m.text.paint = map[int]lipgloss.Style{}
		}

		m.text.paint[at] = *style
	}

	if m.logs.following {
		m.text.toEnd()
	}

	return m
}

// jobLogsTitle says which task of which job, which of its outputs, and how
// many of its allocations are read.
func jobLogsTitle(s screen, logs jobLogsState) string {
	read := plural(len(logs.allocs), "allocation")
	if len(logs.all) > len(logs.allocs) {
		read = fmt.Sprintf("%d of %d allocations", len(logs.allocs), len(logs.all))
	}

	return fmt.Sprintf("Logs (Job: %s, Task: %s) [%s, %s]", s.jobID, s.task, s.source, read)
}

// reloadJobLogs reads the allocations again and opens the logs of the ones
// that run the task now: a deployment may have replaced them since.
func reloadJobLogs(m Model) (Model, tea.Cmd) {
	m.jobLogs.stop()
	m.text = m.text.emptied()

	reading := m.jobLogs.reading

	return m, request(allocsOf(m.client, m.screen), func(allocs []nomad.Alloc) tea.Msg {
		return jobLogAllocsMsg{reading: reading, allocs: allocs}
	})
}

// reloadedJobLogs opens the logs of the allocations read again, when they
// are for the reading on the screen.
func (m Model) reloadedJobLogs(msg jobLogAllocsMsg) (Model, tea.Cmd) {
	if msg.reading != m.jobLogs.reading || m.screen.kind != screenJobLogs {
		return m, nil
	}

	s := m.screen

	for _, choice := range choices(s, msg.allocs) {
		if choice.task == s.task {
			return m.readJobLogs(choice.allocs)
		}
	}

	m, cmd := m.readJobLogs(nil)

	return m.warn(fmt.Sprintf("%s runs in no allocation now", s.task)), cmd
}

// switchJobSource reads the other of the two outputs, from the same
// allocations.
func switchJobSource(m Model) (Model, tea.Cmd) {
	m.screen.source = otherSource(m.screen.source)
	m.text = m.text.emptied()
	m.layout()

	return m.readJobLogs(m.jobLogs.all)
}

// jobLogBindings are the keys of the logs of a job.
var jobLogBindings = []binding{
	{press: "r", label: "Reload", do: reloadJobLogs},
	// The key that opens stderr from the tasks switches to the other of
	// the two here, and says which one it goes to.
	{press: "ctrl+e", label: "Stderr", do: switchJobSource, offered: onSource(nomad.LogStdout)},
	{press: "ctrl+e", label: "Stdout", do: switchJobSource, offered: onSource(nomad.LogStderr)},
	{press: "s", label: "Toggle Autoscroll", do: toggleAutoscroll},
	{press: "w", label: "Toggle Wrap", do: wrapLines},
	{press: "t", label: "Toggle Timestamps", do: showTimes},
	{press: "ctrl+s", label: "Save", do: saveScreen},
}
