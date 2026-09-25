package ui

import (
	"image/color"
	"strconv"

	"github.com/ingvarch/urga/internal/nomad"
)

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
