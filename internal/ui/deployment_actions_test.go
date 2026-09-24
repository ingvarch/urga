package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// ctrl is a key pressed with control.
func ctrl(key rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: key, Mod: tea.ModCtrl} }

// confirmed answers yes to the question on the screen and runs what it
// starts.
func confirmed(t *testing.T, m Model) Model {
	t.Helper()

	require.Equal(t, overlayConfirm, m.overlay)

	m, cmd := answerYes(m)

	return drain(m, cmd)
}

func TestDeployment_PromoteTheGroupUnderTheCursor(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onDeployment(t, client)

	// The canary of web is under the cursor, and web waits for it.
	r.True(offers(m, "p"))

	m, _ = m.update(key('p'))
	r.Contains(plain(m.render()), "Really promote the canaries of group web of web?")

	m = confirmed(t, m)

	r.Equal([]string{"web"}, client.promotedGroups)
	r.Equal("5d1a2b3c-0000-0000-0000-000000000000", client.askedID)
	r.Equal("production", client.askedNamespace)
	r.Contains(plain(m.render()), "Canaries of web promoted.")
}

func TestDeployment_OnlyAGroupThatWaitsIsPromoted(t *testing.T) {
	r := require.New(t)

	m := onDeployment(t, &fakeClient{})

	// api has no canaries to promote; web still has.
	m, _ = m.update(key('j'))

	r.False(offers(m, "p"))
	r.True(offers(m, "ctrl-p"))
}

func TestDeployment_PromoteEveryGroup(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onDeployment(t, client)

	m, _ = m.update(ctrl('p'))
	r.Contains(plain(m.render()), "Really promote the canaries of every group of web?")

	confirmed(t, m)

	r.Equal(1, client.promoted)
	r.Empty(client.promotedGroups)
}

func TestDeployment_PauseAndResume(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onDeployment(t, client)

	// The key says what it does to the deployment as it is.
	r.True(offersLabel(m, "ctrl-s", "Pause"))

	m, _ = m.update(ctrl('s'))
	r.Contains(plain(m.render()), "Really pause the deployment of web?")

	m = confirmed(t, m)
	r.Equal([]bool{true}, client.paused)

	paused := waitingCanary()
	paused.Status = "paused"
	m, _ = m.update(deploymentMsg(paused))

	r.True(offersLabel(m, "ctrl-s", "Resume"))

	m, _ = m.update(ctrl('s'))
	confirmed(t, m)

	r.Equal([]bool{true, false}, client.paused)
}

func TestDeployment_FailFromItsScreen(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onDeployment(t, client)

	m, _ = m.update(key('f'))
	confirmed(t, m)

	r.Equal(1, client.failed)
	r.Equal("5d1a2b3c-0000-0000-0000-000000000000", client.askedID)
}

func TestDeployment_NothingToDoWhenItIsOver(t *testing.T) {
	r := require.New(t)

	m := onDeployment(t, &fakeClient{})

	done := waitingCanary()
	done.Status = "successful"
	m, _ = m.update(deploymentMsg(done))

	for _, press := range []string{"p", "ctrl-p", "f", "ctrl-s"} {
		r.False(offers(m, press), press)
	}

	// What an allocation can do stays.
	r.True(offers(m, "r"))
}

func TestDeployments_PauseFromTheList(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{deployments: []nomad.Deployment{waitingCanary().Deployment}}
	m := newTestModel(client)
	m, _ = m.show(screenDeployments)
	m, _ = m.update(deploymentsMsg(client.deployments))

	r.True(offersLabel(m, "ctrl-s", "Pause"))

	m, _ = m.update(ctrl('s'))
	confirmed(t, m)

	r.Equal([]bool{true}, client.paused)
	r.Equal("5d1a2b3c-0000-0000-0000-000000000000", client.askedID)
}

// offersLabel says the header offers the key under that label.
func offersLabel(m Model, press, label string) bool {
	for _, h := range m.hints() {
		if h.Key == "<"+press+">" && h.Description == label {
			return true
		}
	}

	return false
}
