package ui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// mark marks the row under the cursor, or unmarks it. The cursor stays where
// it is: a mark is a toggle, and removing one must not need the cursor moved
// back to it.
func mark(m Model) (Model, tea.Cmd) {
	all := m.ids()

	at, ok := m.selectedIndex()
	if !ok || at >= len(all) {
		return m, nil
	}

	m.list = m.list.toggle(all[at])
	m.layout()

	return m, nil
}

// markAll marks every row of the screen, or unmarks them all when they are
// already marked.
func markAll(m Model) (Model, tea.Cmd) {
	ids := m.ids()
	if ids == nil {
		return m, nil
	}

	m.list = m.list.toggleAll(ids)
	m.layout()

	return m, nil
}

// ids name the rows of the open screen, nil for a screen whose rows cannot
// be marked.
func (m Model) ids() []string {
	if p, ok := m.screen.page.(marking); ok {
		return p.ids(m.env())
	}

	return nil
}

// allocLabel is what a question about allocations says: the one under the
// cursor by name, or how many were marked.
func allocLabel(allocs []nomad.Alloc) string {
	return many(len(allocs), "the allocation "+shortID(allocs[0].ID), "allocations")
}

// What names a resource of a screen, so that a mark belongs to the job, the
// allocation or the machine rather than to the row it sits on. The marks and
// the actions that use them read the same name, or a mark would stay after
// the thing it was put on is gone.
func jobMark(job nomad.Job) string { return job.Namespace + "/" + job.ID }

func allocMark(alloc nomad.Alloc) string { return alloc.ID }

func nodeMark(node nomad.Node) string { return node.ID }

func aclMark(object nomad.ACLObject) string { return object.ID }

func variableMark(v nomad.Variable) string { return v.Namespace + "/" + v.Path }

func namespaceMark(namespace nomad.Namespace) string { return namespace.Name }

func versionMark(version nomad.JobVersion) string { return strconv.FormatUint(version.Version, 10) }

func deploymentMark(d nomad.Deployment) string { return d.ID }

func instanceMark(instance nomad.ServiceInstance) string { return instance.ID }

func groupMark(group nomad.TaskGroup) string { return group.Name }

func taskMark(task nomad.Task) string { return task.Name }

func allocTaskMark(task allocTask) string { return task.allocID + "/" + task.name }

func names[T any](items []T, mark func(T) string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, mark(item))
	}

	return out
}

// many is how a question names what it is about: one of them by name, or
// how many of them there are.
func many(count int, one, several string) string {
	if count == 1 {
		return one
	}

	return sprintf("%d %s", count, several)
}
