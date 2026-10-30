package ui

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/hashicorp/nomad/api"

	"github.com/ingvarch/urga/internal/nomad"
)

// queuePage lists the batch jobs waiting in or being placed from the batch
// job queues of the cluster or of one node pool.
type queuePage struct {
	pool        string
	queueType   string
	items       []nomad.QueueWorkload
	tenants     []nomad.QueueTenant
	unsupported bool
	listed      bool
}

func (p queuePage) title(_ env, count int) string {
	if p.pool != "" {
		return sprintf("Queue (Pool: %s) [%d]", p.pool, count)
	}

	return sprintf("Queue [%d]", count)
}

func (p queuePage) followsSession() bool { return p.pool == "" }

func (queuePage) topics() []string {
	return []string{nomad.TopicJob, nomad.TopicNodePool}
}

var (
	queueTitlesFifo        = []string{"Pos", "Job", "Namespace", "Status", "Age"}
	queueTitlesPoolFifo    = []string{"Pos", "Job", "Namespace", "Pool", "Status", "Age"}
	queueTitlesDynamic     = []string{"Pos", "Job", "Namespace", "Status", "Priority", "Base", "Fairshare", "Age Adj", "CPU Adj", "Mem Adj", "Age"}
	queueTitlesPoolDynamic = []string{"Pos", "Job", "Namespace", "Pool", "Status", "Priority", "Base", "Fairshare", "Age Adj", "CPU Adj", "Mem Adj", "Age"}
)

func (p queuePage) dynamic() bool {
	if p.queueType == "dynamic priority" || p.queueType == "dynamic_priority" {
		return true
	}

	for _, it := range p.items {
		if it.QueueType == string(api.BatchQueueTypeDynamic) || it.QueueType == "dynamic priority" {
			return true
		}
	}

	return false
}

func (p queuePage) titles() []string {
	if p.dynamic() {
		if p.pool == "" {
			return queueTitlesPoolDynamic
		}

		return queueTitlesDynamic
	}

	if p.pool == "" {
		return queueTitlesPoolFifo
	}

	return queueTitlesFifo
}

func (p queuePage) rows(env) []tableRow {
	rows := make([]tableRow, 0, len(p.items))
	dynamic := p.dynamic()

	for _, item := range p.items {
		row := tableRow{color: colorPending}
		pos := fmt.Sprintf("%d", item.Position+1)

		if dynamic {
			prio := fmt.Sprintf("%d", item.AdjustedPriority)
			base := fmt.Sprintf("%d", item.BasePriority)
			fair := formatAdj(item.FairshareAdjustment)
			ageAdj := formatAdj(item.AgeAdjustment)
			cpuAdj := formatAdj(item.CPUAdjustment)
			memAdj := formatAdj(item.MemoryAdjustment)

			if p.pool == "" {
				row.add(pos, item.JobID, item.Namespace, item.Pool, item.Status, prio, base, fair, ageAdj, cpuAdj, memAdj)
			} else {
				row.add(pos, item.JobID, item.Namespace, item.Status, prio, base, fair, ageAdj, cpuAdj, memAdj)
			}
		} else {
			if p.pool == "" {
				row.add(pos, item.JobID, item.Namespace, item.Pool, item.Status)
			} else {
				row.add(pos, item.JobID, item.Namespace, item.Status)
			}
		}

		row.addAge(item.CreatedAt)
		rows = append(rows, row)
	}

	return rows
}

func formatAdj(v int) string {
	if v > 0 {
		return fmt.Sprintf("+%d", v)
	}

	return fmt.Sprintf("%d", v)
}

func (p queuePage) panel(_ env, width, room int) []string {
	if p.unsupported {
		head := []string{panelRow(styleMuted.Render(truncate("The batch job queue needs Nomad 2.1.", width-tableIndent)))}

		return fitPanel(head, panelBlock{}, room)
	}

	if p.pool != "" && p.dynamic() && len(p.tenants) > 0 {
		return fitPanel(nil, tenantBlock(p.tenants, width), room)
	}

	return nil
}

var tenantTitles = []string{"Tenant", "Share", "CPU", "Memory"}

func tenantBlock(tenants []nomad.QueueTenant, width int) panelBlock {
	cells := make([][]string, 0, len(tenants))
	for _, t := range tenants {
		cells = append(cells, []string{
			t.ID,
			fmt.Sprintf("%d%%", t.PercentageUsed),
			formatMHz(t.Fairshare),
			formatMB(t.Fairshare),
		})
	}

	widths := make([]int, len(tenantTitles))
	for i, title := range tenantTitles {
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

		return truncate(panelRow(strings.TrimRight(strings.Join(padded, strings.Repeat(" ", columnGap)), " ")), width)
	}

	entries := make([][]string, 0, len(tenants))
	for _, row := range cells {
		entries = append(entries, []string{line(row)})
	}

	return panelBlock{
		title:   []string{styleTableHeader.Render(line(tenantTitles))},
		entries: entries,
		more:    func(count int) string { return fmt.Sprintf("  + %d more tenants", count) },
	}
}

func formatMHz(fairshare map[string]float64) string {
	if v, ok := lookupFairshare(fairshare, "cpu"); ok && v > 0 {
		return fmt.Sprintf("%.0f MHz", v)
	}

	return "-"
}

func formatMB(fairshare map[string]float64) string {
	if v, ok := lookupFairshare(fairshare, "memory"); ok && v > 0 {
		return fmt.Sprintf("%.0f MB", v)
	}

	return "-"
}

func lookupFairshare(m map[string]float64, key string) (float64, bool) {
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}

	return 0, false
}

type (
	queueItemsMsg struct {
		pool  string
		items []nomad.QueueWorkload
	}

	queueTenantsMsg struct {
		pool    string
		tenants []nomad.QueueTenant
	}

	queueUnsupportedMsg struct{}
)

func (p queuePage) fetch(e env) tea.Cmd {
	client, pool := e.client, p.pool

	if pool != "" {
		return tea.Batch(
			fetchPoolQueue(client, pool),
			fetchPoolStatus(client, pool),
		)
	}

	return fetchAllQueues(client)
}

func fetchPoolQueue(client Client, pool string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		items, err := client.BatchQueueJobs(ctx, pool)
		if err != nil {
			if errors.Is(err, nomad.ErrQueueUnsupported) {
				return queueUnsupportedMsg{}
			}

			return errMsg{err: err}
		}

		return queueItemsMsg{pool: pool, items: items}
	}
}

func fetchPoolStatus(client Client, pool string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		tenants, err := client.BatchQueueStatus(ctx, pool)
		if err != nil {
			return nil
		}

		return queueTenantsMsg{pool: pool, tenants: tenants}
	}
}

func fetchAllQueues(client Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		pools, err := client.NodePools(ctx)
		if err != nil {
			if errors.Is(err, nomad.ErrQueueUnsupported) {
				return queueUnsupportedMsg{}
			}

			return errMsg{err: err}
		}

		var all []nomad.QueueWorkload
		hasQueue := false

		for _, p := range pools {
			if p.Queue == "" {
				continue
			}
			hasQueue = true

			items, err := client.BatchQueueJobs(ctx, p.Name)
			if err != nil {
				if errors.Is(err, nomad.ErrQueueUnsupported) {
					return queueUnsupportedMsg{}
				}

				continue
			}

			all = append(all, items...)
		}

		if !hasQueue {
			if _, err := client.BatchQueueJobs(ctx, "default"); errors.Is(err, nomad.ErrQueueUnsupported) {
				return queueUnsupportedMsg{}
			}
		}

		sortQueueWorkloads(all)

		return queueItemsMsg{items: all}
	}
}

func sortQueueWorkloads(items []nomad.QueueWorkload) {
	slices.SortStableFunc(items, func(a, b nomad.QueueWorkload) int {
		if a.Position != b.Position {
			return cmp.Compare(a.Position, b.Position)
		}
		if a.Pool != b.Pool {
			return strings.Compare(a.Pool, b.Pool)
		}

		return strings.Compare(a.JobID, b.JobID)
	})
}

func (p queuePage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	switch msg := msg.(type) {
	case queueItemsMsg:
		if p.pool != "" && msg.pool != p.pool {
			return p, outcome{}, false
		}

		p.items = msg.items
		p.listed = true
		p.unsupported = false

		if p.pool != "" && len(p.items) > 0 {
			p.queueType = p.items[0].QueueType
		}

		return p, outcome{}, true

	case queueTenantsMsg:
		if p.pool != "" && msg.pool != p.pool {
			return p, outcome{}, false
		}

		p.tenants = msg.tenants

		return p, outcome{reading: true}, true

	case queueUnsupportedMsg:
		p.unsupported = true
		p.listed = true
		p.items = nil
		p.tenants = nil

		return p, outcome{}, true
	}

	return p, outcome{}, false
}

var queueKeys = []pageKey[queuePage]{
	{press: "enter", label: "Allocations", do: openQueueJob},
}

func (p queuePage) keys(e env) []keyHint { return hintsOf(p, e, queueKeys) }

func (p queuePage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, queueKeys, k)
}

func (p queuePage) ids(env) []string {
	ids := make([]string, 0, len(p.items))
	for _, item := range p.items {
		ids = append(ids, item.Namespace+"/"+item.JobID)
	}

	return ids
}

func openQueueJob(p queuePage, e env) (queuePage, outcome) {
	item, ok := pickedFrom(e, p.items)
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg{allocationsPage{namespace: item.Namespace, jobID: item.JobID}})
}
