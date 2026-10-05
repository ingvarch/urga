package ui

import (
	"context"
	"slices"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// partMsg is one list the overview asked for, with the namespace it was
// asked in, or why the cluster did not answer.
type partMsg[T any] struct {
	namespace string
	items     []T
	err       error
}

// part is one list as the overview holds it.
type part[T any] struct {
	partMsg[T]

	read bool
}

// counted says the list can be counted in the namespace. A list not read
// yet, or read in another namespace, is not known; one the cluster refused
// returns its error.
func (p part[T]) counted(namespace string) (bool, error) {
	if !p.read || p.namespace != namespace {
		return false, nil
	}

	return p.err == nil, p.err
}

type overviewRead struct {
	jobs        part[nomad.Job]
	allocs      part[nomad.Alloc]
	nodes       part[nomad.Node]
	evaluations part[nomad.Evaluation]
	deployments part[nomad.Deployment]
}

// overviewPage counts what needs attention in the namespace of the session.
type overviewPage struct {
	ofTheSession

	read overviewRead
}

// overviewRow is a line of the overview: a fault of one kind of resource,
// whether its list is known in a namespace, and the list as it opens, filled
// with what was read. A count is the rows of that list, so it cannot
// disagree with what the list shows.
type overviewRow struct {
	resource string
	fault    fault
	known    func(r overviewRead, namespace string) (bool, error)
	list     func(r overviewRead) page
}

var overviewTitles = []string{"Resource", "State", "Count"}

var overviewRows = slices.Concat(
	faultLines(jobLine, faultDead),
	faultLines(allocLine, faultFailedOrLost, faultRestarting, faultOOMKilled),
	faultLines(clientLine, faultDown, faultDisconnected, faultDraining, faultIneligible),
	faultLines(evaluationLine, faultBlocked, faultFailed),
	faultLines(deploymentLine, faultFailed, faultPaused, faultRunning),
)

func faultLines(line func(fault) overviewRow, faults ...fault) []overviewRow {
	rows := make([]overviewRow, 0, len(faults))
	for _, f := range faults {
		rows = append(rows, line(f))
	}

	return rows
}

func jobLine(f fault) overviewRow {
	return overviewRow{
		resource: "Jobs", fault: f,
		known: func(r overviewRead, namespace string) (bool, error) { return r.jobs.counted(namespace) },
		list:  func(r overviewRead) page { return jobsPage{jobs: r.jobs.items, fault: f} },
	}
}

func allocLine(f fault) overviewRow {
	return overviewRow{
		resource: "Allocations", fault: f,
		known: func(r overviewRead, namespace string) (bool, error) { return r.allocs.counted(namespace) },
		list:  func(r overviewRead) page { return allocationsPage{allocs: r.allocs.items, fault: f} },
	}
}

// A client belongs to no namespace: it is known in every one.
func clientLine(f fault) overviewRow {
	return overviewRow{
		resource: "Clients", fault: f,
		known: func(r overviewRead, _ string) (bool, error) { return r.nodes.counted("") },
		list:  func(r overviewRead) page { return nodesPage{nodes: r.nodes.items, fault: f} },
	}
}

func evaluationLine(f fault) overviewRow {
	return overviewRow{
		resource: "Evaluations", fault: f,
		known: func(r overviewRead, namespace string) (bool, error) { return r.evaluations.counted(namespace) },
		list:  func(r overviewRead) page { return evaluationsPage{evaluations: r.evaluations.items, fault: f} },
	}
}

func deploymentLine(f fault) overviewRow {
	return overviewRow{
		resource: "Deployments", fault: f,
		known: func(r overviewRead, namespace string) (bool, error) { return r.deployments.counted(namespace) },
		list:  func(r overviewRead) page { return deploymentsPage{deployments: r.deployments.items, fault: f} },
	}
}

func (overviewPage) title(e env, _ int) string {
	return sprintf("Overview (%s)", namespaceLabel(e.namespace))
}

func (overviewPage) titles() []string { return overviewTitles }

func (overviewPage) topics() []string {
	return []string{
		nomad.TopicJob, nomad.TopicAllocation, nomad.TopicNode, nomad.TopicEvaluation, nomad.TopicDeployment,
	}
}

// readPart asks for one list. A refusal is an answer too: it shows in its
// lines, and the other lines stay.
func readPart[T any](namespace string, load func(ctx context.Context) ([]T, error)) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		items, err := load(ctx)

		return partMsg[T]{namespace: namespace, items: items, err: err}
	}
}

func (overviewPage) fetch(e env) tea.Cmd {
	client, namespace := e.client, e.namespace

	return tea.Batch(
		readPart(namespace, func(ctx context.Context) ([]nomad.Job, error) { return client.Jobs(ctx, namespace) }),
		readPart(namespace, func(ctx context.Context) ([]nomad.Alloc, error) {
			return client.Allocations(ctx, namespace, "")
		}),
		readPart("", client.Nodes),
		readPart(namespace, func(ctx context.Context) ([]nomad.Evaluation, error) {
			return client.Evaluations(ctx, namespace)
		}),
		readPart(namespace, func(ctx context.Context) ([]nomad.Deployment, error) {
			return client.Deployments(ctx, namespace)
		}),
	)
}

func (p overviewPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	switch msg := msg.(type) {
	case partMsg[nomad.Job]:
		p.read.jobs = part[nomad.Job]{partMsg: msg, read: true}
	case partMsg[nomad.Alloc]:
		p.read.allocs = part[nomad.Alloc]{partMsg: msg, read: true}
	case partMsg[nomad.Node]:
		p.read.nodes = part[nomad.Node]{partMsg: msg, read: true}
	case partMsg[nomad.Evaluation]:
		p.read.evaluations = part[nomad.Evaluation]{partMsg: msg, read: true}
	case partMsg[nomad.Deployment]:
		p.read.deployments = part[nomad.Deployment]{partMsg: msg, read: true}
	default:
		return p, outcome{}, false
	}

	return p, outcome{}, true
}

// listed are the rows of the line's list, with whether the count is known
// in the session namespace and the error the cluster refused with.
func (line overviewRow) listed(r overviewRead, e env) ([]tableRow, bool, error) {
	counted, err := line.known(r, e.namespace)
	if !counted {
		return nil, false, err
	}

	return line.list(r).rows(e), true, nil
}

// rows are a line for each fault. A count takes the color of the rows it
// opens; one that is zero, not read or refused is grey.
func (p overviewPage) rows(e env) []tableRow {
	rows := make([]tableRow, 0, len(overviewRows))

	for _, line := range overviewRows {
		row := tableRow{cells: []string{line.resource, line.fault.String()}, color: colorSpent}

		listed, counted, err := line.listed(p.read, e)

		switch {
		case err != nil:
			row.cells = append(row.cells, notGiven(err))
		case !counted:
			row.cells = append(row.cells, "n/a")
		default:
			row.cells = append(row.cells, strconv.Itoa(len(listed)))

			if len(listed) > 0 {
				row.color = listed[0].color
			}
		}

		rows = append(rows, row)
	}

	return rows
}

// ids keep the cursor on its line when a sort moves the lines.
func (overviewPage) ids(env) []string {
	ids := make([]string, 0, len(overviewRows))
	for _, line := range overviewRows {
		ids = append(ids, line.resource+"/"+line.fault.String())
	}

	return ids
}

var overviewKeys = []pageKey[overviewPage]{
	{press: "enter", label: "Open", do: openFaultList, offered: countsSomething},
}

func (p overviewPage) keys(e env) []keyHint { return hintsOf(p, e, overviewKeys) }

func (p overviewPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, overviewKeys, k)
}

// countsSomething says the line under the cursor has a count above zero:
// only then is there a list to open.
func countsSomething(p overviewPage, e env) bool {
	line, ok := pickedFrom(e, overviewRows)
	if !ok {
		return false
	}

	listed, _, _ := line.listed(p.read, e)

	return len(listed) > 0
}

// openFaultList opens the list the line counts, filled with what the
// overview read.
func openFaultList(p overviewPage, e env) (overviewPage, outcome) {
	line, ok := pickedFrom(e, overviewRows)
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg{line.list(p.read)})
}
