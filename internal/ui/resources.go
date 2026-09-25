package ui

import (
	"fmt"
	"image/color"
	"strconv"
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

var nodeTitles = []string{"ID", "Name", "Datacenter", "Pool", "Version", "Status", "Eligibility", "Drain", "CPU", "MEM", "Address"}

func nodeRows(nodes []nomad.Node, usage map[string]nomad.ResourceUse) []tableRow {
	rows := make([]tableRow, 0, len(nodes))

	for _, n := range nodes {
		use, known := usage[n.ID]

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
				percentCell(use.CPUPercent, known),
				percentCell(use.MemoryPercent, known),
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

var serverTitles = []string{"Name", "Address", "Port", "Datacenter", "Region", "Version", "Status", ""}

func serverRows(servers []nomad.Server) []tableRow {
	rows := make([]tableRow, 0, len(servers))

	for _, s := range servers {
		leader := ""
		if s.Leader {
			leader = "leader"
		}

		rows = append(rows, tableRow{
			cells: []string{
				s.Name,
				s.Address,
				strconv.Itoa(s.Port),
				s.Datacenter,
				s.Region,
				s.Version,
				s.Status,
				leader,
			},
			color: serverColor(s),
		})
	}

	return rows
}

// serverColor marks the one that leads, and any that is not answering.
func serverColor(s nomad.Server) color.Color {
	if s.Status != "alive" {
		return colorDead
	}

	if s.Leader {
		return colorTitle
	}

	return nil
}

var variableTitles = []string{"Path", "Namespace", "Lock", "Age", "Modified"}

func variableRows(variables []nomad.Variable) []tableRow {
	rows := make([]tableRow, 0, len(variables))

	for _, v := range variables {
		rows = append(rows, tableRow{
			cells: []string{v.Path, v.Namespace, lockOf(v), ageOf(v.Created), ageOf(v.Modified)},
		})
	}

	return rows
}

// lockOf names who holds a variable as a lock, by the ID of the lock: Nomad
// knows the holder by nothing else.
func lockOf(v nomad.Variable) string {
	if v.Lock == nil {
		return ""
	}

	return shortID(v.Lock.ID)
}

var nodePoolTitles = []string{"Name", "Scheduler", "Description"}

func nodePoolRows(pools []nomad.NodePool) []tableRow {
	rows := make([]tableRow, 0, len(pools))

	for _, p := range pools {
		rows = append(rows, tableRow{cells: []string{p.Name, p.Scheduler, p.Description}})
	}

	return rows
}
