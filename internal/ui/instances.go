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

// servicesPage is the services of the namespace the session looks at.
type servicesPage struct {
	ofTheSession

	services []nomad.Service
}

var serviceTitles = []string{"Name", "Namespace", "Tags"}

func (servicesPage) title(e env, count int) string {
	return sprintf("Services (%s) [%d]", namespaceLabel(e.namespace), count)
}

func (servicesPage) titles() []string { return serviceTitles }
func (servicesPage) topics() []string { return []string{nomad.TopicService} }

func (servicesPage) fetch(e env) tea.Cmd {
	client, namespace := e.client, e.namespace

	return fetchList(func(ctx context.Context) ([]nomad.Service, error) {
		return client.Services(ctx, namespace)
	}, func(items []nomad.Service) tea.Msg { return servicesMsg(items) })
}

func (p servicesPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	services, ok := msg.(servicesMsg)
	if !ok {
		return p, outcome{}, false
	}

	p.services = services

	return p, outcome{}, true
}

func (p servicesPage) rows(env) []tableRow { return serviceRows(p.services) }

func serviceRows(services []nomad.Service) []tableRow {
	rows := make([]tableRow, 0, len(services))

	for _, s := range services {
		rows = append(rows, tableRow{cells: []string{s.Name, s.Namespace, strings.Join(s.Tags, ", ")}})
	}

	return rows
}

var servicesKeys = []pageKey[servicesPage]{
	{press: "enter", label: "Instances", do: openServiceInstances},
	{press: "d", label: "Describe", do: describeService},
}

func (p servicesPage) keys(e env) []keyHint { return hintsOf(p, e, servicesKeys) }

func (p servicesPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, servicesKeys, k)
}

// openServiceInstances opens the instances of the service under the cursor.
// The screen keeps the namespace of the service: its stream watches there.
func openServiceInstances(p servicesPage, e env) (servicesPage, outcome) {
	service, ok := pickedFrom(e, p.services)
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg{serviceInstancesPage{namespace: service.Namespace, service: service.Name}})
}

// describeService asks for the service under the cursor, in the words of
// the cluster.
func describeService(p servicesPage, e env) (servicesPage, outcome) {
	service, ok := pickedFrom(e, p.services)
	if !ok {
		return p, outcome{}
	}

	client := e.client

	return p, outcome{cmd: describe(fmt.Sprintf("Service: %s", service.Name), func(ctx context.Context) (string, error) {
		return client.DescribeService(ctx, service.Namespace, service.Name)
	})}
}

// serviceInstancesPage is the instances of one service, and what their
// checks last said, by the allocation that runs them.
type serviceInstancesPage struct {
	namespace, service string

	instances []nomad.ServiceInstance
	byAlloc   map[string][]nomad.Check

	// due says the next reading or its timer is on its way: the list is read
	// again on every change, and none of those may start a second chain.
	due bool
}

func (p serviceInstancesPage) title(_ env, count int) string {
	return sprintf("Service %s (%s) [%d]", p.service, namespaceLabel(p.namespace), count)
}

func (serviceInstancesPage) titles() []string   { return instanceTitles }
func (serviceInstancesPage) topics() []string   { return []string{nomad.TopicService} }
func (p serviceInstancesPage) where(env) string { return p.namespace }

// fetch reads the instances of the service the page is open on.
func (p serviceInstancesPage) fetch(e env) tea.Cmd {
	client, namespace, service := e.client, p.namespace, p.service

	return fetchList(func(ctx context.Context) ([]nomad.ServiceInstance, error) {
		return client.ServiceInstances(ctx, namespace, service)
	}, func(items []nomad.ServiceInstance) tea.Msg { return instancesMsg(items) })
}

func (p serviceInstancesPage) take(msg tea.Msg, e env) (page, outcome, bool) {
	switch msg := msg.(type) {
	case instancesMsg:
		p.instances = msg

		// The first reading of the checks is taken when the instances are
		// listed, and only then: the timer keeps them coming.
		if p.due {
			return p, outcome{}, true
		}

		p.due = true

		return p, outcome{cmd: p.readChecks(e)}, true

	case instanceChecksMsg:
		if msg.service != p.service {
			return p, outcome{}, false
		}

		// What the checks said goes on the rows, and the timer is set for
		// the next reading.
		p.byAlloc = msg.byAlloc
		next := tea.Tick(checksEvery, func(time.Time) tea.Msg { return pollInstanceChecksMsg{} })

		return p, outcome{cmd: next, reading: true}, true

	case pollInstanceChecksMsg:
		// The timer went off: the checks are read again.
		return p, outcome{cmd: p.readChecks(e), reading: true}, true
	}

	return p, outcome{}, false
}

// restart lets go of the reading the page thinks is on its way: it belonged
// to an ask that is over.
func (p serviceInstancesPage) restart() page {
	p.due = false

	return p
}

// readChecks reads the checks of the instances whose allocation runs, each
// from the client that runs it, all at once.
func (p serviceInstancesPage) readChecks(e env) tea.Cmd {
	client, service := e.client, p.service

	running := map[string]string{}
	for _, instance := range p.instances {
		if instance.AllocStatus == statusRunning {
			running[instance.AllocID] = instance.Namespace
		}
	}

	return func() tea.Msg {
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
	}
}

func (p serviceInstancesPage) rows(env) []tableRow { return instanceRows(p.instances, p.byAlloc) }

var serviceInstanceKeys = []pageKey[serviceInstancesPage]{
	{press: "enter", label: "Tasks", do: openInstanceAlloc, offered: instanceAllocHeld},
	{press: "ctrl+d", label: "Delete", do: deleteRegistration, writes: true, offered: instanceStale},
}

func (p serviceInstancesPage) keys(e env) []keyHint { return hintsOf(p, e, serviceInstanceKeys) }

func (p serviceInstancesPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, serviceInstanceKeys, k)
}

// picked is the instance under the cursor.
func (p serviceInstancesPage) picked(e env) (nomad.ServiceInstance, bool) {
	return pickedFrom(e, p.instances)
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
func instanceAllocHeld(p serviceInstancesPage, e env) bool {
	instance, ok := p.picked(e)

	return ok && instance.AllocStatus != ""
}

// instanceStale says the instance under the cursor outlived its allocation.
func instanceStale(p serviceInstancesPage, e env) bool {
	instance, ok := p.picked(e)

	return ok && instance.Stale()
}

// openInstanceAlloc opens the tasks of the allocation that registered the
// instance under the cursor.
func openInstanceAlloc(p serviceInstancesPage, e env) (serviceInstancesPage, outcome) {
	instance, ok := p.picked(e)
	if !ok || instance.AllocStatus == "" {
		return p, outcome{}
	}

	return p, then(openMsg{tasksPage{namespace: instance.Namespace, jobID: instance.JobID, allocID: instance.AllocID}})
}

// deleteRegistration takes a registration that outlived its allocation out
// of the catalog, once it is asked about. The key is offered on no other:
// one that still takes traffic is left alone.
func deleteRegistration(p serviceInstancesPage, e env) (serviceInstancesPage, outcome) {
	instance, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	client, where := e.client, addressOf(instance)

	return p, then(askMsg{
		question: fmt.Sprintf("Really delete the registration of %s at %s? Its allocation is %s.", instance.Service, where, allocState(instance)),
		apply: act(fmt.Sprintf("Registration of %s at %s deleted.", instance.Service, where), func(ctx context.Context) error {
			return client.DeleteServiceRegistration(ctx, instance.Namespace, instance.Service, instance.ID)
		}),
	})
}
