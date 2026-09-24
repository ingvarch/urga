package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ingvarch/urga/internal/nomad"
)

// deploymentMsg is a deployment read in full.
type deploymentMsg nomad.DeploymentDetail

// deploymentAllocTitles are the columns of the allocations of a deployment:
// the job and the namespace are the deployment's, and what it made of each
// allocation is what the screen is read for.
var deploymentAllocTitles = []string{"ID", "TaskGroup", "Node", "Status", "Canary", "Health", "CPU", "MEM", "Age"}

// groupTitles are the columns of the groups of a deployment.
var groupTitles = []string{"Group", "Desired", "Placed", "Healthy", "Unhealthy", "Canaries", "Promoted", "Auto Revert", "Deadline", "Progress By"}

// openDeployment opens the deployment under the cursor: how far it got with
// each group, over the allocations it placed.
func openDeployment(m Model) (Model, tea.Cmd) {
	deployment, ok := selectedOf(m, screenDeployments, m.deployments)
	if !ok {
		return m, nil
	}

	return m.push(screen{
		kind:         screenAllocations,
		namespace:    deployment.Namespace,
		jobID:        deployment.JobID,
		deploymentID: deployment.ID,
	})
}

// fetchDeployment reads the deployment of the screen and the allocations it
// placed.
func fetchDeployment(m Model) tea.Cmd {
	client, s := m.client, m.screen

	return tea.Batch(
		fetchList(func(ctx context.Context) ([]nomad.Alloc, error) {
			return client.DeploymentAllocations(ctx, s.namespace, s.deploymentID)
		}, func(items []nomad.Alloc) tea.Msg { return allocsMsg(items) }),
		request(func(ctx context.Context) (nomad.DeploymentDetail, error) {
			return client.Deployment(ctx, s.namespace, s.deploymentID)
		}, func(d nomad.DeploymentDetail) tea.Msg { return deploymentMsg(d) }),
	)
}

// keepDeployment keeps what the deployment of the screen says about itself.
func (m Model) keepDeployment(msg deploymentMsg) (Model, tea.Cmd) {
	ours := m.screen.isDeployment() && msg.ID == m.screen.deploymentID

	return m.applyWhen(ours, func(m *Model) { m.deployment = nomad.DeploymentDetail(msg) })
}

// deploymentScreenPanel is the panel of the deployment the screen read.
// Until it has, there is none.
func (m Model) deploymentScreenPanel(width int) []string {
	if m.deployment.ID != m.screen.deploymentID {
		return nil
	}

	return deploymentPanel(m.deployment, width, m.rowsForPanel())
}

// deploymentPanel is what the allocations of a deployment show above them:
// what the deployment is, and how far it got with each group.
func deploymentPanel(d nomad.DeploymentDetail, width, room int) []string {
	head := []string{}

	if row := fieldLine([]field{{"Status", d.Status}, {"Job", d.JobID}, {"Version", fmt.Sprint(d.JobVersion)}}, width-1); row != "" {
		head = append(head, " "+row)
	}

	if d.StatusDescription != "" {
		head = append(head, " "+styleMuted.Render(truncate(d.StatusDescription, width-1)))
	}

	return fitPanel(head, groupBlock(d.Groups, d.Active(), width), room)
}

// groupBlock is the groups of a deployment as a table.
func groupBlock(groups []nomad.DeploymentGroup, active bool, width int) panelBlock {
	cells := make([][]string, 0, len(groups))
	for _, g := range groups {
		cells = append(cells, groupCells(g, active))
	}

	widths := make([]int, len(groupTitles))
	for i, title := range groupTitles {
		widths[i] = len(title)
	}

	for _, row := range cells {
		for i, cell := range row {
			widths[i] = max(widths[i], ansi.StringWidth(cell))
		}
	}

	line := func(row []string) string {
		padded := make([]string, len(row))
		for i, cell := range row {
			padded[i] = pad(cell, widths[i])
		}

		return truncate(" "+strings.TrimRight(strings.Join(padded, strings.Repeat(" ", columnGap)), " "), width)
	}

	entries := make([][]string, 0, len(groups))
	for i, g := range groups {
		entries = append(entries, []string{groupStyle(g).Render(line(cells[i]))})
	}

	return panelBlock{
		title:   []string{styleTableHeader.Render(line(groupTitles))},
		entries: entries,
		more:    func(count int) string { return fmt.Sprintf("  + %d more groups", count) },
	}
}

// groupCells are the cells of one group. A group without canaries has
// nothing to promote, and says so with a dash.
func groupCells(g nomad.DeploymentGroup, active bool) []string {
	canaries, promoted := "-", "-"
	if g.DesiredCanaries > 0 {
		canaries = fmt.Sprintf("%d/%d", g.PlacedCanaries, g.DesiredCanaries)
		promoted = yesNo(g.Promoted)
	}

	deadline := "-"
	if g.ProgressDeadline > 0 {
		deadline = age(g.ProgressDeadline)
	}

	// The cluster keeps the deadline of the last step of a deployment that
	// is over: nothing is due by it any more.
	progressBy := "-"
	if left := time.Until(g.RequireProgressBy); active && !g.RequireProgressBy.IsZero() && left > 0 {
		progressBy = "in " + age(left)
	}

	return []string{
		g.Name,
		fmt.Sprint(g.DesiredTotal),
		fmt.Sprint(g.Placed),
		fmt.Sprint(g.Healthy),
		fmt.Sprint(g.Unhealthy),
		canaries,
		promoted,
		yesNo(g.AutoRevert),
		deadline,
		progressBy,
	}
}

// groupStyle is the colour of a group: what failed, and what waits for a
// person to promote it.
func groupStyle(g nomad.DeploymentGroup) lipgloss.Style {
	switch {
	case g.Unhealthy > 0:
		return styleError
	case g.WaitsForPromotion():
		return stylePending
	}

	return styleText
}

// deploymentAllocRows are the allocations of a deployment, with whether each
// is a canary and how the deployment judged it.
func deploymentAllocRows(allocs []nomad.Alloc, usage map[string]nomad.ResourceUse) []tableRow {
	rows := make([]tableRow, 0, len(allocs))

	for _, alloc := range allocs {
		use, known := usage[alloc.ID]

		canary := ""
		if alloc.Canary {
			canary = "yes"
		}

		rows = append(rows, tableRow{
			cells: []string{
				shortID(alloc.ID),
				alloc.TaskGroup,
				alloc.NodeName,
				alloc.Status,
				canary,
				alloc.Health,
				cpuCell(use, known),
				memoryCell(use, known),
				ageOf(alloc.Created),
			},
			color: allocColor(alloc),
		})
	}

	return rows
}
