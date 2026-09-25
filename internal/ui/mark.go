package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// mark takes the row under the cursor, or lets it go. The cursor stays where
// it is: a mark is a toggle, and taking one back must not need the cursor
// walked back to it.
func mark(m Model) (Model, tea.Cmd) {
	ids := m.screen.of().ids
	if ids == nil {
		return m, nil
	}

	at, ok := m.selectedIndex()
	if !ok {
		return m, nil
	}

	all := ids(m)
	if at >= len(all) {
		return m, nil
	}

	m.list = m.list.toggle(all[at])
	m.layout()

	return m, nil
}

// markAll takes every row of the screen, or lets them all go when they are
// already taken.
func markAll(m Model) (Model, tea.Cmd) {
	ids := m.screen.of().ids
	if ids == nil {
		return m, nil
	}

	m.list = m.list.toggleAll(ids(m))
	m.layout()

	return m, nil
}

// marked are the resources that carry a mark. A mark is on the resource, not
// on the line it sits on, so a filter or a sort does not change what an
// action takes. Without a mark anywhere, what the cursor is on is the answer,
// which is how every action reads a list.
func marked[T any](m Model, kind screenKind, items []T) []T {
	res := m.screen.of()

	out := []T{}

	if len(m.list.marks) > 0 && res.ids != nil && m.screen.kind == kind {
		all := res.ids(m)

		for at := range items {
			if at < len(all) && m.list.marks[all[at]] {
				out = append(out, items[at])
			}
		}
	}

	// Marks that name nothing on this screen any more leave the cursor to
	// answer, rather than the key doing nothing at all.
	if len(out) > 0 {
		return out
	}

	one, ok := selectedOf(m, kind, items)
	if !ok {
		return nil
	}

	return []T{one}
}

// allocIDs name the allocations of the screen.
func allocIDs(m Model) []string {
	return names(m.visibleAllocs(), allocMark)
}

// allocLabel is what a question about allocations says: the one under the
// cursor by name, or how many were marked.
func allocLabel(allocs []nomad.Alloc) string {
	return many(len(allocs), "the allocation "+shortID(allocs[0].ID), "allocations")
}

// What names a resource of a screen, so that a mark belongs to the job, the
// allocation or the machine rather than to the row it sits on. The marks and
// the actions that take them read the same name, or a mark would outlive the
// thing it was put on.
func jobMark(job nomad.Job) string { return job.Namespace + "/" + job.ID }

func allocMark(alloc nomad.Alloc) string { return alloc.ID }

func nodeMark(node nomad.Node) string { return node.ID }

func jobIDs(m Model) []string {
	return names(m.jobs, jobMark)
}

func nodeIDs(m Model) []string {
	return names(m.nodes, nodeMark)
}

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
