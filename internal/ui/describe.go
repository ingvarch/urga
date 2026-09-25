package ui

import (
	"context"
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// describeMsg is a description the cluster answered with.
type describeMsg struct {
	label   string
	content string

	// lines are a description urga drew, some of it in a colour of its
	// own; it takes the place of content.
	lines []paintedLine
}

// The screens that can be described each request what the cursor is on, as
// the cluster returns it. With nothing under the cursor, nothing is sent.

func describeJob(p jobsPage, e env) (jobsPage, outcome) {
	job, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	client := e.client

	return p, outcome{cmd: describe(fmt.Sprintf("Job: %s", job.ID), func(ctx context.Context) (string, error) {
		return client.DescribeJob(ctx, job.Namespace, job.ID)
	})}
}

func describeDeployment(p deploymentsPage, e env) (deploymentsPage, outcome) {
	deployment, ok := p.inView(e)
	if !ok {
		return p, outcome{}
	}

	client := e.client

	return p, outcome{cmd: describe(fmt.Sprintf("Deployment: %s", shortID(deployment.ID)), func(ctx context.Context) (string, error) {
		return client.DescribeDeployment(ctx, deployment.Namespace, deployment.ID)
	})}
}

// showJobSpec asks for the file the job under the cursor was submitted with.
func showJobSpec(p jobsPage, e env) (jobsPage, outcome) {
	job, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	client := e.client

	return p, outcome{cmd: describe(fmt.Sprintf("Job spec: %s", job.ID), func(ctx context.Context) (string, error) {
		spec, err := client.JobSpec(ctx, job.Namespace, job.ID)

		// The cluster has no file to show. The screen writes the message;
		// the client returns only an error.
		if errors.Is(err, nomad.ErrNoSource) {
			return fmt.Sprintf(
				"The cluster kept no source for %s.\n\n"+
					"It was registered before submissions were stored, or through the API\n"+
					"without one. <e> edits what the cluster does have of it.", job.ID), nil
		}

		return spec.Source, err
	})}
}

func describe(label string, load func(ctx context.Context) (string, error)) tea.Cmd {
	return request(load, func(content string) tea.Msg {
		return describeMsg{label: label, content: content}
	})
}

// showDescribe shows a description on top of the list it was requested
// from.
func (m Model) showDescribe(msg describeMsg) (Model, tea.Cmd) {
	text := newTextModel(msg.content)
	if msg.lines != nil {
		text = paintedText(msg.lines)
	}

	return m.push(screen{page: describePage{label: msg.label, content: text.textContent}})
}

// describePage is a description: what the cluster says of a resource, or
// what urga drew of one, as text to read.
type describePage struct {
	noAnswers

	label   string
	content textContent
}

var describeKeys = textKeys[describePage]()

func (p describePage) title(env, int) string    { return p.label }
func (describePage) titles() []string           { return nil }
func (describePage) topics() []string           { return nil }
func (describePage) fetch(env) tea.Cmd          { return nil }
func (describePage) rows(env) []tableRow        { return nil }
func (p describePage) text(env) textContent     { return p.content }
func (p describePage) saveAs() (string, string) { return p.label, "txt" }
func (p describePage) keys(e env) []keyHint     { return hintsOf(p, e, describeKeys) }

func (p describePage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, describeKeys, k)
}

// describeLines is describe for a description urga draws in colour.
func describeLines(label string, load func(ctx context.Context) ([]paintedLine, error)) tea.Cmd {
	return request(load, func(lines []paintedLine) tea.Msg {
		return describeMsg{label: label, lines: lines}
	})
}
