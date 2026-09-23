package ui

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// Messages of the regions and the datacenters.
type (
	regionsMsg []string

	// datacentersMsg carries the region it was asked in: datacenters are
	// named per region, and the same name can stand in two of them.
	datacentersMsg struct {
		region string
		names  []string
	}
)

// errNoRegions is a session that was given no way to ask another region.
var errNoRegions = errors.New("this session cannot switch regions")

// InRegionOf is how a session asks a cluster in another region.
func InRegionOf(client *nomad.Client) func(region string) Client {
	return func(region string) Client { return client.InRegion(region) }
}

// regionCommand switches to a region by name, or says which there are.
func (m Model) regionCommand(name string) (Model, tea.Cmd) {
	return m.pick(name, m.regions, "region", "Regions", Model.switchRegion)
}

// datacenterCommand narrows the session to a datacenter, or to every one of
// them again, or says which there are.
func (m Model) datacenterCommand(name string) (Model, tea.Cmd) {
	if name == everyDatacenter {
		return m.switchDatacenter("")
	}

	return m.pick(name, m.datacenters, "datacenter", "Datacenters", Model.switchDatacenter)
}

// pick reads the name a scope was given: no name lists what there is, and a
// name the cluster does not know is turned down with the ones it does.
func (m Model) pick(name string, known []string, kind, kinds string, to func(Model, string) (Model, tea.Cmd)) (Model, tea.Cmd) {
	if name == "" {
		return m.say(fmt.Sprintf("%s: %s.", kinds, listed(known))), nil
	}

	if !slices.Contains(known, name) {
		return m.fail(fmt.Errorf("no such %s: %s (%s)", kind, name, listed(known))), nil
	}

	return to(m, name)
}

// switchRegion points the session at another region. What was open belongs
// to the region that was left, so the session goes back to the list it
// was opened from.
func (m Model) switchRegion(region string) (Model, tea.Cmd) {
	if region == m.regionInUse() {
		return m, nil
	}

	if m.opts.InRegion == nil {
		return m.fail(errNoRegions), nil
	}

	m.closeLogs()

	m.client = m.opts.InRegion(region)
	m.datacenter, m.datacenters = "", nil
	m.usage = nomad.Usage{}

	m.screen, m.history = m.listScreen(), nil
	m.marks = nil

	next, cmd := m.arrive()

	return next, tea.Batch(cmd, fetchDatacenters(next.client), next.fetchClusterUsage())
}

// switchDatacenter narrows the lists and the header to a datacenter, empty
// for every one of them.
func (m Model) switchDatacenter(datacenter string) (Model, tea.Cmd) {
	if datacenter == m.datacenter {
		return m, nil
	}

	m.datacenter = datacenter
	m.usage = nomad.Usage{}

	next, cmd := m.enter()

	return next, tea.Batch(cmd, next.fetchClusterUsage())
}

// listScreen is the list the open screen was reached from: the last screen
// on the way here that can be opened by name.
func (m Model) listScreen() screen {
	if m.screen.of().stored != "" {
		return m.screen
	}

	for i := len(m.history) - 1; i >= 0; i-- {
		if m.history[i].of().stored != "" {
			return m.history[i]
		}
	}

	return screen{kind: screenJobs, namespace: m.namespace}
}

// listed is a list of names as a message reads it.
func listed(names []string) string {
	if len(names) == 0 {
		return "none known"
	}

	return strings.Join(names, ", ")
}

// regionInUse is where the session asks: the region it was pointed at, or
// the one of the agent when it names none. Empty until the agent answers.
func (m Model) regionInUse() string {
	if region := m.client.Region(); region != "" {
		return region
	}

	return m.agentRegion
}

// The lists that have a datacenter are narrowed to it as they are stored:
// every key that finds a row by its place then finds the one on the screen.

func (m Model) jobsInView(jobs []nomad.Job) []nomad.Job {
	return keep(jobs, func(job nomad.Job) bool { return job.RunsIn(m.datacenter) })
}

func (m Model) nodesInView(nodes []nomad.Node) []nomad.Node {
	return keep(nodes, func(node nomad.Node) bool { return m.inDatacenter(node.Datacenter) })
}

// serversInView are the servers of the region in use: the cluster lists
// every region it gossips with, and datacenters are named per region.
func (m Model) serversInView(servers []nomad.Server) []nomad.Server {
	region := m.regionInUse()

	return keep(servers, func(server nomad.Server) bool {
		return (region == "" || server.Region == region) && m.inDatacenter(server.Datacenter)
	})
}

func (m Model) inDatacenter(datacenter string) bool {
	return m.datacenter == "" || m.datacenter == datacenter
}

// keep is what of a list passes, in a list of its own: the answer is not
// written over.
func keep[T any](items []T, pass func(T) bool) []T {
	kept := make([]T, 0, len(items))

	for _, item := range items {
		if pass(item) {
			kept = append(kept, item)
		}
	}

	return kept
}

func fetchRegions(client Client) tea.Cmd {
	return fetchList(client.Regions, func(names []string) tea.Msg { return regionsMsg(names) })
}

func fetchDatacenters(client Client) tea.Cmd {
	region := client.Region()

	return fetchList(client.Datacenters, func(names []string) tea.Msg {
		return datacentersMsg{region: region, names: names}
	})
}
