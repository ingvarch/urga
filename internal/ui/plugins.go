package ui

import (
	"context"
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// Messages of the plugin screens.
type (
	pluginsMsg []nomad.Plugin
	pluginMsg  nomad.PluginDetail
)

// pluginsTitles are the columns of the plugin list: how many controllers and
// node plugins are healthy, out of how many.
var pluginsTitles = []string{"ID", "Provider", "Controllers", "Nodes"}

// pluginsPage is the CSI plugins of the cluster.
type pluginsPage struct {
	ofTheSession

	plugins []nomad.Plugin
}

// A plugin belongs to the cluster, not to a namespace: the title names none.
func (pluginsPage) title(_ env, count int) string { return sprintf("Plugins [%d]", count) }

func (pluginsPage) titles() []string { return pluginsTitles }

// topics: none. The cluster reports no change of a plugin; the list is
// polled.
func (pluginsPage) topics() []string { return nil }

func (pluginsPage) fetch(e env) tea.Cmd {
	return fetchList(e.client.Plugins, func(items []nomad.Plugin) tea.Msg { return pluginsMsg(items) })
}

func (p pluginsPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	plugins, ok := msg.(pluginsMsg)
	if !ok {
		return p, outcome{}, false
	}

	p.plugins = plugins

	return p, outcome{}, true
}

func (p pluginsPage) rows(env) []tableRow {
	rows := make([]tableRow, 0, len(p.plugins))

	for _, plugin := range p.plugins {
		rows = append(rows, tableRow{
			cells: []string{plugin.ID, plugin.Provider, healthOf(plugin.Controllers), healthOf(plugin.Nodes)},
			color: pluginColor(plugin),
		})
	}

	return rows
}

// picked is the plugin under the cursor.
func (p pluginsPage) picked(e env) (nomad.Plugin, bool) { return pickedFrom(e, p.plugins) }

var pluginsKeys = []pageKey[pluginsPage]{
	{press: "enter", label: "Details", do: openPlugin},
	{press: "d", label: "Describe", do: describePlugin},
}

func (p pluginsPage) keys(e env) []keyHint { return hintsOf(p, e, pluginsKeys) }

func (p pluginsPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, pluginsKeys, k)
}

// openPlugin opens the plugin under the cursor: its instances, node by node.
func openPlugin(p pluginsPage, e env) (pluginsPage, outcome) {
	plugin, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg{pluginPage{id: plugin.ID}})
}

// describePlugin shows the plugin under the cursor as the cluster has it.
func describePlugin(p pluginsPage, e env) (pluginsPage, outcome) {
	plugin, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	client := e.client

	return p, outcome{cmd: describe(fmt.Sprintf("Plugin: %s", plugin.ID), func(ctx context.Context) (string, error) {
		return client.DescribePlugin(ctx, plugin.ID)
	})}
}

// pluginColor shows a plugin with no healthy instance of a kind in red: no
// volume of it can be attached. One short of healthy instances gets the
// attention color.
func pluginColor(p nomad.Plugin) color.Color {
	switch {
	case none(p.Controllers) || none(p.Nodes):
		return colorDead
	case short(p.Controllers) || short(p.Nodes):
		return colorAttention
	}

	return nil
}

// none says instances are expected and not one is healthy.
func none(h nomad.Health) bool { return h.Expected > 0 && h.Healthy == 0 }

// pluginTitles are the columns of the instances of a plugin.
var pluginTitles = []string{"Kind", "Node", "Healthy", "Description", "Updated"}

// pluginPage is one plugin: what it is, above its instances, one per node
// and kind.
type pluginPage struct {
	id string

	// plugin is the plugin as the page last read it; read says it has.
	plugin nomad.PluginDetail
	read   bool
}

func (p pluginPage) title(_ env, count int) string { return sprintf("Plugin %s [%d]", p.id, count) }

func (pluginPage) titles() []string { return pluginTitles }

// topics: none, as for the list.
func (pluginPage) topics() []string { return nil }

func (p pluginPage) fetch(e env) tea.Cmd {
	client, id := e.client, p.id

	return request(func(ctx context.Context) (nomad.PluginDetail, error) {
		return client.Plugin(ctx, id)
	}, func(plugin nomad.PluginDetail) tea.Msg { return pluginMsg(plugin) })
}

// take keeps the plugin the page is open on.
func (p pluginPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	plugin, ok := msg.(pluginMsg)
	if !ok || plugin.ID != p.id {
		return p, outcome{}, false
	}

	p.plugin, p.read = nomad.PluginDetail(plugin), true

	return p, outcome{}, true
}

func (p pluginPage) panel(_ env, width, room int) []string {
	if !p.read {
		return nil
	}

	line := fieldLine([]field{
		{"Provider", strings.TrimSpace(p.plugin.Provider + " " + p.plugin.Version)},
		{"Controller required", yesNo(p.plugin.ControllerRequired)},
		{"Controllers", healthOf(p.plugin.Controllers)},
		{"Nodes", healthOf(p.plugin.Nodes)},
	}, width-1)

	return fitPanel([]string{" " + line}, panelBlock{}, room)
}

func (p pluginPage) rows(env) []tableRow {
	rows := make([]tableRow, 0, len(p.plugin.Instances))

	for _, in := range p.plugin.Instances {
		row := tableRow{}
		if !in.Healthy {
			row.color = colorDead
		}

		row.add(in.Kind, dashed(in.NodeName), yesNo(in.Healthy), in.Description)
		row.addAge(in.Updated)

		rows = append(rows, row)
	}

	return rows
}

// allocOf is the allocation that runs the instance under the cursor.
func (p pluginPage) allocOf(e env) (nomad.Alloc, bool) {
	in, ok := pickedFrom(e, p.plugin.Instances)
	if !ok {
		return nomad.Alloc{}, false
	}

	for _, alloc := range p.plugin.Allocs {
		if alloc.ID == in.AllocID {
			return alloc, true
		}
	}

	return nomad.Alloc{}, false
}

var pluginKeys = []pageKey[pluginPage]{
	{press: "enter", label: "Tasks", do: openInstance, offered: func(p pluginPage, e env) bool {
		_, ok := p.allocOf(e)

		return ok
	}},
}

func (p pluginPage) keys(e env) []keyHint { return hintsOf(p, e, pluginKeys) }

func (p pluginPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, pluginKeys, k)
}

// openInstance opens the tasks of the allocation that runs the instance
// under the cursor, where its logs are.
func openInstance(p pluginPage, e env) (pluginPage, outcome) {
	alloc, ok := p.allocOf(e)
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg{listedTasks(alloc)})
}
