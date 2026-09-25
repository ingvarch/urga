package ui

// list is how the rows of a screen are read: the table they are drawn in,
// the filter and the order they are read in, which resource each row shows,
// and which of them are marked.
type list struct {
	table tableModel

	// filter is what the rows are narrowed to.
	filter string

	// index maps a row of the table back to the resource it came from, which
	// the filter and the sort order shift.
	index []int

	// sort is the column the list is ordered by.
	sort sortState

	// marks are the resources of the open screen that an action is to take,
	// by the ids the screen names them with.
	marks map[string]bool

	// shown and held are how many rows are on the screen out of how many
	// the cluster answered with.
	shown int
	held  int
}

// newList is a list with these columns and nothing read into it yet.
func newList(titles []string) list {
	return list{table: newTableModel(titles), sort: newSortState()}
}

// read puts rows on the table the way the list is read: narrowed by the
// filter, and by trouble when only that is wanted, in its order, with the
// marks on the rows of the resources that carry one. ids name the resource
// of each row, nil for a list without marks.
func (l list) read(all []tableRow, titles, ids []string, troubled bool) list {
	l.held = len(all)

	rows, index := filterRows(all, l.filter)

	if troubled {
		rows, index = troubledRows(rows, index)
	}

	rows, index = sortRows(rows, index, l.sort, titles)
	l.shown = len(rows)
	l.index = index

	// The mark of each resource goes on the row that shows it.
	if len(l.marks) > 0 && ids != nil {
		for i := range rows {
			if i < len(index) && index[i] < len(ids) {
				rows[i].marked = l.marks[ids[index[i]]]
			}
		}
	}

	l.table.show(rows, l.sort)

	return l
}

// selected is the resource the cursor is on. The filter shifts the rows, so
// the row number is not the number of the resource.
func (l list) selected() (int, bool) {
	if l.table.cursor < 0 || l.table.cursor >= len(l.index) {
		return 0, false
	}

	return l.index[l.table.cursor], true
}

// toggle takes a resource, or lets it go. A list is a value: the marks are
// copied, so a list kept elsewhere keeps its own.
func (l list) toggle(id string) list {
	marks := make(map[string]bool, len(l.marks)+1)
	for k := range l.marks {
		marks[k] = true
	}

	if marks[id] {
		delete(marks, id)
	} else {
		marks[id] = true
	}

	l.marks = marks

	return l
}

// toggleAll takes every resource on the screen, or lets them all go when
// they are already taken. ids name the resource of each row.
func (l list) toggleAll(ids []string) list {
	shown := make([]string, 0, len(l.index))
	for _, at := range l.index {
		if at < len(ids) {
			shown = append(shown, ids[at])
		}
	}

	if len(shown) == 0 {
		return l
	}

	// All of them already taken means let them go; otherwise take the rest.
	// Counting would clear a mark the filter is hiding.
	all := true
	for _, id := range shown {
		all = all && l.marks[id]
	}

	if all {
		l.marks = nil

		return l
	}

	marks := make(map[string]bool, len(l.marks)+len(shown))
	for k := range l.marks {
		marks[k] = true
	}

	for _, id := range shown {
		marks[id] = true
	}

	l.marks = marks

	return l
}
