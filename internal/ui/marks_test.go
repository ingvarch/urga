package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestVersions_TagAVersion(t *testing.T) {
	r := require.New(t)

	m, client := onVersions(t)
	m, _ = m.update(key('j'))

	r.Contains(m.hints(), hint{Key: "<t>", Description: "Tag"})

	m, _ = m.update(key('t'))
	r.Equal(overlayAnswer, m.overlay)
	r.Contains(plain(m.render()), "tag version 2 as:")
	r.Empty(m.prompt.text)

	m = typeIn(m, "golden")
	m, cmd := m.update(enter())
	m = playOut(m, cmd)

	// The name is the answer: no question after it.
	r.Equal([]string{"TagVersion"}, client.writes)
	r.Equal("web@2=golden", client.tagged)
	r.Equal("production", client.askedNamespace)
	r.Contains(plain(m.render()), "Version 2 of web tagged golden.")
}

func TestVersions_ATagIsRenamedInItsLine(t *testing.T) {
	r := require.New(t)

	m, _ := onVersions(t)

	// A version carries one tag: tagging it again gives it another name.
	m, _ = m.update(key('t'))
	r.Equal("golden", m.prompt.text)
}

func TestVersions_ATagNeedsAName(t *testing.T) {
	r := require.New(t)

	m, client := onVersions(t)
	m, _ = m.update(key('j'))

	m, _ = m.update(key('t'))
	m, cmd := m.update(enter())
	m = playOut(m, cmd)

	r.Empty(client.writes)
	r.Contains(plain(statusLine(m)), "A tag needs a name.")
}

func TestVersions_TakeATagOff(t *testing.T) {
	r := require.New(t)

	m, client := onVersions(t)

	untag := hint{Key: "<ctrl-t>", Description: "Untag"}
	r.Contains(m.hints(), untag)

	m, _ = m.update(ctrlKey('t'))
	r.Contains(plain(m.render()), "Really take the tag golden off version 3 of web?")

	m, cmd := m.update(key('y'))
	m = playOut(m, cmd)

	r.Equal("web=golden", client.untagged)
	r.Contains(plain(m.render()), "Tag golden taken off version 3 of web.")

	// A version without a tag has none to take off.
	m, _ = m.update(key('j'))
	r.NotContains(m.hints(), untag)
}

func TestDeployment_MarkAnAllocationHealthy(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onDeployment(t, client)

	r.Contains(m.hints(), hint{Key: "<h>", Description: "Healthy"})
	r.Contains(m.hints(), hint{Key: "<u>", Description: "Unhealthy"})

	m, _ = m.update(key('h'))
	r.Contains(plain(m.render()), "Really mark the allocation 9a1b2c3d healthy?")

	m, cmd := m.update(key('y'))
	m = playOut(m, cmd)

	r.Equal([]string{"SetAllocHealth"}, client.writes)
	r.Equal([]string{"9a1b2c3d-0000-0000-0000-000000000000"}, client.marked)
	r.True(client.markedHealth)
	r.Equal("production", client.askedNamespace)
	r.Contains(plain(m.render()), "Marked the allocation 9a1b2c3d healthy.")
}

func TestDeployment_MarkTheMarkedUnhealthy(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onDeployment(t, client)
	m, _ = m.update(ctrlKey('a'))

	m, _ = m.update(key('u'))
	r.Contains(plain(m.render()), "Really mark 2 allocations unhealthy? The deployment counts them as failed.")

	m, cmd := m.update(key('y'))
	playOut(m, cmd)

	r.Len(client.marked, 2)
	r.False(client.markedHealth)
}

func TestDeployment_NoHealthToMarkWhenItIsOver(t *testing.T) {
	r := require.New(t)

	// The cluster refuses to mark the allocations of a deployment that ended.
	client := &fakeClient{}
	m := onDeployment(t, client)

	over := waitingCanary()
	over.Status = "successful"
	m, _ = m.update(deploymentMsg(over))

	r.NotContains(m.hints(), hint{Key: "<h>", Description: "Healthy"})
	r.NotContains(m.hints(), hint{Key: "<u>", Description: "Unhealthy"})
}

func TestVariable_ReleaseItsLock(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{}
	m := onVariable(t, client, leaderVariable())

	release := hint{Key: "<ctrl-r>", Description: "Release Lock"}
	r.Contains(m.hints(), release)

	m, _ = m.update(ctrlKey('r'))
	r.Contains(plain(m.render()), "Really release the lock on locks/leader? Whoever holds it loses it.")

	m, cmd := m.update(key('y'))
	m = playOut(m, cmd)

	r.Equal([]string{"ReleaseLock"}, client.writes)
	r.Equal("locks/leader@"+leaderLock, client.releasedLock)
	r.Equal("default", client.askedNamespace)
	r.Contains(plain(m.render()), "Lock on locks/leader released.")
}

func TestVariables_ReleaseALockFromTheList(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{variables: []nomad.Variable{webVariable().Variable, leaderVariable().Variable}}
	m := newTestModel(client)
	m, _ = m.show(variablesView)
	m, _ = m.update(variablesMsg(client.variables))

	release := hint{Key: "<ctrl-r>", Description: "Release Lock"}

	// nomad/jobs/web is held by nobody.
	r.NotContains(m.hints(), release)

	m, _ = m.update(key('j'))
	r.Contains(m.hints(), release)

	m, _ = m.update(ctrlKey('r'))

	var cmd tea.Cmd

	m, cmd = m.update(key('y'))
	playOut(m, cmd)

	r.Equal("locks/leader@"+leaderLock, client.releasedLock)
}
