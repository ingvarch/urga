package nomad_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// csiList and hostList are the two kinds of volume as the cluster lists them.
const (
	csiList = `[{
		"ID": "pg-data", "Name": "pg-data", "Namespace": "production", "PluginID": "aws-ebs0",
		"Schedulable": true, "CurrentReaders": 2, "CurrentWriters": 1,
		"ControllersHealthy": 2, "ControllersExpected": 2, "NodesHealthy": 5, "NodesExpected": 6,
		"CreateTime": 1758499200000000000
	}]`

	hostList = `[{
		"ID": "7f3c1a2b-0000-0000-0000-000000000000", "Name": "scratch", "Namespace": "production",
		"PluginID": "mkdir", "NodeID": "node-1", "NodePool": "default",
		"CapacityBytes": 1073741824, "State": "ready", "CreateTime": 1758499200000000000
	}]`
)

func TestVolumes_BothKinds(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{
		"/v1/volumes?type=csi":  csiList,
		"/v1/volumes?type=host": hostList,
		"/v1/nodes":             `[{"ID": "node-1", "Name": "node-01"}]`,
	})

	volumes, err := client.Volumes(context.Background(), "production")
	r.NoError(err)
	r.Len(volumes, 2)

	// Both lists are asked in the namespace of the session.
	for _, req := range *asked {
		if req.path == "/v1/volumes" {
			r.Equal("production", req.namespace)
		}
	}

	csi := volumes[0]
	r.Equal(nomad.VolumeCSI, csi.Kind)
	r.Equal("pg-data", csi.ID)
	r.Equal("aws-ebs0", csi.PluginID)
	r.Equal("schedulable", csi.State)
	r.Equal(2, csi.Readers)
	r.Equal(1, csi.Writers)
	r.Equal(nomad.Health{Healthy: 2, Expected: 2}, csi.Controllers)
	r.Equal(nomad.Health{Healthy: 5, Expected: 6}, csi.PluginNodes)
	r.Equal(time.Unix(0, 1758499200000000000).UTC(), csi.Created.UTC())

	// A CSI volume is attached where it is claimed: it lives on no node,
	// and the list does not say how big it is.
	r.Empty(csi.NodeName)
	r.Zero(csi.CapacityBytes)

	host := volumes[1]
	r.Equal(nomad.VolumeHost, host.Kind)
	r.Equal("scratch", host.Name)
	r.Equal("mkdir", host.PluginID)
	r.Equal("node-1", host.NodeID)
	r.Equal("node-01", host.NodeName)
	r.Equal("default", host.NodePool)
	r.Equal(int64(1073741824), host.CapacityBytes)
	r.Equal("ready", host.State)
}

func TestVolumes_ACSIVolumeNobodyCanUse(t *testing.T) {
	r := require.New(t)

	client, _ := clusterServer(t, map[string]string{
		"/v1/volumes?type=csi":  `[{"ID": "pg-data", "Schedulable": false}]`,
		"/v1/volumes?type=host": `[]`,
	})

	volumes, err := client.Volumes(context.Background(), "production")
	r.NoError(err)
	r.Equal("unschedulable", volumes[0].State)
}

func TestVolumes_WithoutHostVolumesNoNodesAreAsked(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{
		"/v1/volumes?type=csi":  csiList,
		"/v1/volumes?type=host": `[]`,
	})

	_, err := client.Volumes(context.Background(), "production")
	r.NoError(err)

	// The nodes only name where host volumes live.
	for _, req := range *asked {
		r.NotEqual("/v1/nodes", req.path)
	}
}

func TestVolumes_AClusterBeforeHostVolumes(t *testing.T) {
	r := require.New(t)

	// A cluster older than dynamic host volumes answers the other type with
	// nothing at all.
	client, _ := clusterServer(t, map[string]string{
		"/v1/volumes?type=csi":  csiList,
		"/v1/volumes?type=host": `null`,
	})

	volumes, err := client.Volumes(context.Background(), "production")
	r.NoError(err)
	r.Len(volumes, 1)
}

const csiVolume = `{
	"ID": "pg-data", "Name": "pg-data", "Namespace": "production", "ExternalID": "vol-0abc123",
	"PluginID": "aws-ebs0", "Provider": "ebs.csi.aws.com", "ProviderVersion": "1.2.0",
	"AccessMode": "single-node-writer", "AttachmentMode": "file-system",
	"Capacity": 10737418240, "Schedulable": true,
	"Topologies": [{"Segments": {"zone": "eu-west-1a", "region": "eu-west-1"}}],
	"ControllersHealthy": 2, "ControllersExpected": 2, "NodesHealthy": 5, "NodesExpected": 6,
	"ReadAllocs": {}, "WriteAllocs": {"alloc-1": null},
	"Allocations": [{"ID": "alloc-1", "JobID": "db", "TaskGroup": "db", "NodeID": "node-3", "NodeName": "node-03", "ClientStatus": "running"}]
}`

func TestVolume_CSI(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{"/v1/volume/csi/pg-data": csiVolume})

	volume, err := client.Volume(context.Background(), "production", nomad.VolumeCSI, "pg-data")
	r.NoError(err)
	r.Equal("production", (*asked)[0].namespace)

	r.Equal(nomad.VolumeCSI, volume.Kind)
	r.Equal("schedulable", volume.State)
	r.Equal("ebs.csi.aws.com", volume.Provider)
	r.Equal("1.2.0", volume.ProviderVersion)
	r.Equal("single-node-writer", volume.AccessMode)
	r.Equal("file-system", volume.AttachmentMode)
	r.Equal("vol-0abc123", volume.ExternalID)
	r.Equal(int64(10737418240), volume.CapacityBytes)
	r.Equal([]string{"region=eu-west-1, zone=eu-west-1a"}, volume.Topologies)
	r.Equal(nomad.Health{Healthy: 5, Expected: 6}, volume.PluginNodes)

	// The claims, and the allocations that hold them.
	r.Equal(0, volume.Readers)
	r.Equal(1, volume.Writers)
	r.Len(volume.Allocs, 1)
	r.Equal("node-03", volume.Allocs[0].NodeName)
}

const hostVolume = `{
	"ID": "7f3c1a2b-0000-0000-0000-000000000000", "Name": "scratch", "Namespace": "production",
	"PluginID": "mkdir", "NodeID": "node-1", "NodePool": "default",
	"HostPath": "/opt/nomad/volumes/7f3c1a2b", "CapacityBytes": 1073741824,
	"RequestedCapacityMinBytes": 1073741824, "RequestedCapacityMaxBytes": 2147483648,
	"State": "ready",
	"Allocations": [{"ID": "alloc-2", "JobID": "web", "TaskGroup": "cache", "NodeID": "node-1", "NodeName": "node-01", "ClientStatus": "running"}]
}`

func TestVolume_Host(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{
		"/v1/volume/host/7f3c1a2b-0000-0000-0000-000000000000": hostVolume,
		"/v1/nodes": `[{"ID": "node-1", "Name": "node-01"}]`,
		"/v1/volumes/claims": `[
			{"ID": "claim-2", "JobID": "web", "TaskGroupName": "cache", "VolumeID": "another-volume", "VolumeName": "scratch"},
			{"ID": "claim-1", "JobID": "web", "TaskGroupName": "cache", "VolumeID": "7f3c1a2b-0000-0000-0000-000000000000", "VolumeName": "scratch"}
		]`,
	})

	volume, err := client.Volume(context.Background(), "production", nomad.VolumeHost, "7f3c1a2b-0000-0000-0000-000000000000")
	r.NoError(err)

	r.Equal(nomad.VolumeHost, volume.Kind)
	r.Equal("node-01", volume.NodeName)
	r.Equal("default", volume.NodePool)
	r.Equal("/opt/nomad/volumes/7f3c1a2b", volume.HostPath)
	r.Equal(int64(1073741824), volume.CapacityBytes)
	r.Equal(int64(1073741824), volume.CapacityMin)
	r.Equal(int64(2147483648), volume.CapacityMax)
	r.Len(volume.Allocs, 1)

	// Claims are found by the name of the volume; this one's is the one
	// with its ID.
	r.Equal(nomad.Claim{ID: "claim-1", JobID: "web", TaskGroup: "cache"}, volume.Claim)

	for _, req := range *asked {
		if req.path == "/v1/volumes/claims" {
			r.Equal("scratch", req.query.Get("volume_name"))
			r.Equal("production", req.namespace)
		}
	}
}

func TestVolume_HostWithoutAClaim(t *testing.T) {
	r := require.New(t)

	client, _ := clusterServer(t, map[string]string{
		"/v1/volume/host/7f3c1a2b-0000-0000-0000-000000000000": hostVolume,
		"/v1/nodes":          `[]`,
		"/v1/volumes/claims": `[]`,
	})

	volume, err := client.Volume(context.Background(), "production", nomad.VolumeHost, "7f3c1a2b-0000-0000-0000-000000000000")
	r.NoError(err)
	r.Empty(volume.Claim.ID)
}

func TestVolume_AKindThatIsNot(t *testing.T) {
	r := require.New(t)

	client, _ := clusterServer(t, map[string]string{})

	_, err := client.Volume(context.Background(), "production", "nfs", "share")
	r.ErrorContains(err, "nfs")
}

func TestDescribeVolume(t *testing.T) {
	r := require.New(t)

	client, _ := clusterServer(t, map[string]string{
		"/v1/volume/csi/pg-data":                               csiVolume,
		"/v1/volume/host/7f3c1a2b-0000-0000-0000-000000000000": hostVolume,
	})

	out, err := client.DescribeVolume(context.Background(), "production", nomad.VolumeCSI, "pg-data")
	r.NoError(err)
	r.Contains(out, `"ExternalID": "vol-0abc123"`)

	out, err = client.DescribeVolume(context.Background(), "production", nomad.VolumeHost, "7f3c1a2b-0000-0000-0000-000000000000")
	r.NoError(err)
	r.Contains(out, `"HostPath": "/opt/nomad/volumes/7f3c1a2b"`)
}

func TestDetachVolume(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{}`)

	r.NoError(client.DetachVolume(context.Background(), "production", "pg-data", "node-3"))

	// From the one node the claim is on.
	r.Equal(http.MethodDelete, asked.Method)
	r.Equal("/v1/volume/csi/pg-data/detach", asked.URL.Path)
	r.Equal("node-3", asked.URL.Query().Get("node"))
	r.Equal("production", asked.URL.Query().Get("namespace"))
}

func TestReleaseClaim(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{}`)

	r.NoError(client.ReleaseClaim(context.Background(), "production", "claim-1"))

	r.Equal(http.MethodDelete, asked.Method)
	r.Equal("/v1/volumes/claim/claim-1", asked.URL.Path)
	r.Equal("production", asked.URL.Query().Get("namespace"))
}
