package ui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// onePlugin is a CSI plugin with one of its node plugins down.
func onePlugin() []nomad.Plugin {
	return []nomad.Plugin{{
		ID: "aws-ebs0", Provider: "ebs.csi.aws.com",
		Controllers: nomad.Health{Healthy: 2, Expected: 2}, Nodes: nomad.Health{Healthy: 5, Expected: 6},
	}}
}

// pluginDetail is a plugin read in full: a healthy controller, and a node
// plugin that does not answer.
func pluginDetail() nomad.PluginDetail {
	return nomad.PluginDetail{
		Plugin: nomad.Plugin{
			ID: "aws-ebs0", Provider: "ebs.csi.aws.com",
			Controllers: nomad.Health{Healthy: 1, Expected: 1}, Nodes: nomad.Health{Healthy: 0, Expected: 1},
		},
		Version: "1.2.0", ControllerRequired: true,
		Instances: []nomad.PluginInstance{
			{
				Kind: nomad.PluginController, NodeID: "node-1", NodeName: "node-01", AllocID: "a1",
				Healthy: true, Updated: time.Now().Add(-2 * time.Minute),
			},
			{
				Kind: nomad.PluginNode, NodeID: "node-4", NodeName: "node-04", AllocID: "a4",
				Description: "failed to fingerprint: timeout", Updated: time.Now().Add(-5 * time.Minute),
			},
		},
		Allocs: []nomad.Alloc{
			{ID: "a1", Namespace: "system", JobID: "ebs-controller", NodeName: "node-01", Status: "running"},
			{ID: "a4", Namespace: "system", JobID: "ebs-nodes", NodeName: "node-04", Status: "running"},
		},
	}
}

func TestPlugins_TheList(t *testing.T) {
	r := require.New(t)

	m := typeCommand(newTestModel(&fakeClient{plugins: onePlugin()}), "plugins")

	// A plugin belongs to the cluster, not to a namespace.
	r.IsType(pluginsPage{}, m.screen.page)
	r.Equal("Plugins [1]", m.title())

	rows := m.rows()
	r.Equal([]string{"aws-ebs0", "ebs.csi.aws.com", "2/2", "5/6"}, rows[0].cells)
	r.Equal(colorAttention, rows[0].color)

	r.Equal([]hint{
		{Key: "<enter>", Description: "Details"},
		{Key: "<d>", Description: "Describe"},
	}, m.hints())
}

func TestPlugins_OpenedByItsOtherName(t *testing.T) {
	r := require.New(t)

	m := typeCommand(newTestModel(&fakeClient{plugins: onePlugin()}), "plugin")
	r.IsType(pluginsPage{}, m.screen.page)
}

func TestPlugins_Describe(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{plugins: onePlugin(), describe: `{"ID": "aws-ebs0"}`}
	m := typeCommand(newTestModel(client), "plugins")

	m, cmd := m.update(key('d'))
	m = playOut(m, cmd)

	r.IsType(describePage{}, m.screen.page)
	r.Equal("Plugin: aws-ebs0", m.title())
	r.Equal("aws-ebs0", client.pluginOf)
}

func TestPluginColor(t *testing.T) {
	r := require.New(t)

	plugin := func(controllers, nodes nomad.Health) nomad.Plugin {
		return nomad.Plugin{Controllers: controllers, Nodes: nodes}
	}

	all := nomad.Health{Healthy: 2, Expected: 2}

	r.Nil(pluginColor(plugin(all, all)))
	r.Nil(pluginColor(plugin(nomad.Health{}, all)))
	r.Equal(colorAttention, pluginColor(plugin(all, nomad.Health{Healthy: 1, Expected: 2})))

	// With no healthy instance of a kind, no volume of the plugin can be
	// attached.
	r.Equal(colorDead, pluginColor(plugin(all, nomad.Health{Healthy: 0, Expected: 2})))
	r.Equal(colorDead, pluginColor(plugin(nomad.Health{Healthy: 0, Expected: 1}, all)))
}

// onPlugin is the screen of the plugin of the list.
func onPlugin(t *testing.T, client *fakeClient) Model {
	t.Helper()

	m := typeCommand(newTestModel(client), "plugins")
	m, cmd := m.update(enter())

	return playOut(m, cmd)
}

func TestPlugin_TheScreen(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{plugins: onePlugin(), plugin: pluginDetail()}
	m := onPlugin(t, client)

	r.IsType(pluginPage{}, m.screen.page)
	r.Equal("aws-ebs0", client.pluginOf)
	r.Equal("Plugin aws-ebs0 [2]", m.title())

	panel := panelOf(t, m)
	for _, want := range []string{"Provider ebs.csi.aws.com 1.2.0", "Controller required yes", "Controllers 1/1", "Nodes 0/1"} {
		r.Contains(panel, want)
	}

	// One row for each instance; why one is not healthy is on its row.
	rows := m.rows()
	r.Equal([]string{"controller", "node-01", "yes", "", "2m"}, rows[0].cells)
	r.Nil(rows[0].color)

	r.Equal([]string{"node", "node-04", "no", "failed to fingerprint: timeout", "5m"}, rows[1].cells)
	r.Equal(colorDead, rows[1].color)
}

func TestPlugin_EnterOpensTheAllocationOfAnInstance(t *testing.T) {
	r := require.New(t)

	m := onPlugin(t, &fakeClient{plugins: onePlugin(), plugin: pluginDetail()})
	r.Contains(m.hints(), hint{Key: "<enter>", Description: "Tasks"})

	m, _ = m.update(key('j'))
	m, _ = m.update(enter())

	tasks, ok := m.screen.page.(tasksPage)
	r.True(ok, "%T", m.screen.page)
	r.Equal("a4", tasks.allocID)
	r.Equal("system", tasks.namespace)
}

func TestPlugin_AnotherPluginIsNotTaken(t *testing.T) {
	r := require.New(t)

	m := onPlugin(t, &fakeClient{plugins: onePlugin(), plugin: pluginDetail()})

	other := pluginDetail()
	other.ID, other.Instances = "nfs0", nil
	m, _ = m.update(pluginMsg(other))

	r.Equal("Plugin aws-ebs0 [2]", m.title())
}
