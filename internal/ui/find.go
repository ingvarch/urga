package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// minFind is the shortest text the cluster searches for; it refuses shorter.
const minFind = 2

// find opens what a search of the whole cluster finds. A text too short for
// the cluster is not sent.
func (m Model) find(text string) (Model, tea.Cmd) {
	if len([]rune(text)) < minFind {
		return m.warn(fmt.Sprintf("Type at least %d characters to find.", minFind)), nil
	}

	return m.push(screen{page: findPage{text: text}})
}

// foundMsg is what a search found, or why it found nothing.
type foundMsg struct {
	text  string
	found nomad.Found
	err   error
}

// findTitles are the columns of what a search found: In is where it is, in
// the job it is part of, or the ID it has.
var findTitles = []string{"Type", "Name", "Namespace", "In"}

// findPage is what a search of the whole cluster found.
type findPage struct {
	text string

	// read says the cluster answered. A search is asked once: it is as the
	// cluster was when it was asked, and the screen is polled otherwise.
	read  bool
	found nomad.Found
}

func (p findPage) title(_ env, count int) string { return sprintf("Found %q [%d]", p.text, count) }

func (findPage) titles() []string { return findTitles }
func (findPage) topics() []string { return nil }

// fetch asks once. A failure is an answer too: shown once, not asked again
// every few seconds.
func (p findPage) fetch(e env) tea.Cmd {
	if p.read {
		return nil
	}

	client, text := e.client, p.text

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		found, err := client.Find(ctx, text)

		return foundMsg{text: text, found: found, err: err}
	}
}

func (p findPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	answer, ok := msg.(foundMsg)
	if !ok || answer.text != p.text {
		return p, outcome{}, false
	}

	p.read, p.found = true, answer.found

	switch {
	case answer.err != nil:
		return p, then(failMsg{err: answer.err}), true
	case len(answer.found.Truncated) > 0:
		return p, then(warnMsg(fmt.Sprintf("Not every %s is shown: type more to narrow the search.",
			strings.Join(answer.found.Truncated, " and ")))), true
	}

	return p, outcome{}, true
}

func (p findPage) rows(env) []tableRow {
	rows := make([]tableRow, 0, len(p.found.Matches))

	for _, m := range p.found.Matches {
		rows = append(rows, tableRow{cells: []string{m.Kind, m.Name, dashed(m.Namespace), whereIs(m)}})
	}

	return rows
}

// whereIs is where a match is: the job, group and task a part of a job is
// in, or the ID of what the cluster names by one.
func whereIs(m nomad.Match) string {
	switch m.Kind {
	case nomad.MatchGroup:
		return m.JobID
	case nomad.MatchTask:
		return placesOf(m.JobID, m.Group)
	case nomad.MatchService, nomad.MatchImage, nomad.MatchCommand:
		return placesOf(m.JobID, m.Group, m.Task)
	case nomad.MatchAlloc, nomad.MatchNode, nomad.MatchVolume, nomad.MatchHostVolume:
		return shortID(m.ID)
	}

	return ""
}

// placesOf joins the places a match is in, from the outside in.
func placesOf(places ...string) string {
	kept := []string{}

	for _, place := range places {
		if place != "" {
			kept = append(kept, place)
		}
	}

	return strings.Join(kept, " / ")
}

// picked is the match under the cursor.
func (p findPage) picked(e env) (nomad.Match, bool) { return pickedFrom(e, p.found.Matches) }

var findKeys = []pageKey[findPage]{
	{press: "enter", label: "Open", do: openFound, offered: func(p findPage, e env) bool {
		m, ok := p.picked(e)

		return ok && m.Kind != nomad.MatchPool && m.Kind != nomad.MatchClass
	}},
}

func (p findPage) keys(e env) []keyHint { return hintsOf(p, e, findKeys) }

func (p findPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, findKeys, k)
}

// openFound opens the match under the cursor where it can be acted on.
func openFound(p findPage, e env) (findPage, outcome) {
	m, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	return p, openMatch(e.client, m)
}

// openMatch opens a match where it can be acted on. What was found by its ID
// alone is read first: its namespace and its job are not known until then.
func openMatch(client Client, m nomad.Match) outcome {
	if opened := pageOf(m); opened != nil {
		return then(openMsg{opened})
	}

	switch m.Kind {
	case nomad.MatchNamespace:
		return then(switchNamespaceMsg(m.Name), openMsg{jobsPage{}})

	case nomad.MatchAlloc:
		return outcome{cmd: request(func(ctx context.Context) (nomad.Alloc, error) {
			return client.Allocation(ctx, m.Namespace, m.ID)
		}, func(alloc nomad.Alloc) tea.Msg { return openMsg{listedTasks(alloc)} })}

	case nomad.MatchDeployment:
		return outcome{cmd: request(func(ctx context.Context) (nomad.DeploymentDetail, error) {
			return client.Deployment(ctx, m.Namespace, m.ID)
		}, func(d nomad.DeploymentDetail) tea.Msg { return openMsg{deploymentOf(d.Deployment)} })}

	case nomad.MatchEval:
		return outcome{cmd: describeEvaluation(client, m.Namespace, m.ID)}
	}

	return outcome{}
}

// pageOf is the page a match opens on, when what it needs is known already.
func pageOf(m nomad.Match) page {
	switch m.Kind {
	case nomad.MatchJob:
		return jobsPage{only: nomad.Job{ID: m.ID, Namespace: m.Namespace}}
	case nomad.MatchGroup, nomad.MatchTask, nomad.MatchImage, nomad.MatchCommand:
		return allocationsPage{namespace: m.Namespace, jobID: m.JobID, group: m.Group}
	case nomad.MatchService:
		return serviceInstancesPage{namespace: m.Namespace, service: m.Name}
	case nomad.MatchNode:
		return clientOf(nomad.Node{ID: m.ID, Name: m.Name})
	case nomad.MatchVariable:
		return variablePage{namespace: m.Namespace, path: m.ID}
	case nomad.MatchVolume:
		return volumePage{namespace: m.Namespace, kind: nomad.VolumeCSI, id: m.ID, name: m.Name}
	case nomad.MatchHostVolume:
		return volumePage{namespace: m.Namespace, kind: nomad.VolumeHost, id: m.ID, name: m.Name}
	case nomad.MatchPlugin:
		return pluginPage{id: m.ID}
	}

	return nil
}
