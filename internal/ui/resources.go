package ui

import (
	"fmt"
	"image/color"
	"strconv"
	"time"

	"github.com/ingvarch/urga/internal/nomad"
)

// serverTitles are the columns of the server list: Contact is how long ago
// the server heard from the leader, Behind how many raft entries it lacks.
var serverTitles = []string{"Name", "Address", "Port", "Datacenter", "Region", "Version", "Status", "Health", "Voter", "Contact", "Behind", ""}

// serverRows are the servers, with how each stands when the health of the
// cluster is known.
func serverRows(servers []nomad.Server, health nomad.ClusterHealth, known bool) []tableRow {
	rows := make([]tableRow, 0, len(servers))

	for _, s := range servers {
		leader := ""
		if s.Leader {
			leader = "leader"
		}

		standing, found := standingOf(s, health)
		found = found && known

		cells := []string{s.Name, s.Address, strconv.Itoa(s.Port), s.Datacenter, s.Region, s.Version, s.Status}
		cells = append(cells, standingCells(standing, found, leaderIndex(health))...)

		row := tableRow{cells: append(cells, leader), color: serverColor(s)}
		if found && !standing.Healthy && s.Status == "alive" {
			row.color = colorDead
		}

		rows = append(rows, row)
	}

	return rows
}

// standingCells are how a server stands: healthy, voter, how long ago it
// heard from the leader and how far behind it is. The leader is not behind
// itself. A dash each when it is not known.
func standingCells(s nomad.ServerHealth, found bool, leaderAt uint64) []string {
	if !found {
		return []string{"-", "-", "-", "-"}
	}

	contact, behind := "-", "-"
	if !s.Leader {
		contact = shortDuration(s.LastContact)
		behind = strconv.FormatUint(entriesBehind(s, leaderAt), 10)
	}

	return []string{healthWord(s.Healthy), yesNo(s.Voter), contact, behind}
}

// standingOf is the health of a server, found by its name or the address
// it takes calls on.
func standingOf(server nomad.Server, health nomad.ClusterHealth) (nomad.ServerHealth, bool) {
	for _, s := range health.Servers {
		if s.Name == server.Name || (s.Address != "" && s.Address == server.RPCAddress) {
			return s, true
		}
	}

	return nomad.ServerHealth{}, false
}

// leaderIndex is the last raft entry of the leader, zero with none known.
func leaderIndex(health nomad.ClusterHealth) uint64 {
	for _, s := range health.Servers {
		if s.Leader {
			return s.LastIndex
		}
	}

	return 0
}

// entriesBehind is how many raft entries a server lacks. One the leader has
// not caught up with yet is not ahead of it.
func entriesBehind(s nomad.ServerHealth, leaderAt uint64) uint64 {
	if s.LastIndex >= leaderAt {
		return 0
	}

	return leaderAt - s.LastIndex
}

func healthWord(healthy bool) string {
	if healthy {
		return "healthy"
	}

	return "unhealthy"
}

// shortDuration is a short time as it reads: milliseconds under a second,
// the unit of an age above.
func shortDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}

	return age(d)
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
