package ui

import (
	"context"
	"fmt"
	"image/color"
	"sort"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// The columns of the detail screens of a client.
var (
	nodeEventTitles = []string{"Age", "Subsystem", "Message"}
	driverTitles    = []string{"Driver", "Detected", "Healthy", "Updated", "Description"}
	volumeTitles    = []string{"Name", "Path", "Read only"}
	metaTitles      = []string{"Field", "Value", "Set by"}
)

// machine is the client a screen was opened for, and the detail last read
// from it: each screen of a client shows a part of the same answer.
type machine struct {
	nodeID, name string
	detail       nomad.NodeDetail
}

// topics: the event stream carries nothing for these screens, so they are
// polled on a timer.
func (machine) topics() []string { return nil }

// fetch requests the full detail of the client.
func (d machine) fetch(e env) tea.Cmd {
	client, nodeID := e.client, d.nodeID

	return request(func(ctx context.Context) (nomad.NodeDetail, error) {
		return client.NodeDetail(ctx, nodeID)
	}, func(detail nomad.NodeDetail) tea.Msg { return nodeDetailMsg(detail) })
}

// took keeps the detail read from the client. An answer for another client
// is ignored, so moving from one client to the next never shows the first
// one's data under the second's name.
func (d machine) took(msg tea.Msg) (machine, bool) {
	detail, ok := msg.(nodeDetailMsg)
	if !ok || detail.ID != d.nodeID {
		return d, false
	}

	d.detail = nomad.NodeDetail(detail)

	return d, true
}

// nodeScreen is a key that opens one of the screens of the client, empty
// until its detail arrives.
func nodeScreen(open func(machine) page) func(clientPage, env) (clientPage, outcome) {
	return func(p clientPage, _ env) (clientPage, outcome) {
		return p, then(openMsg{open(machine{nodeID: p.nodeID, name: p.name})})
	}
}

// nodeEventsPage is what happened to the machine.
type nodeEventsPage struct {
	noKeys
	machine
}

func (p nodeEventsPage) title(_ env, count int) string {
	return sprintf("Events (Client: %s) [%d]", p.name, count)
}

func (nodeEventsPage) titles() []string { return nodeEventTitles }

func (p nodeEventsPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	var ok bool
	p.machine, ok = p.took(msg)

	return p, outcome{}, ok
}

func (p nodeEventsPage) rows(env) []tableRow { return nodeEventRows(p.detail.Events) }

// nodeDriversPage is what the machine can run.
type nodeDriversPage struct {
	machine
}

func (p nodeDriversPage) title(_ env, count int) string {
	return sprintf("Drivers (Client: %s) [%d]", p.name, count)
}

func (nodeDriversPage) titles() []string { return driverTitles }

func (p nodeDriversPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	var ok bool
	p.machine, ok = p.took(msg)

	return p, outcome{}, ok
}

func (p nodeDriversPage) rows(env) []tableRow { return driverRows(p.detail.Drivers) }

var nodeDriversKeys = []pageKey[nodeDriversPage]{{press: "enter", label: "Details", do: openDriver}}

func (p nodeDriversPage) keys(e env) []keyHint { return hintsOf(p, e, nodeDriversKeys) }

func (p nodeDriversPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, nodeDriversKeys, k)
}

// openDriver opens the attributes of one driver. Until the client answers
// again, it shows the detail the driver list was read with.
func openDriver(p nodeDriversPage, e env) (nodeDriversPage, outcome) {
	driver, ok := pickedFrom(e, p.detail.Drivers)
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg{nodeDriverPage{machine: p.machine, driver: driver.Name}})
}

// nodeDriverPage is the attributes one driver of the client reports.
type nodeDriverPage struct {
	machine
	driver string
}

func (p nodeDriverPage) title(_ env, count int) string {
	return sprintf("Driver %s [%d]", p.driver, count)
}

func (nodeDriverPage) titles() []string { return fieldTitles }

func (p nodeDriverPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	var ok bool
	p.machine, ok = p.took(msg)

	return p, outcome{}, ok
}

func (p nodeDriverPage) rows(env) []tableRow { return fieldRows(p.attributes()) }

// attributes are the attributes the driver of the page reports.
func (p nodeDriverPage) attributes() map[string]string {
	for _, driver := range p.detail.Drivers {
		if driver.Name == p.driver {
			return driver.Attributes
		}
	}

	return nil
}

// A driver attribute is worth copying, like any field.
var nodeDriverKeys = []pageKey[nodeDriverPage]{copyKey[nodeDriverPage]()}

func (p nodeDriverPage) keys(e env) []keyHint { return hintsOf(p, e, nodeDriverKeys) }

func (p nodeDriverPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, nodeDriverKeys, k)
}

// nodeVolumesPage is the host volumes the client offers to jobs.
type nodeVolumesPage struct {
	noKeys
	machine
}

func (p nodeVolumesPage) title(_ env, count int) string {
	return sprintf("Host volumes (Client: %s) [%d]", p.name, count)
}

func (nodeVolumesPage) titles() []string { return volumeTitles }

func (p nodeVolumesPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	var ok bool
	p.machine, ok = p.took(msg)

	return p, outcome{}, ok
}

func (p nodeVolumesPage) rows(env) []tableRow { return volumeRows(p.detail.Volumes) }

// nodeAttributesPage is the attributes of the client: CPU, OS, versions.
type nodeAttributesPage struct {
	machine
}

func (p nodeAttributesPage) title(_ env, count int) string {
	return sprintf("Attributes (Client: %s) [%d]", p.name, count)
}

func (nodeAttributesPage) titles() []string { return fieldTitles }

func (p nodeAttributesPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	var ok bool
	p.machine, ok = p.took(msg)

	return p, outcome{}, ok
}

func (p nodeAttributesPage) rows(env) []tableRow { return fieldRows(p.detail.Attributes) }

// An attribute is the sort of thing that goes into a constraint, so it
// copies like a field.
var nodeAttributesKeys = []pageKey[nodeAttributesPage]{copyKey[nodeAttributesPage]()}

func (p nodeAttributesPage) keys(e env) []keyHint { return hintsOf(p, e, nodeAttributesKeys) }

func (p nodeAttributesPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, nodeAttributesKeys, k)
}

// nodeMetaPage is the metadata the machine carries, and where each key of
// it came from.
type nodeMetaPage struct {
	nodeID, name string

	meta []nomad.MetaEntry
}

func (p nodeMetaPage) title(_ env, count int) string {
	return sprintf("Meta (Client: %s) [%d]", p.name, count)
}

func (nodeMetaPage) titles() []string { return metaTitles }
func (nodeMetaPage) topics() []string { return nil }

// fetch asks the machine itself, which is the only place that knows which
// keys came from the API.
func (p nodeMetaPage) fetch(e env) tea.Cmd {
	client, nodeID := e.client, p.nodeID

	return fetchList(func(ctx context.Context) ([]nomad.MetaEntry, error) {
		return client.NodeMeta(ctx, nodeID)
	}, func(meta []nomad.MetaEntry) tea.Msg { return nodeMetaMsg{nodeID: nodeID, meta: meta} })
}

// take keeps the metadata of the machine the page is open on.
func (p nodeMetaPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	meta, ok := msg.(nodeMetaMsg)
	if !ok || meta.nodeID != p.nodeID {
		return p, outcome{}, false
	}

	p.meta = meta.meta

	return p, outcome{}, true
}

func (p nodeMetaPage) rows(env) []tableRow { return metaRows(p.meta) }

// The value is what copies, not the column that says where it came from.
var nodeMetaKeys = []pageKey[nodeMetaPage]{
	copyKey[nodeMetaPage](),
	{press: "e", label: "Edit", do: editMeta, writes: true},
}

func (p nodeMetaPage) keys(e env) []keyHint { return hintsOf(p, e, nodeMetaKeys) }

func (p nodeMetaPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, nodeMetaKeys, k)
}

func nodeEventRows(events []nomad.NodeEvent) []tableRow {
	rows := make([]tableRow, 0, len(events))

	for _, event := range events {
		row := tableRow{color: eventColor(event)}
		row.addAge(event.Time)
		row.add(event.Subsystem, event.Message)

		rows = append(rows, row)
	}

	return rows
}

// eventColor marks an event that reports a failure.
func eventColor(event nomad.NodeEvent) color.Color {
	if event.Details["failed"] == "true" {
		return colorDead
	}

	return nil
}

func driverRows(drivers []nomad.Driver) []tableRow {
	rows := make([]tableRow, 0, len(drivers))

	for _, driver := range drivers {
		row := tableRow{color: driverColor(driver)}
		row.add(driver.Name, yesNo(driver.Detected), yesNo(driver.Healthy))
		row.addAge(driver.Updated)
		row.add(driver.Description)

		rows = append(rows, row)
	}

	return rows
}

// driverColor says whether the driver is there and working. A driver the
// machine never found is not a problem, one it found and cannot use is.
func driverColor(driver nomad.Driver) color.Color {
	switch {
	case !driver.Detected:
		return colorSpent
	case !driver.Healthy:
		return colorDead
	}

	return nil
}

func volumeRows(volumes []nomad.HostVolume) []tableRow {
	rows := make([]tableRow, 0, len(volumes))

	for _, volume := range volumes {
		rows = append(rows, tableRow{cells: []string{volume.Name, volume.Path, yesNo(volume.ReadOnly)}})
	}

	return rows
}

// fieldRows are a map as a list of fields, in the order of the keys: a map
// hands them over differently every time.
func fieldRows(values map[string]string) []tableRow {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	rows := make([]tableRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, tableRow{cells: []string{key, values[key]}})
	}

	return rows
}

// metaRows say where each key comes from, because only what the API set can
// be changed from here.
func metaRows(meta []nomad.MetaEntry) []tableRow {
	rows := make([]tableRow, 0, len(meta))

	for _, entry := range meta {
		source, paint := "agent", colorMuted

		if entry.Dynamic {
			source, paint = "api", color.Color(nil)
		}

		rows = append(rows, tableRow{cells: []string{entry.Key, entry.Value, source}, color: paint})
	}

	return rows
}

// metaFile is the metadata of a client as a file.
func metaFile(client nodesClient, nodeID, name string) load {
	return func(ctx context.Context) (file, error) {
		content, err := client.NodeMetaSpec(ctx, nodeID)

		return file{extension: "json", content: content, submit: reopening("json", func(source string) tea.Cmd {
			return act(fmt.Sprintf("Metadata of %s submitted.", name), func(ctx context.Context) error {
				return client.SubmitNodeMeta(ctx, nodeID, source)
			})
		})}, err
	}
}
