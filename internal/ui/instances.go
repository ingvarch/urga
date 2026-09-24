package ui

import (
	"context"
	"fmt"
	"image/color"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// instanceTitles are the columns of the instances of a service.
var instanceTitles = []string{"Address", "Allocation", "Node", "Tags", "Checks", "Status"}

// Messages of the instances of a service.
type (
	instancesMsg []nomad.ServiceInstance

	// instanceChecksMsg is what the checks of the instances said, by the
	// allocation that runs them, for the service they were read for.
	instanceChecksMsg struct {
		service string
		byAlloc map[string][]nomad.Check
	}

	// pollInstanceChecksMsg is the timer of the checks going off.
	pollInstanceChecksMsg struct{}
)

// instanceChecksState is what the checks of the instances last said.
type instanceChecksState struct {
	byAlloc map[string][]nomad.Check

	// due says the next reading or its timer is on its way: the list is read
	// again on every change, and none of those may start a second chain.
	due bool
}

var instanceBindings = []binding{
	{press: "enter", label: "Tasks", do: openInstanceAlloc, offered: instanceAllocHeld},
	{press: "ctrl+d", label: "Delete", do: deleteRegistration, writes: true, offered: instanceStale},
}

// openServiceInstances opens the instances of the service under the cursor.
func openServiceInstances(m Model) (Model, tea.Cmd) {
	service, ok := selectedOf(m, screenServices, m.services)
	if !ok {
		return m, nil
	}

	return m.push(screen{kind: screenServiceInstances, namespace: service.Namespace, label: service.Name})
}

// fetchInstances reads the instances of the service the screen is open on.
func fetchInstances(m Model) tea.Cmd {
	client, s := m.client, m.screen

	return fetchList(func(ctx context.Context) ([]nomad.ServiceInstance, error) {
		return client.ServiceInstances(ctx, s.namespace, s.label)
	}, func(items []nomad.ServiceInstance) tea.Msg { return instancesMsg(items) })
}

// instanceChecksOnce takes the first reading of the checks when the
// instances are listed, and only then: the timer keeps them coming.
func instanceChecksOnce(m Model, read tea.Cmd) (Model, tea.Cmd) {
	if m.screen.kind != screenServiceInstances || m.instanceChecks.due {
		return m, read
	}

	m.instanceChecks.due = true

	return m, tea.Batch(read, m.readInstanceChecks())
}

// readInstanceChecks reads the checks of the instances whose allocation
// runs, each from the client that runs it, all at once.
func (m Model) readInstanceChecks() tea.Cmd {
	client, service := m.client, m.screen.label

	running := map[string]string{}
	for _, instance := range m.instances {
		if instance.AllocStatus == statusRunning {
			running[instance.AllocID] = instance.Namespace
		}
	}

	return askedFor(m.asked, func() tea.Msg {
		byAlloc := map[string][]nomad.Check{}

		var (
			wait sync.WaitGroup
			lock sync.Mutex
		)

		for allocID, namespace := range running {
			wait.Go(func() {
				ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()

				checks, err := client.AllocationChecks(ctx, namespace, allocID)
				if err != nil {
					return
				}

				lock.Lock()
				byAlloc[allocID] = checks
				lock.Unlock()
			})
		}

		wait.Wait()

		return instanceChecksMsg{service: service, byAlloc: byAlloc}
	})
}

// keepInstanceChecks puts what the checks said on the rows, and sets the
// timer for the next reading.
func (m Model) keepInstanceChecks(msg instanceChecksMsg) (Model, tea.Cmd) {
	if m.screen.kind != screenServiceInstances || msg.service != m.screen.label {
		return m, nil
	}

	m.instanceChecks.byAlloc = msg.byAlloc
	m.layout()

	return m, askedFor(m.asked, tea.Tick(checksEvery, func(time.Time) tea.Msg { return pollInstanceChecksMsg{} }))
}

// pollInstanceChecks reads the checks again when their timer goes off.
func (m Model) pollInstanceChecks() (Model, tea.Cmd) {
	if m.screen.kind != screenServiceInstances {
		return m, nil
	}

	return m, m.readInstanceChecks()
}

// instanceRows are the instances of a service: where each takes traffic,
// what registered it, its checks, and whether it outlived its allocation.
func instanceRows(instances []nomad.ServiceInstance, byAlloc map[string][]nomad.Check) []tableRow {
	rows := make([]tableRow, 0, len(instances))

	for _, instance := range instances {
		node := instance.NodeName
		if node == "" {
			node = shortID(instance.NodeID)
		}

		checks, failing := checkSummary(instance, byAlloc[instance.AllocID])

		rows = append(rows, tableRow{
			cells: []string{
				addressOf(instance),
				shortID(instance.AllocID),
				node,
				strings.Join(instance.Tags, ", "),
				checks,
				instanceStatus(instance),
			},
			color: instanceColor(instance, failing),
		})
	}

	return rows
}

// checkSummary says how the checks of the service went on the allocation of
// an instance: the worst of them first. What does not run has none.
func checkSummary(instance nomad.ServiceInstance, checks []nomad.Check) (string, bool) {
	counts := map[string]int{}

	for _, check := range checks {
		if check.Service == instance.Service {
			counts[check.Status]++
		}
	}

	switch {
	case instance.AllocStatus != statusRunning || len(counts) == 0:
		return "-", false
	case counts[checkFailure] > 0:
		return fmt.Sprintf("%d failing", counts[checkFailure]), true
	case counts["success"] == 0:
		return fmt.Sprintf("%d pending", len(checks)), false
	}

	return fmt.Sprintf("%d passing", counts["success"]), false
}

// instanceStatus is what became of the allocation that registered the
// instance.
func instanceStatus(instance nomad.ServiceInstance) string {
	if !instance.Stale() {
		return instance.AllocStatus
	}

	return "stale: alloc " + allocState(instance)
}

// allocState is the status of the allocation of an instance, or gone when
// the cluster no longer holds it.
func allocState(instance nomad.ServiceInstance) string {
	if instance.AllocStatus == "" {
		return "gone"
	}

	return instance.AllocStatus
}

// instanceColor marks what outlived its allocation, and what fails its
// checks.
func instanceColor(instance nomad.ServiceInstance, failing bool) color.Color {
	switch {
	case instance.Stale():
		return colorDead
	case failing:
		return colorAttention
	}

	return nil
}

// addressOf is where an instance takes traffic.
func addressOf(instance nomad.ServiceInstance) string {
	return net.JoinHostPort(instance.Address, strconv.Itoa(instance.Port))
}

// instanceAllocHeld says the allocation of the instance under the cursor is
// still held by the cluster: there are tasks to open.
func instanceAllocHeld(m Model) bool {
	instance, ok := selectedOf(m, screenServiceInstances, m.instances)

	return ok && instance.AllocStatus != ""
}

// instanceStale says the instance under the cursor outlived its allocation.
func instanceStale(m Model) bool {
	instance, ok := selectedOf(m, screenServiceInstances, m.instances)

	return ok && instance.Stale()
}

// openInstanceAlloc opens the tasks of the allocation that registered the
// instance under the cursor.
func openInstanceAlloc(m Model) (Model, tea.Cmd) {
	instance, ok := selectedOf(m, screenServiceInstances, m.instances)
	if !ok || instance.AllocStatus == "" {
		return m, nil
	}

	return m.push(screen{kind: screenTasks, namespace: instance.Namespace, jobID: instance.JobID, allocID: instance.AllocID})
}

// deleteRegistration takes a registration that outlived its allocation out
// of the catalog, once it is asked about. The key is offered on no other:
// one that still takes traffic is left alone.
func deleteRegistration(m Model) (Model, tea.Cmd) {
	instance, ok := selectedOf(m, screenServiceInstances, m.instances)
	if !ok {
		return m, nil
	}

	client, where := m.client, addressOf(instance)

	return m.ask(
		fmt.Sprintf("Really delete the registration of %s at %s? Its allocation is %s.", instance.Service, where, allocState(instance)),
		act(fmt.Sprintf("Registration of %s at %s deleted.", instance.Service, where), func(ctx context.Context) error {
			return client.DeleteServiceRegistration(ctx, instance.Namespace, instance.Service, instance.ID)
		}),
	)
}
