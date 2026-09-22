package ui

import (
	"fmt"

	"charm.land/bubbles/v2/table"

	"github.com/ingvarch/urga/internal/nomad"
)

// cellPadding is what the table puts around a cell, one space on each side.
const cellPadding = 2

// jobsTitle labels the list with the namespace it shows and how many rows
// are in it.
func jobsTitle(namespace string, count int) string {
	return fmt.Sprintf("Jobs (%s) [%d]", namespaceLabel(namespace), count)
}

// namespaceLabel is the namespace as it reads in a title.
func namespaceLabel(namespace string) string {
	if namespace == "" || namespace == nomad.AllNamespaces {
		return "all"
	}

	return namespace
}

// jobColumns lays the columns out over the width available. The ID takes
// what the fixed ones leave.
func jobColumns(width int) []table.Column {
	fixed := []table.Column{
		{Title: "Name", Width: 20},
		{Title: "Type", Width: 8},
		{Title: "Namespace", Width: 14},
		{Title: "Status", Width: 9},
		{Title: "Allocs", Width: 7},
		{Title: "Age", Width: 5},
	}

	taken := 0
	for _, column := range fixed {
		taken += column.Width + cellPadding
	}

	id := width - taken - cellPadding
	if id < minColumnWidth {
		id = minColumnWidth
	}

	// The last column swallows what is left over, so the row ends where the
	// border does.
	columns := append([]table.Column{{Title: "ID", Width: id}}, fixed...)

	return fitColumns(columns, width)
}

const minColumnWidth = 1

// fitColumns shrinks the columns from the right until the row fits, and gives
// what is left to the last one, so the total is exactly the width.
func fitColumns(columns []table.Column, width int) []table.Column {
	total := 0
	for _, column := range columns {
		total += column.Width + cellPadding
	}

	for i := len(columns) - 1; i >= 0 && total > width; i-- {
		over := total - width
		room := columns[i].Width - minColumnWidth

		cut := min(over, room)

		columns[i].Width -= cut
		total -= cut
	}

	if total < width {
		last := len(columns) - 1
		columns[last].Width += width - total
	}

	return columns
}

// jobRows is one row per job, in the order of the columns.
func jobRows(jobs []nomad.Job) []table.Row {
	rows := make([]table.Row, 0, len(jobs))

	for _, job := range jobs {
		rows = append(rows, table.Row{
			job.ID,
			job.Name,
			job.Type,
			job.Namespace,
			job.Status,
			fmt.Sprintf("%d/%d", job.Running, job.Desired),
			ageOf(job.SubmitTime),
		})
	}

	return rows
}
