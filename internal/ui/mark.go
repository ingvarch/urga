package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// markGlyph stands in the margin of a row that is marked, where the table
// keeps its distance from the border: the columns do not move for it.
const markGlyph = "•"

// mark takes the row under the cursor, or lets it go. The cursor stays where
// it is: a mark is a toggle, and taking one back must not need the cursor
// walked back to it.
func (m Model) mark() (Model, tea.Cmd) {
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

	if m.marks == nil {
		m.marks = map[string]bool{}
	}

	if m.marks[all[at]] {
		delete(m.marks, all[at])
	} else {
		m.marks[all[at]] = true
	}

	m.layout()

	return m, nil
}

// markAll takes every row of the screen, or lets them all go when they are
// already taken.
func (m Model) markAll() (Model, tea.Cmd) {
	ids := m.screen.of().ids
	if ids == nil {
		return m, nil
	}

	shown := m.shownIDs(ids(m))
	if len(shown) == 0 {
		return m, nil
	}

	// All of them already taken means let them go; otherwise take the rest.
	// Counting would clear a mark the filter is hiding.
	if m.allMarked(shown) {
		m.marks = nil
		m.layout()

		return m, nil
	}

	if m.marks == nil {
		m.marks = map[string]bool{}
	}

	for _, id := range shown {
		m.marks[id] = true
	}

	m.layout()

	return m, nil
}

// showMarks puts the mark of each resource on the row that shows it.
func (m Model) showMarks(rows []tableRow) {
	res := m.screen.of()
	if len(m.marks) == 0 || res.ids == nil {
		return
	}

	all := res.ids(m)

	for i := range rows {
		if i < len(m.index) && m.index[i] < len(all) {
			rows[i].marked = m.marks[all[m.index[i]]]
		}
	}
}

// allMarked says every row on the screen is taken.
func (m Model) allMarked(shown []string) bool {
	for _, id := range shown {
		if !m.marks[id] {
			return false
		}
	}

	return true
}

// shownIDs are the ids of the rows that are on the screen, which is what a
// filter narrows.
func (m Model) shownIDs(all []string) []string {
	out := make([]string, 0, len(m.index))

	for _, at := range m.index {
		if at < len(all) {
			out = append(out, all[at])
		}
	}

	return out
}

// marked are the resources that carry a mark. A mark is on the resource, not
// on the line it sits on, so a filter or a sort does not change what an
// action takes. Without a mark anywhere, what the cursor is on is the answer,
// which is how every action reads a list.
func marked[T any](m Model, kind screenKind, items []T) []T {
	res := m.screen.of()

	out := []T{}

	if len(m.marks) > 0 && res.ids != nil && m.screen.kind == kind {
		all := res.ids(m)

		for at := range items {
			if at < len(all) && m.marks[all[at]] {
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

// allocIDs name the allocations of the screen, so that a mark belongs to the
// allocation rather than to the row it sits on.
func allocIDs(m Model) []string {
	allocs := m.visibleAllocs()

	ids := make([]string, 0, len(allocs))
	for _, alloc := range allocs {
		ids = append(ids, alloc.ID)
	}

	return ids
}

// allocLabel is what a question about allocations says: the one under the
// cursor by name, or how many were marked.
func allocLabel(allocs []nomad.Alloc) string {
	if len(allocs) == 1 {
		return "the allocation " + shortID(allocs[0].ID)
	}

	return sprintf("%d allocations", len(allocs))
}
