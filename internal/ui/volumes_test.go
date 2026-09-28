package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// twoVolumes are a CSI volume one allocation writes to, and a host volume on
// a node.
func twoVolumes() []nomad.Volume {
	return []nomad.Volume{
		{
			ID: "pg-data", Name: "pg-data", Namespace: "production", Kind: nomad.VolumeCSI, PluginID: "aws-ebs0",
			State: "schedulable", Writers: 1,
			Controllers: nomad.Health{Healthy: 2, Expected: 2}, PluginNodes: nomad.Health{Healthy: 6, Expected: 6},
			Created: time.Now().Add(-72 * time.Hour),
		},
		{
			ID: "7f3c1a2b-0000-0000-0000-000000000000", Name: "scratch", Namespace: "production", Kind: nomad.VolumeHost,
			PluginID: "mkdir", NodeID: "node-1", NodeName: "node-01", NodePool: "default",
			CapacityBytes: 1 << 30, State: "ready", Created: time.Now().Add(-2 * time.Hour),
		},
	}
}

// onVolumes is the volume list, opened by name.
func onVolumes(t *testing.T, client *fakeClient) Model {
	t.Helper()

	return typeCommand(newTestModel(client), "volumes")
}

func TestVolumes_TheList(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{volumes: twoVolumes()}
	m := onVolumes(t, client)

	r.IsType(volumesPage{}, m.screen.page)
	r.Equal("Volumes (production) [2]", m.title())
	r.Equal("production", client.askedNamespace)

	rows := m.rows()
	r.Len(rows, 2)

	// A CSI volume lives on no node, and the list does not say how big it
	// is; its claims are how many allocations write to it and read it.
	r.Equal([]string{"pg-data", "pg-data", "csi", "production", "aws-ebs0", "-", "-", "schedulable", "1W 0R", "3d"}, rows[0].cells)

	// A host volume has an ID made up by the cluster, and a node.
	r.Equal([]string{"7f3c1a2b", "scratch", "host", "production", "mkdir", "node-01", "1.0 GiB", "ready", "-", "2h"}, rows[1].cells)
}

func TestVolumes_TheKeys(t *testing.T) {
	r := require.New(t)

	m := onVolumes(t, &fakeClient{volumes: twoVolumes()})

	r.Equal([]hint{
		{Key: "<enter>", Description: "Details"},
		{Key: "<d>", Description: "Describe"},
	}, m.hints())
}

func TestVolumes_OpenedByTheirOtherNames(t *testing.T) {
	r := require.New(t)

	for _, name := range []string{"volume", "vol"} {
		m := typeCommand(newTestModel(&fakeClient{volumes: twoVolumes()}), name)
		r.IsType(volumesPage{}, m.screen.page, name)
	}
}

func TestVolumes_Describe(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{volumes: twoVolumes(), describe: `{"ID": "pg-data"}`}
	m := onVolumes(t, client)

	m, cmd := m.update(key('d'))
	m = playOut(m, cmd)

	r.IsType(describePage{}, m.screen.page)
	r.Equal("Volume: pg-data", m.title())
	r.Equal("production/csi/pg-data", client.volumeOf)
}

func TestVolumeColor(t *testing.T) {
	r := require.New(t)

	healthy := nomad.Health{Healthy: 2, Expected: 2}
	csi := func(state string, controllers, nodes nomad.Health) nomad.Volume {
		return nomad.Volume{Kind: nomad.VolumeCSI, State: state, Controllers: controllers, PluginNodes: nodes}
	}

	r.Nil(volumeColor(csi("schedulable", healthy, healthy)))
	r.Equal(colorDead, volumeColor(csi("unschedulable", healthy, healthy)))

	// A plugin short of healthy instances may fail to attach the volume.
	r.Equal(colorAttention, volumeColor(csi("schedulable", nomad.Health{Healthy: 1, Expected: 2}, healthy)))
	r.Equal(colorAttention, volumeColor(csi("schedulable", healthy, nomad.Health{Healthy: 5, Expected: 6})))

	host := func(state string) nomad.Volume { return nomad.Volume{Kind: nomad.VolumeHost, State: state} }

	r.Nil(volumeColor(host("ready")))
	r.Equal(colorPending, volumeColor(host("pending")))
	r.Equal(colorDead, volumeColor(host("unavailable")))
}

// csiDetail is the CSI volume of twoVolumes, read in full: one allocation
// on node-03 writes to it.
func csiDetail() nomad.VolumeDetail {
	v := twoVolumes()[0]
	v.CapacityBytes = 10 << 30

	return nomad.VolumeDetail{
		Volume:   v,
		Provider: "ebs.csi.aws.com", ProviderVersion: "1.2.0",
		AccessMode: "single-node-writer", AttachmentMode: "file-system",
		ExternalID: "vol-0abc123", Topologies: []string{"region=eu-west-1, zone=eu-west-1a"},
		Allocs: []nomad.Alloc{{
			ID: "9a1b2c3d-0000-0000-0000-000000000000", Namespace: "production", JobID: "db", TaskGroup: "db",
			NodeID: "node-3", NodeName: "node-03", Status: "running",
			Tasks: []nomad.Task{{Name: "postgres", State: "running"}},
		}},
	}
}

// hostDetail is the host volume of twoVolumes, read in full: the cache group
// of web claims it.
func hostDetail() nomad.VolumeDetail {
	return nomad.VolumeDetail{
		Volume:   twoVolumes()[1],
		HostPath: "/opt/nomad/volumes/7f3c1a2b", CapacityMin: 1 << 30, CapacityMax: 2 << 30,
		Claim: nomad.Claim{ID: "claim-1", JobID: "web", TaskGroup: "cache"},
		Allocs: []nomad.Alloc{{
			ID: "4f2a1c9e-0000-0000-0000-000000000000", Namespace: "production", JobID: "web", TaskGroup: "cache",
			NodeID: "node-1", NodeName: "node-01", Status: "running",
		}},
	}
}

// onVolume opens the volume the cursor is on after down presses.
func onVolume(t *testing.T, client *fakeClient, down int) Model {
	t.Helper()

	m := onVolumes(t, client)
	for range down {
		m, _ = m.update(key('j'))
	}

	m, cmd := m.update(enter())

	return playOut(m, cmd)
}

// panelOf is the panel of the open page, as text.
func panelOf(t *testing.T, m Model) string {
	t.Helper()

	p, ok := m.screen.page.(panelled)
	require.True(t, ok, "%T has no panel", m.screen.page)

	return plain(strings.Join(p.panel(m.env(), 160, 20), "\n"))
}

func TestVolume_CSI(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{volumes: twoVolumes(), volume: csiDetail()}
	m := onVolume(t, client, 0)

	r.IsType(volumePage{}, m.screen.page)
	r.Equal("production/csi/pg-data", client.volumeOf)
	r.Equal("Volume pg-data (csi) [1]", m.title())

	panel := panelOf(t, m)
	for _, want := range []string{
		"Type csi", "Plugin aws-ebs0 (ebs.csi.aws.com 1.2.0)", "Controllers 2/2", "Nodes 6/6",
		"Access single-node-writer, file-system", "Capacity 10.0 GiB", "Schedulable yes", "Claims 1W 0R",
		"External ID vol-0abc123", "Topology region=eu-west-1, zone=eu-west-1a",
	} {
		r.Contains(panel, want)
	}

	// The allocations that use the volume, with the keys of any list of
	// allocations.
	r.Contains(plain(m.render()), "9a1b2c3d")
	r.Contains(m.hints(), hint{Key: "<enter>", Description: "Tasks"})
}

func TestVolume_Host(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{volumes: twoVolumes(), volume: hostDetail()}
	m := onVolume(t, client, 1)

	r.Equal("production/host/7f3c1a2b-0000-0000-0000-000000000000", client.volumeOf)
	r.Equal("Volume scratch (host) [1]", m.title())

	panel := panelOf(t, m)
	for _, want := range []string{
		"Type host", "Plugin mkdir", "State ready",
		"Node node-01", "Pool default", "Path /opt/nomad/volumes/7f3c1a2b",
		"Capacity 1.0 GiB", "Asked 1.0 GiB - 2.0 GiB", "Claim web / cache",
	} {
		r.Contains(panel, want)
	}
}

func TestVolume_TheKeysOfEachKind(t *testing.T) {
	r := require.New(t)

	detach := hint{Key: "<ctrl-d>", Description: "Detach"}
	release := hint{Key: "<ctrl-r>", Description: "Release Claim"}

	m := onVolume(t, &fakeClient{volumes: twoVolumes(), volume: csiDetail()}, 0)
	r.Contains(m.hints(), detach)
	r.NotContains(m.hints(), release)

	m = onVolume(t, &fakeClient{volumes: twoVolumes(), volume: hostDetail()}, 1)
	r.Contains(m.hints(), release)
	r.NotContains(m.hints(), detach)

	// A host volume nobody claims has nothing to release.
	unclaimed := hostDetail()
	unclaimed.Claim = nomad.Claim{}

	m = onVolume(t, &fakeClient{volumes: twoVolumes(), volume: unclaimed}, 1)
	r.NotContains(m.hints(), release)
}

func TestVolume_Detach(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{volumes: twoVolumes(), volume: csiDetail()}
	m := onVolume(t, client, 0)

	m, _ = m.update(ctrlKey('d'))
	r.Contains(plain(m.render()), "Really detach pg-data from node-03?")

	m, cmd := m.update(key('y'))
	m = playOut(m, cmd)

	// From the node of the allocation under the cursor.
	r.Equal([]string{"DetachVolume"}, client.writes)
	r.Equal("pg-data@node-3", client.detached)
	r.Equal("production", client.askedNamespace)
	r.Contains(plain(m.render()), "Volume pg-data detached from node-03.")
}

func TestVolume_ReleaseClaim(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{volumes: twoVolumes(), volume: hostDetail()}
	m := onVolume(t, client, 1)

	m, _ = m.update(ctrlKey('r'))
	r.Contains(plain(m.render()), "Really release the claim of web / cache on scratch?")

	m, cmd := m.update(key('y'))
	m = playOut(m, cmd)

	r.Equal([]string{"ReleaseClaim"}, client.writes)
	r.Equal("claim-1", client.released)
	r.Equal("production", client.askedNamespace)
	r.Contains(plain(m.render()), "Claim of web / cache released.")
}

func TestVolume_StaysInItsNamespace(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{volumes: twoVolumes(), volume: csiDetail()}
	m := onVolume(t, client, 0)
	m.namespaceOrder = []string{"production", "staging"}

	m, cmd := m.update(key('2'))
	m = playOut(m, cmd)

	r.Equal("staging", m.namespace)
	r.IsType(volumePage{}, m.screen.page)
	r.Equal("production/csi/pg-data", client.volumeOf)
}

func TestVolume_TheLogsOfItsAllocations(t *testing.T) {
	r := require.New(t)

	// Two tasks to pick from: with one, its logs open at once.
	volume := csiDetail()
	volume.Allocs[0].Tasks = []nomad.Task{{Name: "postgres", State: "running"}, {Name: "backup", State: "running"}}

	client := &fakeClient{volumes: twoVolumes(), volume: volume}
	m := onVolume(t, client, 0)
	client.volumeOf = ""

	m, cmd := m.update(key('l'))
	m = playOut(m, cmd)

	// The tasks to pick from are those of the allocations of the volume.
	r.IsType(logTasksPage{}, m.screen.page)
	r.Equal("production/csi/pg-data", client.volumeOf)
	r.Equal("Logs of which task? (Volume: pg-data)", m.title())

	out := plain(m.render())
	r.Contains(out, "postgres")
	r.Contains(out, "backup")
}

func TestVolume_AnotherVolumeIsNotTaken(t *testing.T) {
	r := require.New(t)

	m := onVolume(t, &fakeClient{volumes: twoVolumes(), volume: csiDetail()}, 0)

	other := hostDetail()
	m, _ = m.update(volumeMsg(other))

	r.Equal("Volume pg-data (csi) [1]", m.title())
	r.NotContains(panelOf(t, m), "scratch")
	r.Contains(panelOf(t, m), "Type csi")
}

func TestVolume_ACapacityNotKnownIsLeftOut(t *testing.T) {
	r := require.New(t)

	// A plugin that does not say how big the volume is.
	unsized := hostDetail()
	unsized.CapacityBytes, unsized.CapacityMin, unsized.CapacityMax = 0, 0, 0

	m := onVolume(t, &fakeClient{volumes: twoVolumes(), volume: unsized}, 1)

	panel := panelOf(t, m)
	r.NotContains(panel, "Capacity")
	r.NotContains(panel, "Asked")
}
