package nomad

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/hashicorp/nomad/api"
)

// The kinds of plugin instance: a controller talks to the storage, a node
// plugin mounts volumes on its node.
const (
	PluginController = "controller"
	PluginNode       = "node"
)

// Plugin is a CSI plugin, and how many of its instances are healthy.
type Plugin struct {
	ID, Provider       string
	Controllers, Nodes Health
}

// PluginDetail is one plugin, with its instances.
type PluginDetail struct {
	Plugin

	Version            string
	ControllerRequired bool
	Instances          []PluginInstance

	// Allocs are the allocations that run the instances.
	Allocs []Alloc
}

// PluginInstance is a plugin that runs on one node, as a controller or as a
// node plugin.
type PluginInstance struct {
	Kind             string
	NodeID, NodeName string
	AllocID          string

	// Healthy says the plugin answers; Description says why it does not.
	Healthy     bool
	Description string
	Updated     time.Time
}

// Plugins lists the CSI plugins of the cluster.
func (c *Client) Plugins(ctx context.Context) ([]Plugin, error) {
	list, _, err := c.api.CSIPlugins().List(c.query(ctx, ""))
	if err != nil {
		return nil, err
	}

	out := make([]Plugin, 0, len(list))
	for _, p := range list {
		out = append(out, Plugin{
			ID:          p.ID,
			Provider:    p.Provider,
			Controllers: Health{Healthy: p.ControllersHealthy, Expected: p.ControllersExpected},
			Nodes:       Health{Healthy: p.NodesHealthy, Expected: p.NodesExpected},
		})
	}

	return out, nil
}

// Plugin reads one CSI plugin.
func (c *Client) Plugin(ctx context.Context, id string) (PluginDetail, error) {
	p, _, err := c.api.CSIPlugins().Info(id, c.query(ctx, ""))
	if err != nil {
		return PluginDetail{}, err
	}

	// The instances name their node by ID; the allocations that run them
	// carry its name.
	names := map[string]string{}
	for _, alloc := range p.Allocations {
		if alloc != nil {
			names[alloc.NodeID] = alloc.NodeName
		}
	}

	instances := slices.Concat(pluginInstances(PluginController, p.Controllers, names), pluginInstances(PluginNode, p.Nodes, names))

	return PluginDetail{
		Plugin: Plugin{
			ID:          p.ID,
			Provider:    p.Provider,
			Controllers: Health{Healthy: p.ControllersHealthy, Expected: p.ControllersExpected},
			Nodes:       Health{Healthy: p.NodesHealthy, Expected: p.NodesExpected},
		},
		Version:            p.Version,
		ControllerRequired: p.ControllerRequired,
		Instances:          instances,
		Allocs:             allocsOf(p.Allocations),
	}, nil
}

// pluginInstances are the instances of one kind, by the name of their node.
func pluginInstances(kind string, byNode map[string]*api.CSIInfo, names map[string]string) []PluginInstance {
	out := make([]PluginInstance, 0, len(byNode))

	for nodeID, info := range byNode {
		if info == nil {
			continue
		}

		out = append(out, PluginInstance{
			Kind:        kind,
			NodeID:      nodeID,
			NodeName:    names[nodeID],
			AllocID:     info.AllocID,
			Healthy:     info.Healthy,
			Description: info.HealthDescription,
			Updated:     info.UpdateTime,
		})
	}

	slices.SortFunc(out, func(a, b PluginInstance) int {
		return cmp.Or(cmp.Compare(a.NodeName, b.NodeName), cmp.Compare(a.NodeID, b.NodeID))
	})

	return out
}

// DescribePlugin is what the cluster knows about a CSI plugin.
func (c *Client) DescribePlugin(ctx context.Context, id string) (string, error) {
	p, _, err := c.api.CSIPlugins().Info(id, c.query(ctx, ""))
	if err != nil {
		return "", err
	}

	return asJSON(p)
}
