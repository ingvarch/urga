package ui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

const claimsText = "{\n  \"nomad_node_id\": \"node-1\"\n}"

// identityCluster is one client of the given version, with an identity that
// expires after left, or none when left is zero.
func identityCluster(version string, left time.Duration) *fakeClient {
	nodes := busyClient()
	nodes[0].Version = version

	client := &fakeClient{
		nodes:      nodes,
		nodeAllocs: clientAllocs(),
		use:        map[string]nomad.ResourceUse{"node-1": {}},
	}

	if left != 0 {
		client.identity = &nomad.NodeIdentity{Expires: time.Now().Add(left), Claims: claimsText}
	}

	return client
}

func onIdentity(t *testing.T, client *fakeClient) Model {
	t.Helper()

	m, _ := nodeModelOf(client)
	m, cmd := m.update(enter())

	return drain(m, cmd)
}

func TestClient_AskedForItsIdentityOnEachPollFromNomad111(t *testing.T) {
	r := require.New(t)

	client := identityCluster("1.11.0", time.Hour)
	m := onIdentity(t, client)

	// The list said the version already: the open and the machine's answer
	// ask once.
	r.Equal([]string{"node-1"}, client.identityAsked)

	drain(m, m.fetch())

	r.Equal([]string{"node-1", "node-1"}, client.identityAsked)
}

func TestClient_AskedForItsIdentityOnceTheMachineShowsAVersionThatQualifies(t *testing.T) {
	r := require.New(t)

	client := identityCluster("1.11.0", time.Hour)

	page := clientPage{nodeID: "node-1", name: "nomad-server-01"}
	e := newTestModel(client).env()

	// A version not read yet asks nothing.
	drain(newTestModel(client), page.fetch(e))
	r.Empty(client.identityAsked)

	_, out, _ := page.take(hostMsg(nomad.Node{ID: "node-1", Version: "1.11.0"}), e)
	r.NotNil(out.cmd)

	msg, ok := out.cmd().(identityMsg)
	r.True(ok)
	r.Equal("node-1", msg.nodeID)
	r.Equal([]string{"node-1"}, client.identityAsked)

	// A machine that is already known to qualify is not asked a second
	// time for the same answer.
	known := clientPage{nodeID: "node-1", host: hostModel{node: nomad.Node{ID: "node-1", Version: "1.11.0"}}}
	_, out, _ = known.take(hostMsg(nomad.Node{ID: "node-1", Version: "1.11.0"}), e)
	r.Nil(out.cmd)
}

func TestClient_NeverAsksAnOlderClientForItsIdentity(t *testing.T) {
	r := require.New(t)

	client := identityCluster("1.10.5", time.Hour)
	m := onIdentity(t, client)
	m = drain(m, m.fetch())

	r.Empty(client.identityAsked)
	r.NotContains(bodyOf(m), "Identity")
	r.NotContains(m.flash.text, nomad.ErrNoIdentity.Error())
}

func TestClient_ShowsWhenItsIdentityExpires(t *testing.T) {
	r := require.New(t)

	m := onIdentity(t, identityCluster("1.11.1", 23*time.Hour+30*time.Minute))

	r.Contains(bodyOf(m), "Identity expires in 23h")
}

func TestClient_ShowsAnIdentityThatExpiredAsExpired(t *testing.T) {
	r := require.New(t)

	m := onIdentity(t, identityCluster("1.11.1", -time.Hour))

	r.Contains(bodyOf(m), "Identity expired")
	r.NotContains(bodyOf(m), "expires expired")
}

func TestClient_WithoutAnIdentityShowsNothingOfIt(t *testing.T) {
	r := require.New(t)

	m := onIdentity(t, identityCluster("1.11.1", 0))
	m = drain(m, m.fetch())

	r.NotContains(bodyOf(m), "Identity")
	r.NotContains(m.hints(), hint{Key: "<i>", Description: "Identity"})
	r.NotContains(m.hints(), hint{Key: "<ctrl-n>", Description: "Renew"})
	r.NotContains(m.flash.text, nomad.ErrNoIdentity.Error())

	// An identity without an expiry has no field either, and still has keys.
	client := identityCluster("1.11.1", 0)
	client.identity = &nomad.NodeIdentity{Claims: claimsText}
	m = onIdentity(t, client)

	r.NotContains(bodyOf(m), "Identity")
	r.Contains(m.hints(), hint{Key: "<i>", Description: "Identity"})
}

func TestClient_AnIdentityThatCannotBeReadShowsTheError(t *testing.T) {
	r := require.New(t)

	client := identityCluster("1.11.1", 0)
	m := onIdentity(t, client)

	client.identityErr = errTest
	m = drain(m, m.fetch())

	r.Contains(m.flash.text, errTest.Error())
	r.NotContains(bodyOf(m), "Identity")
}

func TestClient_DropsTheIdentityOfAnotherNode(t *testing.T) {
	r := require.New(t)

	m := onIdentity(t, identityCluster("1.11.1", 0))

	m, _ = m.update(identityMsg{nodeID: "node-2", identity: &nomad.NodeIdentity{Expires: time.Now().Add(time.Hour)}})

	r.NotContains(bodyOf(m), "Identity")

	m, _ = m.update(identityMsg{nodeID: "node-1", identity: &nomad.NodeIdentity{Expires: time.Now().Add(time.Hour)}})

	r.Contains(bodyOf(m), "Identity expires in")
}

func TestClient_IdentityShowsItsClaims(t *testing.T) {
	r := require.New(t)

	client := identityCluster("1.11.1", time.Hour)
	m := onIdentity(t, client)
	asked := len(client.identityAsked)

	m, cmd := m.update(key('i'))

	r.Nil(cmd)
	r.Len(client.identityAsked, asked, "the page holds the claims")
	r.Contains(plain(m.render()), "Identity (Client: nomad-server-01)")
	r.Contains(plain(m.render()), `"nomad_node_id": "node-1"`)
}

func TestClient_RenewAsksAndSendsOnYes(t *testing.T) {
	r := require.New(t)

	client := identityCluster("1.11.1", time.Hour)
	m := onIdentity(t, client)

	m, _ = m.update(ctrlKey('n'))

	r.Equal(overlayConfirm, m.overlay)
	r.Contains(plain(m.render()), "Really renew the identity of the client nomad-server-01?")
	r.Empty(client.renewed)

	m = confirmed(t, m)

	r.Equal([]string{"node-1"}, client.renewed)
	r.Contains(m.flash.text, "The client nomad-server-01 gets a new identity at its next heartbeat.")
}

func TestClient_ReadOnlyOffersIdentityAndNotRenew(t *testing.T) {
	r := require.New(t)

	client := identityCluster("1.11.1", time.Hour)
	m, _ := nodeModelOf(client)
	m.opts.ReadOnly = true
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	r.Contains(m.hints(), hint{Key: "<i>", Description: "Identity"})
	r.NotContains(m.hints(), hint{Key: "<ctrl-n>", Description: "Renew"})

	m, cmd = m.update(ctrlKey('n'))
	r.Nil(cmd)
	r.NotEqual(overlayConfirm, m.overlay)
	r.Empty(client.renewed)
}
