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

	// The column holds how long ago it was, which is what the rest of the
	// screens call that.
	r.Contains(out, "Age")
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

func TestClient_DriversGoByWhenTheyWereUpdated(t *testing.T) {
	r := require.New(t)

	m, client := onAClient(t)
	client.nodeDetail.Drivers = []nomad.Driver{
		{Name: "exec", Updated: time.Now().Add(-30 * time.Minute)},
		{Name: "docker", Updated: time.Now().Add(-time.Hour)},
	}

	m, cmd := m.update(ctrlKey('d'))
	m = drain(m, cmd)

	m, _ = m.update(key('U'))

	// Read as text, "1h" comes before "30m" and is older.
	r.Equal([]string{"30m", "1h"}, cellsOf(m.list.table, 3))
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

	// The value is what copies, not the column that says where it came
	// from: this screen carries one more column than the other field
	// screens.
	_, cmd := m.update(key('c'))
	r.Equal("igor", clipboardOf(cmd))
}

// onMeta is the metadata of the client, with the editor at hand.
func onMeta(t *testing.T, client *fakeClient, editor *fakeEditor) Model {
	t.Helper()

	client.nodes, client.nodeAllocs = busyClient(), clientAllocs()
	client.nodeDetail, client.nodeMeta = clientDetail(), clientMeta()
	client.metaSpec = `{"owner": "igor"}`

	m := New(client, Options{Namespace: "production", Version: "v-test", Editor: editor, PollEvery: time.Millisecond})
	m, _ = m.update(sizeMsg())
	m, _ = m.update(key(':'))
	m = typeIn(m, "clients")
	m, _ = m.update(enter())
	m, _ = m.update(nodesMsg(busyClient()))

	m, cmd := m.update(enter())
	m = drain(m, cmd)

	m, cmd = m.update(key('m'))

	return drain(m, cmd)
}

func TestClient_EditsTheMetadata(t *testing.T) {
	r := require.New(t)

	editor := &fakeEditor{replace: `{"owner": "ingvar"}`}
	client := &fakeClient{}
	m := onMeta(t, client, editor)

	m, cmd := m.update(key('e'))
	m = follow(m, cmd, 5)

	// What the API can set is what the editor was given, and what comes
	// back goes to the machine.
	r.NotEmpty(editor.opened)
	r.Equal(`{"owner": "ingvar"}`, client.metaSubmitted)
	r.Equal("node-1", client.metaNodeID)
	r.Contains(plain(m.render()), "Metadata of nomad-server-01 submitted.")
}

func TestClient_TheMetadataOfAnotherMachineIsNotShownHere(t *testing.T) {
	r := require.New(t)

	m, _ := pressed(t, key('m'))

	m, _ = m.update(nodeMetaMsg{nodeID: "node-9", meta: []nomad.MetaEntry{{Key: "owner", Value: "somebody else"}}})

	// An answer for the machine that was left must not turn up under the
	// name of the one that is open.
	r.NotContains(plain(m.render()), "somebody else")
}

func TestClient_KeepsOfferingWhatTheAllocationsAnswer(t *testing.T) {
	r := require.New(t)

	m, _ := onAClient(t)

	keys := []string{}
	for _, h := range m.hints() {
		keys = append(keys, h.Key)
	}

	// The screen of a client answers for the machine and for the work on
	// it, so the header offers both.
	r.Contains(keys, "<m>")
	r.Contains(keys, "<r>")
	r.Contains(keys, "<d>")
	r.Contains(keys, "<ctrl-k>")
}

func TestClient_AnAnswerClearsTheErrorAndAsksAgain(t *testing.T) {
	r := require.New(t)

	m, _ := pressed(t, key('a'))

	m, _ = m.update(errMsg{err: errTest})
	m, _ = m.update(pollMsg{})

	m, cmd := m.update(nodeDetailMsg(clientDetail()))

	// The machine answered, so what went wrong is over, and the next ask is
	// on its way.
	r.NotContains(plain(m.render()), "no answer")
	r.NotNil(cmd)
	r.IsType(pollMsg{}, cmd())
}

func TestClient_WhatAnotherMachineSaysIsNotShownHere(t *testing.T) {
	r := require.New(t)

	m, _ := pressed(t, key('a'))
	m, _ = m.update(pollMsg{})

	other := clientDetail()
	other.ID = "node-9"
	other.Attributes = map[string]string{"cpu.arch": "arm64"}

	m, cmd := m.update(nodeDetailMsg(other))

	r.Nil(cmd)
	r.NotContains(plain(m.render()), "arm64")
}

// machineScreens are the screens of a client, each by the key that opens it.
var machineScreens = []struct {
	name   string
	press  tea.KeyPressMsg
	titles []string
	keys   []string
}{
	{"events", key('e'), []string{"Age", "Subsystem", "Message"}, []string{}},
	{"drivers", ctrlKey('d'), []string{"Driver", "Detected", "Healthy", "Updated", "Description"}, []string{"enter Details"}},
	{"volumes", ctrlKey('h'), []string{"Name", "Path", "Read only"}, []string{}},
	{"attributes", key('a'), []string{"Field", "Value"}, []string{"c Copy"}},
	{"meta", key('m'), []string{"Field", "Value", "Set by"}, []string{"c Copy", "e Edit"}},
}

func TestClient_EachOfItsScreensHasItsColumnsAndKeys(t *testing.T) {
	r := require.New(t)

	for _, s := range machineScreens {
		m, _ := pressed(t, s.press)

		r.Equal(s.titles, m.screen.titles(), s.name)
		r.Equal(s.keys, keyNames(m), s.name)

		// The cluster says nothing of a machine on its stream: the screens
		// of one are asked on a timer.
		r.Empty(m.screen.topics(), s.name)
	}

	m, _ := pressed(t, ctrlKey('d'))
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	r.Contains(plain(m.render()), "Driver docker [1]")
	r.Equal([]string{"Field", "Value"}, m.screen.titles())
	r.Equal([]string{"c Copy"}, keyNames(m))
	r.Empty(m.screen.topics())
}

func TestClient_WhatAnotherMachineSaysIsNotShownOnAnyOfItsScreens(t *testing.T) {
	r := require.New(t)

	other := nomad.NodeDetail{
		ID:         "node-9",
		Events:     []nomad.NodeEvent{{Subsystem: "Cluster", Message: "elsewhere"}},
		Drivers:    []nomad.Driver{{Name: "docker", Description: "elsewhere", Attributes: map[string]string{"driver.docker.version": "elsewhere"}}},
		Volumes:    []nomad.HostVolume{{Name: "elsewhere", Path: "/elsewhere"}},
		Attributes: map[string]string{"cpu.arch": "elsewhere"},
	}

	for _, s := range machineScreens[:4] {
		m, _ := pressed(t, s.press)

		m, _ = m.update(nodeDetailMsg(other))
		r.NotContains(plain(m.render()), "elsewhere", s.name)
	}

	m, _ := pressed(t, ctrlKey('d'))
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	m, _ = m.update(nodeDetailMsg(other))
	r.NotContains(plain(m.render()), "elsewhere", "driver")
}

func TestClient_EachOfItsScreensKeepsUpWithTheMachine(t *testing.T) {
	r := require.New(t)

	later := clientDetail()
	later.Events = append(later.Events, nomad.NodeEvent{Subsystem: "Cluster", Message: "later"})
	later.Drivers[0].Description = "later"
	later.Drivers[0].Attributes["driver.docker.version"] = "later"
	later.Volumes = append(later.Volumes, nomad.HostVolume{Name: "later", Path: "/later"})
	later.Attributes["later"] = "later"

	for _, s := range machineScreens[:4] {
		m, _ := pressed(t, s.press)

		m, _ = m.update(nodeDetailMsg(later))
		r.Contains(plain(m.render()), "later", s.name)
	}

	m, _ := pressed(t, ctrlKey('d'))
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	m, _ = m.update(nodeDetailMsg(later))
	r.Contains(plain(m.render()), "later", "driver")

	// The metadata is read on its own, from the machine itself.
	m, client := pressed(t, key('m'))
	r.Equal("node-1", client.askedNodeID)

	m, _ = m.update(nodeMetaMsg{nodeID: "node-1", meta: []nomad.MetaEntry{{Key: "rack", Value: "later"}}})
	r.Contains(plain(m.render()), "later")
}

func TestClient_ADriverShowsWhatItSaysBeforeItIsAskedAgain(t *testing.T) {
	r := require.New(t)

	m, _ := pressed(t, ctrlKey('d'))

	// What the list of drivers read is what the driver shows until the
	// machine answers again.
	m, _ = m.update(enter())

	r.Contains(plain(m.render()), "27.1.1")
}

func TestClient_WhatReadsAsWrong(t *testing.T) {
	r := require.New(t)

	// An event the machine marks as failed.
	events := nodeEventRows([]nomad.NodeEvent{{Details: map[string]string{"failed": "true"}}, {}})
	r.Equal(colorDead, events[0].color)
	r.Nil(events[1].color)

	// A driver that works, one found and broken, and one never found.
	drivers := driverRows([]nomad.Driver{{Detected: true, Healthy: true}, {Detected: true}, {}})
	r.Nil(drivers[0].color)
	r.Equal(colorDead, drivers[1].color)
	r.Equal(colorSpent, drivers[2].color)

	// What the agent set is muted: it cannot be changed from here.
	meta := metaRows(clientMeta())
	r.Nil(meta[0].color)
	r.Equal(colorMuted, meta[1].color)
}

func TestClient_AScreenOfAnotherClientStartsEmpty(t *testing.T) {
	r := require.New(t)

	nodes := append(busyClient(), nomad.Node{ID: "node-2", Name: "nomad-client-02", Status: "ready", Eligibility: "eligible"})
	client := &fakeClient{nodes: nodes, nodeAllocs: clientAllocs(), nodeDetail: clientDetail()}

	m, _ := nodeModelOf(client)
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	m, cmd = m.update(key('a'))
	m = drain(m, cmd)
	r.Contains(plain(m.render()), "amd64")

	// Back to the list, and the attributes of the next client.
	m, _ = m.update(escape())
	m, _ = m.update(escape())
	m, _ = m.update(down())

	m, cmd = m.update(enter())
	m = drain(m, cmd)

	m, _ = m.update(key('a'))

	// Until that machine answers, it has said nothing: what the first one
	// said is not its.
	out := plain(m.render())
	r.Contains(out, "Attributes (Client: nomad-client-02) [0]")
	r.NotContains(out, "amd64")
}

func TestClient_TheDriverUnderTheCursorIsTheOneThatOpens(t *testing.T) {
	r := require.New(t)

	m, _ := pressed(t, ctrlKey('d'))
	m, _ = m.update(down())

	m, cmd := m.update(enter())
	m = drain(m, cmd)

	r.Contains(plain(m.render()), "Driver exec [0]")
}
