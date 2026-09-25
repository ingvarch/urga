package ui

import (
	"fmt"
	"image/color"
	"strconv"

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
