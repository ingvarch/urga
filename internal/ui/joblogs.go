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

// logScope is what the logs are read of: the allocations of a job, of one
// group of it, of a client, or of a deployment.
type logScope struct {
	namespace, jobID, group string

	// nodeID and deploymentID read the allocations of a client or of a
	// deployment rather than those of the job; label names the client.
	nodeID, deploymentID, label string
}

// logChoice is a task and the allocations that run it.
type logChoice struct {
	job, group, task string
	allocs           []nomad.Alloc
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
func jobLogs(p jobsPage, e env) (jobsPage, outcome) {
	job, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	return p, askLogScope(e, logScope{namespace: job.Namespace, jobID: job.ID})
}

// groupLogs reads the logs of the task group under the cursor.
func groupLogs(p taskGroupsPage, e env) (taskGroupsPage, outcome) {
	group, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	return p, askLogScope(e, logScope{namespace: p.namespace, jobID: p.jobID, group: group.Name})
}

// askLogScope reads the allocations the logs are to come from: which of
// them run, and which tasks. What they answer is for the screen that asked.
func askLogScope(e env, scope logScope) outcome {
	return then(requestMsg(request(allocsOf(e.client, scope), func(allocs []nomad.Alloc) tea.Msg {
		return showLogScope(scope, allocs)
	})))
}

// allocsOf reads the allocations of a list: of a deployment, of a client, of
// a job, or of the namespace.
func allocsOf(client allocsClient, s logScope) func(ctx context.Context) ([]nomad.Alloc, error) {
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
func choices(scope logScope, allocs []nomad.Alloc) []logChoice {
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
func inScope(scope logScope, alloc nomad.Alloc) bool {
	return (scope.jobID == "" || alloc.JobID == scope.jobID) &&
		(scope.group == "" || alloc.TaskGroup == scope.group)
}

// scopeName names what the logs were asked of, for a question or a warning.
func scopeName(scope logScope) string {
	switch {
	case scope.group != "":
		return scope.group
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
func showLogScope(scope logScope, allocs []nomad.Alloc) tea.Msg {
	found := choices(scope, allocs)

	switch len(found) {
	case 0:
		return warnMsg(fmt.Sprintf("%s has no allocation running", scopeName(scope)))
	case 1:
		return openMsg(jobLogsScreen(scope, found[0]))
	}

	return openMsg(screen{kind: screenLogTasks, page: logTasksPage{scope: scope, choices: found}})
}

// logTasksPage is the question which task to read: what it was asked
// about, and the answers.
type logTasksPage struct {
	noAnswers

	scope   logScope
	choices []logChoice
}

// title says what the question of which task is about.
func (p logTasksPage) title(env, int) string {
	switch {
	case p.scope.jobID != "":
		return fmt.Sprintf("Logs of which task? (Job: %s)", p.scope.jobID)
	case p.scope.nodeID != "":
		return fmt.Sprintf("Logs of which task? (Client: %s)", p.scope.label)
	}

	return "Logs of which task?"
}

func (logTasksPage) titles() []string      { return logTaskTitles }
func (logTasksPage) topics() []string      { return nil }
func (logTasksPage) fetch(env) tea.Cmd     { return nil }
func (p logTasksPage) rows(env) []tableRow { return logTaskRows(p.choices) }

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

var logTasksKeys = []pageKey[logTasksPage]{{press: "enter", label: "Logs", do: pickLogTask}}

func (p logTasksPage) keys(e env) []keyHint { return hintsOf(p, e, logTasksKeys) }

func (p logTasksPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, logTasksKeys, k)
}

// pickLogTask reads the task under the cursor.
func pickLogTask(p logTasksPage, e env) (logTasksPage, outcome) {
	choice, ok := pickedFrom(e, p.choices)
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg(jobLogsScreen(p.scope, choice)))
}

// jobLogsPage follows what a task writes in every allocation that runs it.
type jobLogsPage struct {
	// scope is what the allocations are read of, narrowed to the job and
	// the group of the task.
	scope        logScope
	task, source string

	// first are the allocations the task was picked with, read when the
	// page is entered the first time. Every time after that they are read
	// again: a deployment may have replaced them meanwhile.
	first []nomad.Alloc

	logs    jobLogsState
	content textContent
}

// jobLogsScreen follows what the task writes to stdout in every allocation
// that runs it, on top of the screen it was asked from: escape closes them
// all and goes back.
func jobLogsScreen(scope logScope, choice logChoice) screen {
	scope.jobID, scope.group = choice.job, choice.group

	p := jobLogsPage{scope: scope, task: choice.task, source: nomad.LogStdout, first: choice.allocs}

	return screen{kind: screenJobLogs, page: p}
}

// title says which task of which job, which of its outputs, and how many of
// its allocations are read.
func (p jobLogsPage) title(env, int) string {
	read := plural(len(p.logs.allocs), "allocation")
	if len(p.logs.all) > len(p.logs.allocs) {
		read = fmt.Sprintf("%d of %d allocations", len(p.logs.allocs), len(p.logs.all))
	}

	return fmt.Sprintf("Logs (Job: %s, Task: %s) [%s, %s]", p.scope.jobID, p.task, p.source, read)
}

func (jobLogsPage) titles() []string       { return nil }
func (jobLogsPage) topics() []string       { return nil }
func (jobLogsPage) fetch(env) tea.Cmd      { return nil }
func (jobLogsPage) rows(env) []tableRow    { return nil }
func (jobLogsPage) follows() bool          { return true }
func (p jobLogsPage) text(env) textContent { return p.content }

func (p jobLogsPage) saveAs() (string, string) {
	return fmt.Sprintf("%s-%s-%s", p.scope.jobID, p.task, p.source), "log"
}

// open reads the allocations the task was picked with, the first time.
// Coming back, they are read again.
func (p jobLogsPage) open(e env) (page, tea.Cmd) {
	if p.first != nil {
		first := p.first
		p.first = nil

		return p.read(e.client, first)
	}

	return p.reload(e.client)
}

func (p jobLogsPage) close() page {
	p.logs.stop()

	return p
}

// read opens a stream for each of the allocations, the newest first and no
// more than the limit.
func (p jobLogsPage) read(client filesClient, allocs []nomad.Alloc) (jobLogsPage, tea.Cmd) {
	p.logs.stop()

	read := allocs[:min(len(allocs), jobLogLimit)]

	p.logs = jobLogsState{
		reading: p.logs.reading,
		streams: map[*nomad.LogStream]string{},
		tags:    map[string]tag{},
		all:     allocs,
		allocs:  read,
	}

	namespace, task, source, reading := p.scope.namespace, p.task, p.source, p.logs.reading
	cmds := make([]tea.Cmd, 0, len(read))

	for i, alloc := range read {
		style := lipgloss.NewStyle().Foreground(tagColours[i%len(tagColours)])
		p.logs.tags[alloc.ID] = tag{text: shortID(alloc.ID) + " │ ", style: style}

		id := alloc.ID
		cmds = append(cmds, func() tea.Msg {
			stream, err := client.Logs(context.Background(), namespace, id, task, source)

			return jobLogOpenedMsg{reading: reading, allocID: id, stream: stream, err: err}
		})
	}

	return p, tea.Batch(cmds...)
}

// reload reads the allocations again, to open the logs of the ones that
// run the task now.
func (p jobLogsPage) reload(client allocsClient) (jobLogsPage, tea.Cmd) {
	p.logs.stop()
	p.content = textContent{}

	reading := p.logs.reading

	return p, request(allocsOf(client, p.scope), func(allocs []nomad.Alloc) tea.Msg {
		return jobLogAllocsMsg{reading: reading, allocs: allocs}
	})
}

// take keeps what the logs of the reading on the screen say. A stream
// opened for another reading is not the page's: the session lets go of it.
// None of it is an answer to what the page asked.
func (p jobLogsPage) take(msg tea.Msg, e env) (page, outcome, bool) {
	switch msg := msg.(type) {
	case jobLogAllocsMsg:
		if msg.reading != p.logs.reading {
			return p, outcome{}, false
		}

		return p.reloaded(e.client, msg.allocs)

	case jobLogOpenedMsg:
		if msg.reading != p.logs.reading {
			return p, outcome{}, false
		}

		if msg.err != nil {
			return p.lines(msg.allocID, &styleError, "could not read: "+msg.err.Error()), outcome{reading: true}, true
		}

		// The streams are kept from the first one on, whatever came before.
		if p.logs.streams == nil {
			p.logs.streams = map[*nomad.LogStream]string{}
		}

		p.logs.streams[msg.stream] = msg.allocID

		return p, outcome{cmd: waitForStream(msg.stream), reading: true}, true

	case logLineMsg:
		allocID, ok := p.logs.streams[msg.stream]
		if !ok {
			return p, outcome{}, false
		}

		// What an allocation wrote goes at the end, behind its tag.
		return p.lines(allocID, nil, linesOf(msg.text)...), outcome{cmd: waitForStream(msg.stream), reading: true}, true

	case logEndMsg:
		allocID, ok := p.logs.streams[msg.stream]
		if !ok {
			return p, outcome{}, false
		}

		// A stream that ended is let go of, and says so: the others go on.
		delete(p.logs.streams, msg.stream)

		return p.lines(allocID, &styleMuted, "stopped"), outcome{reading: true}, true
	}

	return p, outcome{}, false
}

// reloaded opens the logs of the allocations read again.
func (p jobLogsPage) reloaded(client filesClient, allocs []nomad.Alloc) (page, outcome, bool) {
	for _, choice := range choices(p.scope, allocs) {
		if choice.task == p.task {
			p, cmd := p.read(client, choice.allocs)

			return p, outcome{cmd: cmd, reading: true}, true
		}
	}

	p, cmd := p.read(client, nil)
	warn := warnMsg(fmt.Sprintf("%s runs in no allocation now", p.task))

	return p, outcome{now: []tea.Msg{warn}, cmd: cmd, reading: true}, true
}

// lines adds lines of an allocation, all with the one time they arrived, in
// a style of their own when they are not something the task wrote.
func (p jobLogsPage) lines(allocID string, style *lipgloss.Style, lines ...string) jobLogsPage {
	label := p.logs.tags[allocID]
	p.content.add(lines, time.Now(), &label, style)

	return p
}

// jobLogsKeys are the keys of the logs of a job.
var jobLogsKeys = []pageKey[jobLogsPage]{
	{press: "r", label: "Reload", do: reloadJobLogs},
	// The key that opens stderr from the tasks switches to the other of
	// the two here, and says which one it goes to.
	{press: "ctrl+e", label: "Stderr", do: switchJobSource, offered: readsJobSource(nomad.LogStdout)},
	{press: "ctrl+e", label: "Stdout", do: switchJobSource, offered: readsJobSource(nomad.LogStderr)},
	followKey[jobLogsPage](),
	wrapKey[jobLogsPage](),
	timesKey[jobLogsPage](),
	saveKey[jobLogsPage](),
}

func (p jobLogsPage) keys(e env) []keyHint { return hintsOf(p, e, jobLogsKeys) }

func (p jobLogsPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, jobLogsKeys, k)
}

// readsJobSource says the logs read the source.
func readsJobSource(source string) func(p jobLogsPage, _ env) bool {
	return func(p jobLogsPage, _ env) bool { return p.source == source }
}

// reloadJobLogs reads the allocations again and opens the logs of the ones
// that run the task now: a deployment may have replaced them since.
func reloadJobLogs(p jobLogsPage, e env) (jobLogsPage, outcome) {
	p, cmd := p.reload(e.client)

	return p, then(requestMsg(cmd))
}

// switchJobSource reads the other of the two outputs, from the same
// allocations.
func switchJobSource(p jobLogsPage, e env) (jobLogsPage, outcome) {
	p.source = otherSource(p.source)
	p.content = textContent{}

	p, cmd := p.read(e.client, p.logs.all)

	return p, then(requestMsg(cmd))
}
