package ui

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/ingvarch/urga/internal/nomad"
)

var deploymentTitles = []string{"ID", "JobID", "Namespace", "Version", "Status", "Description"}

func deploymentRows(deployments []nomad.Deployment) []tableRow {
	rows := make([]tableRow, 0, len(deployments))

	for _, d := range deployments {
		rows = append(rows, tableRow{
			cells: []string{
				shortID(d.ID),
				d.JobID,
				d.Namespace,
				fmt.Sprintf("%d", d.JobVersion),
				d.Status,
				d.StatusDescription,
			},
			color: deploymentColor(d),
		})
	}

	return rows
}

func deploymentColor(d nomad.Deployment) color.Color {
	switch d.Status {
	case "running":
		return colorPending
	case "failed", "cancelled":
		return colorDead
	case "successful":
		return nil
	}

	return nil
}

var namespaceTitles = []string{"Name", "Quota", "Description"}

func namespaceRows(namespaces []nomad.Namespace) []tableRow {
	rows := make([]tableRow, 0, len(namespaces))

	for _, n := range namespaces {
		rows = append(rows, tableRow{cells: []string{n.Name, n.Quota, n.Description}})
	}

	return rows
}

var serviceTitles = []string{"Name", "Namespace", "Tags"}

func serviceRows(services []nomad.Service) []tableRow {
	rows := make([]tableRow, 0, len(services))

	for _, s := range services {
		rows = append(rows, tableRow{cells: []string{s.Name, s.Namespace, strings.Join(s.Tags, ", ")}})
	}

	return rows
}

var evaluationTitles = []string{"ID", "JobID", "Namespace", "Type", "TriggeredBy", "Status", "Age"}

func evaluationRows(evals []nomad.Evaluation) []tableRow {
	rows := make([]tableRow, 0, len(evals))

	for _, e := range evals {
		rows = append(rows, tableRow{
			cells: []string{
				shortID(e.ID),
				e.JobID,
				e.Namespace,
				e.Type,
				e.TriggeredBy,
				e.Status,
				ageOf(e.Created),
			},
			color: evaluationColor(e),
		})
	}

	return rows
}

func evaluationColor(e nomad.Evaluation) color.Color {
	switch e.Status {
	case "pending", "blocked":
		return colorPending
	case "failed", "canceled":
		return colorDead
	}

	return nil
}

var nodeTitles = []string{"ID", "Name", "Datacenter", "Pool", "Version", "Status", "Eligibility", "Drain", "Address"}

func nodeRows(nodes []nomad.Node) []tableRow {
	rows := make([]tableRow, 0, len(nodes))

	for _, n := range nodes {
		rows = append(rows, tableRow{
			cells: []string{
				shortID(n.ID),
				n.Name,
				n.Datacenter,
				n.NodePool,
				n.Version,
				n.Status,
				n.Eligibility,
				fmt.Sprintf("%t", n.Drain),
				n.Address,
			},
			color: nodeColor(n),
		})
	}

	return rows
}

// nodeColor marks a node that takes no work: down, draining or held back.
func nodeColor(n nomad.Node) color.Color {
	switch {
	case n.Status != "ready":
		return colorDead
	case n.Drain, n.Eligibility != "eligible":
		return colorAttention
	}

	return nil
}

var variableTitles = []string{"Path", "Namespace", "Age", "Modified"}

func variableRows(variables []nomad.Variable) []tableRow {
	rows := make([]tableRow, 0, len(variables))

	for _, v := range variables {
		rows = append(rows, tableRow{
			cells: []string{v.Path, v.Namespace, ageOf(v.Created), ageOf(v.Modified)},
		})
	}

	return rows
}

var nodePoolTitles = []string{"Name", "Scheduler", "Description"}

func nodePoolRows(pools []nomad.NodePool) []tableRow {
	rows := make([]tableRow, 0, len(pools))

	for _, p := range pools {
		rows = append(rows, tableRow{cells: []string{p.Name, p.Scheduler, p.Description}})
	}

	return rows
}
