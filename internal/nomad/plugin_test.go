package nomad_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestPlugins(t *testing.T) {
	r := require.New(t)

	client, _ := clusterServer(t, map[string]string{
		"/v1/plugins?type=csi": `[{
			"ID": "aws-ebs0", "Provider": "ebs.csi.aws.com", "ControllerRequired": true,
			"ControllersHealthy": 2, "ControllersExpected": 2, "NodesHealthy": 5, "NodesExpected": 6
		}]`,
	})

	plugins, err := client.Plugins(context.Background())
	r.NoError(err)

	r.Equal([]nomad.Plugin{{
		ID:          "aws-ebs0",
		Provider:    "ebs.csi.aws.com",
		Controllers: nomad.Health{Healthy: 2, Expected: 2},
		Nodes:       nomad.Health{Healthy: 5, Expected: 6},
	}}, plugins)
}

const csiPlugin = `{
	"ID": "aws-ebs0", "Provider": "ebs.csi.aws.com", "Version": "1.2.0", "ControllerRequired": true,
	"ControllersHealthy": 1, "ControllersExpected": 1, "NodesHealthy": 1, "NodesExpected": 2,
	"Controllers": {"node-1": {"AllocID": "a1", "Healthy": true, "UpdateTime": "2026-09-28T10:00:00Z"}},
	"Nodes": {
		"node-4": {"AllocID": "a4", "Healthy": false, "HealthDescription": "failed to fingerprint: timeout", "UpdateTime": "2026-09-28T10:05:00Z"},
		"node-2": {"AllocID": "a2", "Healthy": true, "UpdateTime": "2026-09-28T10:01:00Z"}
	},
	"Allocations": [
		{"ID": "a1", "Namespace": "system", "JobID": "ebs-controller", "NodeID": "node-1", "NodeName": "node-01"},
		{"ID": "a4", "NodeID": "node-4", "NodeName": "node-04"},
		{"ID": "a2", "NodeID": "node-2", "NodeName": "node-02"}
	]
}`

func TestPlugin(t *testing.T) {
	r := require.New(t)

	client, _ := clusterServer(t, map[string]string{"/v1/plugin/csi/aws-ebs0": csiPlugin})

	plugin, err := client.Plugin(context.Background(), "aws-ebs0")
	r.NoError(err)

	r.Equal("ebs.csi.aws.com", plugin.Provider)
	r.Equal("1.2.0", plugin.Version)
	r.True(plugin.ControllerRequired)
	r.Equal(nomad.Health{Healthy: 1, Expected: 2}, plugin.Nodes)

	// The controllers first, then the node plugins, each by the name of
	// the node it runs on: the cluster keeps them in no order.
	r.Equal([]nomad.PluginInstance{
		{
			Kind: nomad.PluginController, NodeID: "node-1", NodeName: "node-01", AllocID: "a1",
			Healthy: true, Updated: time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC),
		},
		{
			Kind: nomad.PluginNode, NodeID: "node-2", NodeName: "node-02", AllocID: "a2",
			Healthy: true, Updated: time.Date(2026, 9, 28, 10, 1, 0, 0, time.UTC),
		},
		{
			Kind: nomad.PluginNode, NodeID: "node-4", NodeName: "node-04", AllocID: "a4",
			Description: "failed to fingerprint: timeout", Updated: time.Date(2026, 9, 28, 10, 5, 0, 0, time.UTC),
		},
	}, plugin.Instances)

	// The allocations that run the instances, to open one of them.
	r.Len(plugin.Allocs, 3)
	r.Equal("a1", plugin.Allocs[0].ID)
}

func TestDescribePlugin(t *testing.T) {
	r := require.New(t)

	client, _ := clusterServer(t, map[string]string{"/v1/plugin/csi/aws-ebs0": csiPlugin})

	out, err := client.DescribePlugin(context.Background(), "aws-ebs0")
	r.NoError(err)
	r.Contains(out, `"Version": "1.2.0"`)
}
