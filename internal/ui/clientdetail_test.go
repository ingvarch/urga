package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func clientDetail() nomad.NodeDetail {
	return nomad.NodeDetail{
		ID:   "node-1",
		Name: "nomad-server-01",
		Events: []nomad.NodeEvent{
			{Time: time.Now().Add(-time.Hour), Subsystem: "Driver", Message: "Driver docker is healthy", Details: map[string]string{"driver": "docker"}},
			{Time: time.Now().Add(-48 * time.Hour), Subsystem: "Cluster", Message: "Node registered"},
		},
		Drivers: []nomad.Driver{
			{Name: "docker", Detected: true, Healthy: true, Description: "Healthy", Updated: time.Now().Add(-time.Hour),
				Attributes: map[string]string{"driver.docker.version": "27.1.1"}},
			{Name: "exec", Description: "Driver must run as root"},
		},
		Volumes: []nomad.HostVolume{
			{Name: "certs", Path: "/etc/ssl/certs", ReadOnly: true},
			{Name: "data", Path: "/srv/data"},
		},
		Attributes: map[string]string{"cpu.arch": "amd64", "os.name": "ubuntu"},
	}
}

func clientMeta() []nomad.MetaEntry {
	return []nomad.MetaEntry{
		{Key: "owner", Value: "igor", Dynamic: true},
		{Key: "role", Value: "bot"},
	}
}

// onAClient opens the screen of one machine, where its own keys live.
func onAClient(t *testing.T) (Model, *fakeClient) {
	t.Helper()

	client := &fakeClient{
		nodes:      busyClient(),
		nodeAllocs: clientAllocs(),
		nodeDetail: clientDetail(),
		nodeMeta:   clientMeta(),
	}

	m, _ := nodeModelOf(client)
	m, cmd := m.update(enter())

	return drain(m, cmd), client
}

// pressed opens what a key of the client screen opens.
func pressed(t *testing.T, msg tea.KeyPressMsg) (Model, *fakeClient) {
	t.Helper()

	m, client := onAClient(t)

	next, cmd := m.update(msg)

	return drain(next, cmd), client
}

func TestClient_ShowsWhatHappenedToIt(t *testing.T) {
	r := require.New(t)

	m, client := pressed(t, key('e'))

	r.Equal(screenNodeEvents, m.screen.kind)
	r.Equal("node-1", client.askedNodeID)

	out := plain(m.render())
	r.Contains(out, "Events (Client: nomad-server-01) [2]")
	r.Contains(out, "Driver docker is healthy")
	r.Contains(out, "Cluster")
}

func TestClient_ShowsWhatItCanRun(t *testing.T) {
	r := require.New(t)

	m, _ := pressed(t, ctrlKey('d'))

	r.Equal(screenNodeDrivers, m.screen.kind)

	out := plain(m.render())
	r.Contains(out, "Drivers (Client: nomad-server-01) [2]")
	r.Contains(out, "docker")
	r.Contains(out, "exec")
	r.Contains(out, "Driver must run as root")
}

func TestClient_ADriverOpensItsOwnDetails(t *testing.T) {
	r := require.New(t)

	m, _ := pressed(t, ctrlKey('d'))

	m, cmd := m.update(enter())
	m = drain(m, cmd)

	r.Equal(screenNodeDriver, m.screen.kind)

	out := plain(m.render())
	r.Contains(out, "Driver docker")
	r.Contains(out, "driver.docker.version")
	r.Contains(out, "27.1.1")

	// What a driver says about itself is worth copying, like any field.
	_, cmd = m.update(key('c'))
	r.Equal("27.1.1", clipboardOf(cmd))
}

func TestClient_ShowsWhatItLendsOut(t *testing.T) {
	r := require.New(t)

	m, _ := pressed(t, ctrlKey('h'))

	r.Equal(screenNodeVolumes, m.screen.kind)

	out := plain(m.render())
	r.Contains(out, "Host volumes (Client: nomad-server-01) [2]")
	r.Contains(out, "/etc/ssl/certs")
	r.Contains(out, "yes")
}

func TestClient_ShowsHowItIsBuilt(t *testing.T) {
	r := require.New(t)

	m, _ := pressed(t, key('a'))

	r.Equal(screenNodeAttributes, m.screen.kind)

	out := plain(m.render())
	r.Contains(out, "Attributes (Client: nomad-server-01) [2]")
	r.Contains(out, "cpu.arch")
	r.Contains(out, "amd64")

	// An attribute is the sort of thing that goes into a constraint, so it
	// copies like a field.
	_, cmd := m.update(key('c'))
	r.Equal("amd64", clipboardOf(cmd))
}

func TestClient_ShowsItsMetadataAndWhereItComesFrom(t *testing.T) {
	r := require.New(t)

	m, _ := pressed(t, key('m'))

	r.Equal(screenNodeMeta, m.screen.kind)

	out := plain(m.render())
	r.Contains(out, "Meta (Client: nomad-server-01) [2]")
	r.Contains(out, "owner")
	r.Contains(out, "igor")

	// Only what the API set can be changed from here, so the screen says
	// which keys those are.
	r.Contains(out, "api")
	r.Contains(out, "agent")
}

func TestClient_EditsTheMetadata(t *testing.T) {
	r := require.New(t)

	editor := &fakeEditor{replace: `{"owner": "ingvar"}`}

	client := &fakeClient{
		nodes:      busyClient(),
		nodeAllocs: clientAllocs(),
		nodeDetail: clientDetail(),
		nodeMeta:   clientMeta(),
		metaSpec:   `{"owner": "igor"}`,
	}

	m := New(client, Options{Namespace: "production", Version: "v-test", Editor: editor, PollEvery: time.Millisecond})
	m, _ = m.update(sizeMsg())
	m, _ = m.update(key(':'))
	m = typeIn(m, "clients")
	m, _ = m.update(enter())
	m, _ = m.update(nodesMsg(busyClient()))

	m, cmd := m.update(enter())
	m = drain(m, cmd)

	m, cmd = m.update(key('m'))
	m = drain(m, cmd)

	m, cmd = m.update(key('e'))
	follow(m, cmd, 5)

	// What the API can set is what the editor was given, and what comes
	// back goes to the machine.
	r.NotEmpty(editor.opened)
	r.Equal(`{"owner": "ingvar"}`, client.metaSubmitted)
	r.Equal("node-1", client.askedNodeID)
}
