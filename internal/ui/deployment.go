package ui

import (
	"context"
	"fmt"
	"image/color"
	"slices"
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
// the job and the namespace are the deployment's, and the screen is read for
// the canary and health of each allocation.
var deploymentAllocTitles = []string{"ID", "TaskGroup", "Node", "Status", "Canary", "Health", "CPU", "MEM", "Age"}

// groupTitles are the columns of the groups of a deployment.
var groupTitles = []string{"Group", "Desired", "Placed", "Healthy", "Unhealthy", "Canaries", "Promoted", "Auto Revert", "Deadline", "Progress By"}

// deploymentTitles are the columns of the deployment list.
var deploymentTitles = []string{"ID", "JobID", "Namespace", "Version", "Status", "Description"}

// deploymentsPage is the deployments of the namespace the session looks at.
type deploymentsPage struct {
	ofTheSession

	deployments []nomad.Deployment
}

func (deploymentsPage) title(e env, count int) string {
	return sprintf("Deployments (%s) [%d]", namespaceLabel(e.namespace), count)
}

func (deploymentsPage) titles() []string { return deploymentTitles }
func (deploymentsPage) topics() []string { return []string{nomad.TopicDeployment} }

func (deploymentsPage) fetch(e env) tea.Cmd {
	client, namespace := e.client, e.namespace

	return fetchList(func(ctx context.Context) ([]nomad.Deployment, error) {
		return client.Deployments(ctx, namespace)
	}, func(items []nomad.Deployment) tea.Msg { return deploymentsMsg(items) })
}

func (p deploymentsPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	deployments, ok := msg.(deploymentsMsg)
	if !ok {
		return p, outcome{}, false
	}

	p.deployments = deployments

	return p, outcome{}, true
}

func (p deploymentsPage) rows(env) []tableRow { return deploymentRows(p.deployments) }

// inView is the deployment under the cursor.
func (p deploymentsPage) inView(e env) (nomad.Deployment, bool) { return pickedFrom(e, p.deployments) }

var deploymentsKeys = []pageKey[deploymentsPage]{
	{press: "enter", label: "Details", do: openDeployment},
	{press: "d", label: "Describe", do: describeDeployment},
	{press: "p", label: "Promote", do: promoteDeployment[deploymentsPage], writes: true},
	{press: "f", label: "Fail", do: failDeployment[deploymentsPage], writes: true},
	{press: "ctrl+s", label: "Pause", do: pauseDeployment[deploymentsPage], writes: true, offered: deploymentIs[deploymentsPage]("running")},
	{press: "ctrl+s", label: "Resume", do: pauseDeployment[deploymentsPage], writes: true, offered: deploymentIs[deploymentsPage]("paused")},
}

func (p deploymentsPage) keys(e env) []keyHint { return hintsOf(p, e, deploymentsKeys) }

func (p deploymentsPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, deploymentsKeys, k)
}

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

// openDeployment opens the deployment under the cursor: how far it got with
// each group, above the allocations it placed.
func openDeployment(p deploymentsPage, e env) (deploymentsPage, outcome) {
	deployment, ok := p.inView(e)
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg{deploymentOf(deployment)})
}

// deploymentHolder is a page whose keys act on one deployment: the one
// under the cursor of the list, or the one a deployment page read.
type deploymentHolder interface {
	page
	inView(e env) (nomad.Deployment, bool)
}

// deploymentIs offers a key when the deployment in view is in that state.
func deploymentIs[P deploymentHolder](status string) func(p P, e env) bool {
	return func(p P, e env) bool {
		d, ok := p.inView(e)

		return ok && d.Status == status
	}
}

// promoteDeployment promotes the canaries of every group of the deployment
// in view.
func promoteDeployment[P deploymentHolder](p P, e env) (P, outcome) {
	deployment, ok := p.inView(e)
	if !ok {
		return p, outcome{}
	}

	client := e.client

	return p, then(askMsg{
		question: fmt.Sprintf("Really promote the canaries of every group of %s?", deployment.JobID),
		apply: act(fmt.Sprintf("Deployment of %s promoted.", deployment.JobID), func(ctx context.Context) error {
			return client.PromoteDeployment(ctx, deployment.Namespace, deployment.ID)
		}),
	})
}

// failDeployment marks a deployment as failed, which stops it.
func failDeployment[P deploymentHolder](p P, e env) (P, outcome) {
	deployment, ok := p.inView(e)
	if !ok {
		return p, outcome{}
	}

	client := e.client

	return p, then(askMsg{
		question: fmt.Sprintf("Really fail the deployment of %s? It rolls back where the job says to.", deployment.JobID),
		apply: act(fmt.Sprintf("Deployment of %s failed.", deployment.JobID), func(ctx context.Context) error {
			return client.FailDeployment(ctx, deployment.Namespace, deployment.ID)
		}),
	})
}

// pauseDeployment pauses the deployment in view, or resumes a paused one.
func pauseDeployment[P deploymentHolder](p P, e env) (P, outcome) {
	d, ok := p.inView(e)
	if !ok {
		return p, outcome{}
	}

	pause, verb, done := true, "pause", "paused"
	if d.Status == "paused" {
		pause, verb, done = false, "resume", "resumed"
	}

	client := e.client

	return p, then(askMsg{
		question: fmt.Sprintf("Really %s the deployment of %s?", verb, d.JobID),
		apply: act(fmt.Sprintf("Deployment of %s %s.", d.JobID, done), func(ctx context.Context) error {
			return client.PauseDeployment(ctx, d.Namespace, d.ID, pause)
		}),
	})
}

// deploymentPage is one deployment: how far it got with each group, above
// the allocations it placed.
type deploymentPage struct {
	namespace, jobID, deploymentID string

	// deployment is the deployment as the page last read it, on its own
	// and apart from its allocations; read says the page has read it. Until
	// then there is no panel, and no key of the deployment.
	deployment nomad.DeploymentDetail
	read       bool

	allocs []nomad.Alloc
}

// deploymentOf is the page of a deployment in its namespace, which is where
// its stream watches.
func deploymentOf(d nomad.Deployment) deploymentPage {
	return deploymentPage{namespace: d.Namespace, jobID: d.JobID, deploymentID: d.ID}
}

func (p deploymentPage) title(_ env, count int) string {
	return sprintf("Deployment %s (Job: %s) [%d]", shortID(p.deploymentID), p.jobID, count)
}

func (deploymentPage) titles() []string { return deploymentAllocTitles }

// topics: the allocations it placed change, and so does the deployment.
func (p deploymentPage) where(env) string { return p.namespace }

func (deploymentPage) topics() []string {
	return []string{nomad.TopicAllocation, nomad.TopicDeployment}
}

// fetch reads the allocations the deployment placed, and the deployment on
// its own next to them.
func (p deploymentPage) fetch(e env) tea.Cmd {
	client, namespace, id := e.client, p.namespace, p.deploymentID

	return tea.Batch(
		fetchList(func(ctx context.Context) ([]nomad.Alloc, error) {
			return client.DeploymentAllocations(ctx, namespace, id)
		}, func(items []nomad.Alloc) tea.Msg { return allocsMsg(items) }),
		fetchDeployment(client, namespace, id),
	)
}

func (p deploymentPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	switch msg := msg.(type) {
	case allocsMsg:
		p.allocs = msg

		return p, outcome{}, true

	case deploymentMsg:
		// Only the deployment of this page is kept: an answer about another
		// deployment is ignored.
		if msg.ID != p.deploymentID {
			return p, outcome{}, false
		}

		p.deployment, p.read = nomad.DeploymentDetail(msg), true

		return p, outcome{}, true
	}

	return p, outcome{}, false
}

// panel is the panel of the deployment the page read. Until it has, there
// is none.
func (p deploymentPage) panel(_ env, width, room int) []string {
	if !p.read {
		return nil
	}

	return deploymentPanel(p.deployment, width, room)
}

func (p deploymentPage) rows(e env) []tableRow { return deploymentAllocRows(p.allocs, e.usage) }

func (p deploymentPage) visible(env) []nomad.Alloc { return p.allocs }

func (p deploymentPage) ids(env) []string { return names(p.allocs, allocMark) }

func (p deploymentPage) readings(e env) []rowRef { return runningRefs(e.index, p.allocs) }

func (deploymentPage) reading(ctx context.Context, client Client, ref rowRef) (nomad.ResourceUse, error) {
	return allocReading(ctx, client, ref)
}

// logsOf: the logs of the allocations the deployment placed for its job.
func (p deploymentPage) logsOf(env) logScope {
	return logScope{namespace: p.namespace, jobID: p.jobID, deploymentID: p.deploymentID}
}

// inView is the deployment the page read.
func (p deploymentPage) inView(env) (nomad.Deployment, bool) { return p.deployment.Deployment, p.read }

// deploymentKeys are the actions of the deployment, then those of its
// allocations, the way a client screen lists the node first.
var deploymentKeys = append([]pageKey[deploymentPage]{
	{press: "p", label: "Promote Group", do: promoteGroup, writes: true, offered: groupWaitsHere},
	{press: "ctrl+p", label: "Promote All", do: promoteDeployment[deploymentPage], writes: true, offered: someGroupWaits},
	{press: "f", label: "Fail", do: failDeployment[deploymentPage], writes: true, offered: deploymentActive},
	{press: "ctrl+s", label: "Pause", do: pauseDeployment[deploymentPage], writes: true, offered: deploymentIs[deploymentPage]("running")},
	{press: "ctrl+s", label: "Resume", do: pauseDeployment[deploymentPage], writes: true, offered: deploymentIs[deploymentPage]("paused")},
}, allocKeys[deploymentPage]()...)

func (p deploymentPage) keys(e env) []keyHint { return hintsOf(p, e, deploymentKeys) }

func (p deploymentPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, deploymentKeys, k)
}

// active says the deployment the page read is not over.
func (p deploymentPage) active() bool { return p.read && p.deployment.Active() }

// waitingGroup is the group of the allocation under the cursor, when its
// canaries wait to be promoted.
func (p deploymentPage) waitingGroup(e env) (string, bool) {
	alloc, ok := pickedFrom(e, p.allocs)
	if !ok || !p.active() {
		return "", false
	}

	for _, g := range p.deployment.Groups {
		if g.Name == alloc.TaskGroup {
			return g.Name, g.WaitsForPromotion()
		}
	}

	return "", false
}

// deploymentActive says the deployment in view is not over.
func deploymentActive(p deploymentPage, _ env) bool { return p.active() }

func groupWaitsHere(p deploymentPage, e env) bool {
	_, ok := p.waitingGroup(e)

	return ok
}

// someGroupWaits says a group of the deployment on the screen has canaries
// to promote.
func someGroupWaits(p deploymentPage, _ env) bool {
	return p.active() && slices.ContainsFunc(p.deployment.Groups, nomad.DeploymentGroup.WaitsForPromotion)
}

// promoteGroup promotes the canaries of the group of the allocation under
// the cursor. The other groups are left as they are.
func promoteGroup(p deploymentPage, e env) (deploymentPage, outcome) {
	group, ok := p.waitingGroup(e)
	if !ok {
		return p, outcome{}
	}

	client, d := e.client, p.deployment

	return p, then(askMsg{
		question: fmt.Sprintf("Really promote the canaries of group %s of %s?", group, d.JobID),
		apply: act(fmt.Sprintf("Canaries of %s promoted.", group), func(ctx context.Context) error {
			return client.PromoteGroups(ctx, d.Namespace, d.ID, []string{group})
		}),
	})
}

// fetchDeployment reads a deployment on its own. The allocations it placed
// are read the way those of any list are.
func fetchDeployment(client deploymentsClient, namespace, deploymentID string) tea.Cmd {
	return request(func(ctx context.Context) (nomad.DeploymentDetail, error) {
		return client.Deployment(ctx, namespace, deploymentID)
	}, func(d nomad.DeploymentDetail) tea.Msg { return deploymentMsg(d) })
}

// deploymentPanel is the panel above the allocations of a deployment: what
// the deployment is, and how far it got with each group.
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
// nothing to promote, and shows a dash there.
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

		row := tableRow{color: allocColor(alloc)}
		row.add(
			shortID(alloc.ID),
			alloc.TaskGroup,
			alloc.NodeName,
			alloc.Status,
			canary,
			alloc.Health,
			cpuCell(use, known),
			memoryCell(use, known),
		)
		row.addAge(alloc.Created)

		rows = append(rows, row)
	}

	return rows
}
