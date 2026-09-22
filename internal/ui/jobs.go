package ui

import (
	"fmt"
	"image/color"

	"github.com/ingvarch/urga/internal/nomad"
)

// jobTitles are the columns of the job list.
var jobTitles = []string{"ID", "Name", "Type", "Namespace", "Status", "Allocs", "Age"}

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

// jobRows is one row per job, in the order of the columns.
func jobRows(jobs []nomad.Job) []tableRow {
	rows := make([]tableRow, 0, len(jobs))

	for _, job := range jobs {
		rows = append(rows, tableRow{
			cells: []string{
				job.ID,
				job.Name,
				job.Type,
				job.Namespace,
				job.Status,
				fmt.Sprintf("%d/%d", job.Running, job.Desired),
				ageOf(job.SubmitTime),
			},
			color: jobColor(job),
		})
	}

	return rows
}

// jobColor says what a job is up to without reading the row: a service short
// of allocations stands out, a dead one is red, a batch job that ended is
// spent rather than broken.
func jobColor(job nomad.Job) color.Color {
	switch job.Status {
	case statusRunning:
		if job.Type == typeService && job.Running != job.Desired {
			return colorAttention
		}
	case statusPending:
		return colorPending
	case statusDead, statusFailed:
		if job.Type == typeBatch {
			return colorSpent
		}

		return colorDead
	}

	return nil
}

const (
	statusRunning = "running"
	statusPending = "pending"
	statusDead    = "dead"
	statusFailed  = "failed"

	typeService = "service"
	typeBatch   = "batch"
)
