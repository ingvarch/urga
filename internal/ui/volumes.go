package ui

import (
	"context"
	"fmt"
	"image/color"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// volumesMsg is the volumes of a namespace.
type volumesMsg []nomad.Volume

// volumesTitles are the columns of the volume list: CSI and host volumes in
// one table, each with what it has.
var volumesTitles = []string{"ID", "Name", "Type", "Namespace", "Plugin", "Node", "Capacity", "State", "Claims", "Age"}

// volumesPage is the CSI and host volumes of the namespace the session looks
// at.
type volumesPage struct {
	ofTheSession

	volumes []nomad.Volume
}

func (volumesPage) title(e env, count int) string {
	return sprintf("Volumes (%s) [%d]", namespaceLabel(e.namespace), count)
}

func (volumesPage) titles() []string { return volumesTitles }

// topics: none. The cluster reports no change of a CSI volume, and a topic
// for half the list would leave the other half stale; the list is polled.
func (volumesPage) topics() []string { return nil }

func (volumesPage) fetch(e env) tea.Cmd {
	client, namespace := e.client, e.namespace

	return fetchList(func(ctx context.Context) ([]nomad.Volume, error) {
		return client.Volumes(ctx, namespace)
	}, func(items []nomad.Volume) tea.Msg { return volumesMsg(items) })
}

func (p volumesPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	volumes, ok := msg.(volumesMsg)
	if !ok {
		return p, outcome{}, false
	}

	p.volumes = volumes

	return p, outcome{}, true
}

func (p volumesPage) rows(env) []tableRow {
	rows := make([]tableRow, 0, len(p.volumes))

	for _, v := range p.volumes {
		row := tableRow{color: volumeColor(v)}
		row.add(volumeID(v), v.Name, v.Kind, v.Namespace, v.PluginID, dashed(v.NodeName), dashed(knownSize(v.CapacityBytes)), v.State, claimsOf(v))
		row.addAge(v.Created)

		rows = append(rows, row)
	}

	return rows
}

// picked is the volume under the cursor.
func (p volumesPage) picked(e env) (nomad.Volume, bool) { return pickedFrom(e, p.volumes) }

var volumesKeys = []pageKey[volumesPage]{
	{press: "enter", label: "Details", do: openVolume},
	{press: "d", label: "Describe", do: describeVolume},
}

func (p volumesPage) keys(e env) []keyHint { return hintsOf(p, e, volumesKeys) }

func (p volumesPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, volumesKeys, k)
}

// openVolume opens the volume under the cursor: what it is, above the
// allocations that use it.
func openVolume(p volumesPage, e env) (volumesPage, outcome) {
	v, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg{volumePage{namespace: v.Namespace, kind: v.Kind, id: v.ID, name: v.Name}})
}

// describeVolume shows the volume under the cursor as the cluster has it.
func describeVolume(p volumesPage, e env) (volumesPage, outcome) {
	v, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	client := e.client

	return p, outcome{cmd: describe(fmt.Sprintf("Volume: %s", v.Name), func(ctx context.Context) (string, error) {
		return client.DescribeVolume(ctx, v.Namespace, v.Kind, v.ID)
	})}
}

// volumeID is how a volume is named in its row: a host volume has an ID the
// cluster made up, shortened like any other; a CSI volume has the one it was
// registered with.
func volumeID(v nomad.Volume) string {
	if v.Kind == nomad.VolumeHost {
		return shortID(v.ID)
	}

	return v.ID
}

// claimsOf is how many allocations write to a CSI volume and read it. A host
// volume does not say in the list.
func claimsOf(v nomad.Volume) string {
	if v.Kind != nomad.VolumeCSI {
		return "-"
	}

	return fmt.Sprintf("%dW %dR", v.Writers, v.Readers)
}

// knownSize is a size, empty when it is not known: the list shows a dash,
// the panel leaves the field out.
func knownSize(bytes int64) string {
	if bytes <= 0 {
		return ""
	}

	return sizeOf(bytes)
}

// dashed is a value, or a dash for none.
func dashed(value string) string {
	if value == "" {
		return "-"
	}

	return value
}

// volumeColor shows a volume nobody can use in red, one on its way in
// yellow, and a CSI volume whose plugin is short of healthy instances in the
// attention color: attaching it may fail.
func volumeColor(v nomad.Volume) color.Color {
	switch v.State {
	case "unschedulable", "unavailable":
		return colorDead
	case "pending":
		return colorPending
	}

	if short(v.Controllers) || short(v.PluginNodes) {
		return colorAttention
	}

	return nil
}

// short says fewer instances of a plugin are healthy than expected.
func short(h nomad.Health) bool { return h.Healthy < h.Expected }

// volumeMsg is one volume read in full.
type volumeMsg nomad.VolumeDetail

// volumePage is one volume: what it is, above the allocations that use it.
type volumePage struct {
	namespace, kind, id, name string

	// volume is the volume as the page last read it; read says it has. Until
	// then there is no panel, and no key of the volume.
	volume nomad.VolumeDetail
	read   bool
}

func (p volumePage) title(_ env, count int) string {
	return sprintf("Volume %s (%s) [%d]", p.name, p.kind, count)
}

func (volumePage) titles() []string { return allocTitles }

// topics: none, as for the list.
func (volumePage) topics() []string { return nil }

// where: a volume lives in its namespace, whatever the session looks at.
func (p volumePage) where(env) string { return p.namespace }

func (p volumePage) fetch(e env) tea.Cmd {
	client, namespace, kind, id := e.client, p.namespace, p.kind, p.id

	return request(func(ctx context.Context) (nomad.VolumeDetail, error) {
		return client.Volume(ctx, namespace, kind, id)
	}, func(v nomad.VolumeDetail) tea.Msg { return volumeMsg(v) })
}

// take keeps the volume the page is open on.
func (p volumePage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	v, ok := msg.(volumeMsg)
	if !ok || v.ID != p.id {
		return p, outcome{}, false
	}

	p.volume, p.read = nomad.VolumeDetail(v), true

	return p, outcome{}, true
}

func (p volumePage) panel(_ env, width, room int) []string {
	if !p.read {
		return nil
	}

	return volumePanel(p.volume, width, room)
}

func (p volumePage) rows(e env) []tableRow { return allocRows(p.volume.Allocs, e.usage) }

func (p volumePage) visible(env) []nomad.Alloc { return p.volume.Allocs }

func (p volumePage) ids(env) []string { return names(p.volume.Allocs, allocMark) }

func (p volumePage) readings(e env) []rowRef { return runningRefs(e.index, p.volume.Allocs) }

func (volumePage) reading(ctx context.Context, client Client, ref rowRef) (nomad.ResourceUse, error) {
	return allocReading(ctx, client, ref)
}

// logsOf: the logs of the allocations that use the volume.
func (p volumePage) logsOf(env) logScope {
	return logScope{namespace: p.namespace, volumeKind: p.kind, volumeID: p.id, label: p.name}
}

// volumeKeys are the actions of the volume, then those of its allocations.
var volumeKeys = append([]pageKey[volumePage]{
	{press: "ctrl+d", label: "Detach", do: detachVolume, writes: true, offered: attachedHere},
	{press: "ctrl+r", label: "Release Claim", do: releaseClaim, writes: true, offered: claimed},
}, allocKeys[volumePage]()...)

func (p volumePage) keys(e env) []keyHint { return hintsOf(p, e, volumeKeys) }

func (p volumePage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, volumeKeys, k)
}

// attachedHere says a CSI volume is claimed by the allocation under the
// cursor, whose node it can be detached from.
func attachedHere(p volumePage, e env) bool {
	_, ok := pickedFrom(e, p.volume.Allocs)

	return ok && p.volume.Kind == nomad.VolumeCSI
}

// claimed says a task group claims the host volume.
func claimed(p volumePage, _ env) bool {
	return p.volume.Kind == nomad.VolumeHost && p.volume.Claim.ID != ""
}

// detachVolume detaches the volume from the node of the allocation under the
// cursor: a node that is gone leaves its claim behind, and the volume cannot
// be attached anywhere else until it is released.
func detachVolume(p volumePage, e env) (volumePage, outcome) {
	alloc, ok := pickedFrom(e, p.volume.Allocs)
	if !ok {
		return p, outcome{}
	}

	client, v := e.client, p.volume
	node := dashed(alloc.NodeName)

	return p, then(askMsg{
		question: fmt.Sprintf("Really detach %s from %s?", v.Name, node),
		apply: act(fmt.Sprintf("Volume %s detached from %s.", v.Name, node), func(ctx context.Context) error {
			return client.DetachVolume(ctx, v.Namespace, v.ID, alloc.NodeID)
		}),
	})
}

// releaseClaim deletes the claim of a task group on the host volume: its next
// allocation may be placed with another volume.
func releaseClaim(p volumePage, e env) (volumePage, outcome) {
	client, v := e.client, p.volume
	owner := v.Claim.JobID + " / " + v.Claim.TaskGroup

	return p, then(askMsg{
		question: fmt.Sprintf("Really release the claim of %s on %s?", owner, v.Name),
		apply: act(fmt.Sprintf("Claim of %s released.", owner), func(ctx context.Context) error {
			return client.ReleaseClaim(ctx, v.Namespace, v.Claim.ID)
		}),
	})
}

// volumePanel is what the page shows above the allocations: for a CSI volume
// its plugin, how it is attached and where it can be reached; for a host
// volume its node, its directory and who claims it.
func volumePanel(v nomad.VolumeDetail, width, room int) []string {
	lines := hostPanelLines(v)
	if v.Kind == nomad.VolumeCSI {
		lines = csiPanelLines(v)
	}

	rows := []string{}

	for _, line := range lines {
		if row := fieldLine(line, width-1); row != "" {
			rows = append(rows, " "+row)
		}
	}

	return fitPanel(rows, panelBlock{}, room)
}

func csiPanelLines(v nomad.VolumeDetail) [][]field {
	plugin := v.PluginID
	if v.Provider != "" {
		plugin += " (" + strings.TrimSpace(v.Provider+" "+v.ProviderVersion) + ")"
	}

	access := strings.Join(slices.DeleteFunc([]string{v.AccessMode, v.AttachmentMode}, isEmpty), ", ")

	return [][]field{
		{
			{"Type", v.Kind},
			{"Plugin", plugin},
			{"Controllers", healthOf(v.Controllers)},
			{"Nodes", healthOf(v.PluginNodes)},
		},
		{
			{"Access", access},
			{"Capacity", knownSize(v.CapacityBytes)},
			{"Schedulable", yesNo(v.State == "schedulable")},
			{"Claims", claimsOf(v.Volume)},
		},
		{
			{"External ID", v.ExternalID},
			{"Topology", strings.Join(v.Topologies, "; ")},
		},
	}
}

func hostPanelLines(v nomad.VolumeDetail) [][]field {
	claim := ""
	if v.Claim.ID != "" {
		claim = v.Claim.JobID + " / " + v.Claim.TaskGroup
	}

	return [][]field{
		{
			{"Type", v.Kind},
			{"Plugin", v.PluginID},
			{"State", v.State},
		},
		{
			{"Node", v.NodeName},
			{"Pool", v.NodePool},
			{"Path", v.HostPath},
		},
		{
			{"Capacity", knownSize(v.CapacityBytes)},
			{"Asked", askedCapacity(v.CapacityMin, v.CapacityMax)},
			{"Claim", claim},
		},
	}
}

// askedCapacity is the capacity a host volume asked for: at least, or
// between. Empty when it asked for none.
func askedCapacity(least, most int64) string {
	switch {
	case least > 0 && most > 0:
		return sizeOf(least) + " - " + sizeOf(most)
	case least > 0:
		return sizeOf(least)
	case most > 0:
		return "up to " + sizeOf(most)
	}

	return ""
}

// healthOf is how many instances are healthy, out of how many.
func healthOf(h nomad.Health) string { return fmt.Sprintf("%d/%d", h.Healthy, h.Expected) }

func isEmpty(s string) bool { return s == "" }
