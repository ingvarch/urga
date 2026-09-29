package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// stopped offers a key for a job under the cursor that is dead.
func stopped(p jobsPage, e env) bool {
	job, ok := p.picked(e)

	return ok && job.Status == statusDead
}

// purgeJobs takes the stopped ones of the marked jobs, or the one under the
// cursor, out of the cluster, after the user confirms. A job that runs is
// left as it is: purging it would stop it too.
func purgeJobs(p jobsPage, e env) (jobsPage, outcome) {
	jobs := keep(markedFrom(e, p.visible(e), jobMark), func(job nomad.Job) bool { return job.Status == statusDead })
	if len(jobs) == 0 {
		return p, outcome{}
	}

	client, label := e.client, jobLabel(jobs)

	return p, then(askMsg{
		question: sprintf("Really purge %s? %s", label,
			byCount(len(jobs), "Its versions and history go with it.", "Their versions and history go with them.")),
		apply: each("Purged", label, jobs, jobMark, func(ctx context.Context, job nomad.Job) error {
			return client.PurgeJob(ctx, job.Namespace, job.ID)
		}),
	})
}

// isDown and isUp say whether the cluster hears from a client.
func isDown(node nomad.Node) bool { return node.Status == stateDown }
func isUp(node nomad.Node) bool   { return !isDown(node) }

// clientIs offers a key for a client under the cursor that fits.
func clientIs(fits func(nomad.Node) bool) func(nodesPage, env) bool {
	return func(p nodesPage, e env) bool {
		node, ok := pickedFrom(e, p.visible(e))

		return ok && fits(node)
	}
}

// purgeNodes takes the down ones of the marked clients, or the one under the
// cursor, out of the cluster, after the user confirms.
func purgeNodes(p nodesPage, e env) (nodesPage, outcome) {
	nodes := keep(markedFrom(e, p.visible(e), nodeMark), isDown)
	if len(nodes) == 0 {
		return p, outcome{}
	}

	client, label := e.client, nodeLabel(nodes)

	return p, then(askMsg{
		question: sprintf("Really purge %s? The cluster forgets %s until %s again.", label,
			byCount(len(nodes), "it", "them"), byCount(len(nodes), "it registers", "they register")),
		apply: each("Purged", label, nodes, nodeMark, func(ctx context.Context, node nomad.Node) error {
			return client.PurgeNode(ctx, node.ID)
		}),
	})
}

// collectNodes deletes what the clients that run, of the marked ones or the
// one under the cursor, keep of their allocations that ended, after the user
// confirms. One that is down answers nothing.
func collectNodes(p nodesPage, e env) (nodesPage, outcome) {
	nodes := keep(markedFrom(e, p.visible(e), nodeMark), isUp)
	if len(nodes) == 0 {
		return p, outcome{}
	}

	client, label := e.client, nodeLabel(nodes)

	return p, then(askMsg{
		question: sprintf("Really collect the garbage of %s? Logs of %s ended allocations are deleted.",
			label, byCount(len(nodes), "its", "their")),
		apply: each("Collected the garbage of", label, nodes, nodeMark, func(ctx context.Context, node nomad.Node) error {
			return client.CollectNode(ctx, node.ID)
		}),
	})
}

// collectGarbage makes the servers forget now what they would forget later,
// after the user confirms. It changes the cluster: read-only mode refuses
// it.
func (m Model) collectGarbage() (Model, tea.Cmd) {
	if m.opts.ReadOnly {
		return m.warn("Read-only: :gc changes the cluster."), nil
	}

	return m.ask("Really collect the garbage of the cluster? Dead jobs, ended allocations and down clients go now.",
		act("Garbage of the cluster collected.", m.client.CollectGarbage))
}

// byCount is what a sentence says of one, or of several.
func byCount(count int, one, several string) string {
	if count == 1 {
		return one
	}

	return several
}
