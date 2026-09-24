package ui

import (
	"context"
	"image/color"
	"sort"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// The columns of what a client says about itself.
var (
	nodeEventTitles = []string{"Age", "Subsystem", "Message"}
	driverTitles    = []string{"Driver", "Detected", "Healthy", "Updated", "Description"}
	volumeTitles    = []string{"Name", "Path", "Read only"}
	metaTitles      = []string{"Field", "Value", "Set by"}
)

// The keys each of those screens answers.
var (
	// clientBindings are the keys of the machine, and after them the keys
	// of the allocations on it, which answer here as on any other list of
	// allocations.
	clientBindings = append([]binding{
		{press: "e", label: "Events", do: nodeScreen(screenNodeEvents)},
		// Draining is a key of the list of clients; on the screen of one
		// client the same key opens what it can run.
		{press: "ctrl+d", label: "Drivers", do: nodeScreen(screenNodeDrivers)},
		{press: "ctrl+h", label: "Host volumes", do: nodeScreen(screenNodeVolumes)},
		{press: "a", label: "Attributes", do: nodeScreen(screenNodeAttributes)},
		{press: "m", label: "Meta", do: nodeScreen(screenNodeMeta)},
	}, allocBindings...)

	driverBindings = []binding{{press: "enter", label: "What the driver says", do: Model.open}}

	metaBindings = []binding{
		{press: "c", label: "Copy the value", do: Model.copyField},
		{press: "e", label: "Edit", do: Model.edit},
	}
)

// nodeScreen is a key that opens one of the screens of the client.
func nodeScreen(kind screenKind) func(Model) (Model, tea.Cmd) {
	return func(m Model) (Model, tea.Cmd) { return m.openNodeScreen(kind) }
}

// openNodeScreen opens one of the screens of the client the cursor came
// from. They all read the same answer from the machine.
func (m Model) openNodeScreen(kind screenKind) (Model, tea.Cmd) {
	if !m.screen.isClient() {
		return m, nil
	}

	return m.push(screen{kind: kind, nodeID: m.screen.nodeID, label: m.screen.label})
}

// openDriver opens what one driver says about itself.
func (m Model) openDriver() (Model, tea.Cmd) {
	driver, ok := selectedOf(m, screenNodeDrivers, m.nodeDetail.Drivers)
	if !ok {
		return m, nil
	}

	return m.push(screen{kind: screenNodeDriver, nodeID: m.screen.nodeID, label: driver.Name})
}

// fetchNodeDetail asks the machine for everything it says about itself: the
// screens of a client are different readings of one answer.
func fetchNodeDetail(m Model) tea.Cmd {
	client, nodeID := m.client, m.screen.nodeID

	return request(func(ctx context.Context) (nomad.NodeDetail, error) {
		return client.NodeDetail(ctx, nodeID)
	}, func(detail nomad.NodeDetail) tea.Msg { return nodeDetailMsg(detail) })
}

// fetchNodeMeta asks the machine itself, which is the only place that knows
// which keys came from the API.
func fetchNodeMeta(m Model) tea.Cmd {
	client, nodeID := m.client, m.screen.nodeID

	return fetchList(func(ctx context.Context) ([]nomad.MetaEntry, error) {
		return client.NodeMeta(ctx, nodeID)
	}, func(meta []nomad.MetaEntry) tea.Msg { return nodeMetaMsg{nodeID: nodeID, meta: meta} })
}

func nodeEventRows(events []nomad.NodeEvent) []tableRow {
	rows := make([]tableRow, 0, len(events))

	for _, event := range events {
		rows = append(rows, tableRow{
			cells: []string{ageOf(event.Time), event.Subsystem, event.Message},
			color: eventColor(event),
		})
	}

	return rows
}

// eventColor marks what a client complains about.
func eventColor(event nomad.NodeEvent) color.Color {
	if event.Details["failed"] == "true" {
		return colorDead
	}

	return nil
}

func driverRows(drivers []nomad.Driver) []tableRow {
	rows := make([]tableRow, 0, len(drivers))

	for _, driver := range drivers {
		rows = append(rows, tableRow{
			cells: []string{
				driver.Name,
				yesNo(driver.Detected),
				yesNo(driver.Healthy),
				ageOf(driver.Updated),
				driver.Description,
			},
			color: driverColor(driver),
		})
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

// driverAttributes are what the open driver says about itself.
func (m Model) driverAttributes() map[string]string {
	for _, driver := range m.nodeDetail.Drivers {
		if driver.Name == m.screen.label {
			return driver.Attributes
		}
	}

	return nil
}

// metaFile is the metadata of a client as a file.
func metaFile(client Client, nodeID string) file {
	return func(ctx context.Context) (string, string, error) {
		content, err := client.NodeMetaSpec(ctx, nodeID)

		return "json", content, err
	}
}
