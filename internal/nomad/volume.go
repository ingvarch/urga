package nomad

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/nomad/api"
)

// The kinds of volume: one a CSI plugin provides, and one a host volume
// plugin makes on a node.
const (
	VolumeCSI  = "csi"
	VolumeHost = "host"
)

// Health is how many instances of a plugin are healthy, out of how many are
// expected.
type Health struct {
	Healthy, Expected int
}

// Volume is one entry of the volume list: a CSI volume or a dynamic host
// volume.
type Volume struct {
	ID, Name, Namespace string
	Kind                string
	PluginID            string

	// NodeID, NodeName and NodePool are where a host volume lives. A CSI
	// volume is attached where it is claimed, so it has none.
	NodeID, NodeName, NodePool string

	// CapacityBytes is what a host volume holds. It is zero when not known,
	// as for a CSI volume in the list.
	CapacityBytes int64

	// State is schedulable or unschedulable for a CSI volume; pending, ready
	// or unavailable for a host volume.
	State string

	// Readers and Writers are the allocations that claim a CSI volume.
	Readers, Writers int

	// Controllers and PluginNodes are the health of the plugin of a CSI
	// volume.
	Controllers, PluginNodes Health

	Created time.Time
}

// Volumes lists the CSI and the dynamic host volumes of a namespace.
func (c *Client) Volumes(ctx context.Context, namespace string) ([]Volume, error) {
	csi, _, err := c.api.CSIVolumes().List(c.query(ctx, namespace))
	if err != nil {
		return nil, err
	}

	// A cluster older than dynamic host volumes answers with none.
	host, _, err := c.api.HostVolumes().List(nil, c.query(ctx, namespace))
	if err != nil {
		return nil, err
	}

	volumes := make([]Volume, 0, len(csi)+len(host))
	for _, v := range csi {
		volumes = append(volumes, newCSIVolume(v))
	}

	if len(host) == 0 {
		return volumes, nil
	}

	names, err := c.nodeNames(ctx)
	if err != nil {
		return nil, err
	}

	for _, v := range host {
		volume := newHostVolume(v)
		volume.NodeName = names[v.NodeID]
		volumes = append(volumes, volume)
	}

	return volumes, nil
}

// nodeNames are the names of the nodes of the cluster, by ID.
func (c *Client) nodeNames(ctx context.Context) (map[string]string, error) {
	nodes, _, err := c.api.Nodes().List(c.query(ctx, ""))
	if err != nil {
		return nil, err
	}

	names := make(map[string]string, len(nodes))
	for _, node := range nodes {
		names[node.ID] = node.Name
	}

	return names, nil
}

func newCSIVolume(v *api.CSIVolumeListStub) Volume {
	return Volume{
		ID:          v.ID,
		Name:        v.Name,
		Namespace:   v.Namespace,
		Kind:        VolumeCSI,
		PluginID:    v.PluginID,
		State:       csiState(v.Schedulable),
		Readers:     v.CurrentReaders,
		Writers:     v.CurrentWriters,
		Controllers: Health{Healthy: v.ControllersHealthy, Expected: v.ControllersExpected},
		PluginNodes: Health{Healthy: v.NodesHealthy, Expected: v.NodesExpected},
		Created:     unixTime(v.CreateTime),
	}
}

func newHostVolume(v *api.HostVolumeStub) Volume {
	return Volume{
		ID:            v.ID,
		Name:          v.Name,
		Namespace:     v.Namespace,
		Kind:          VolumeHost,
		PluginID:      v.PluginID,
		NodeID:        v.NodeID,
		NodePool:      v.NodePool,
		CapacityBytes: v.CapacityBytes,
		State:         string(v.State),
		Created:       unixTime(v.CreateTime),
	}
}

// VolumeDetail is one volume, with what the list leaves out.
type VolumeDetail struct {
	Volume

	// Of a CSI volume: the plugin that provides it, how it is attached, its
	// ID in the storage behind it, and where it can be reached.
	Provider, ProviderVersion  string
	AccessMode, AttachmentMode string
	ExternalID                 string
	Topologies                 []string

	// Of a host volume: its directory on the node, the capacity it asked
	// for, and the task group that claims it.
	HostPath                 string
	CapacityMin, CapacityMax int64
	Claim                    Claim

	// Allocs are the allocations that use the volume.
	Allocs []Alloc
}

// Claim binds a task group to a host volume: its allocations are placed
// where the volume is. An empty ID is none.
type Claim struct {
	ID, JobID, TaskGroup string
}

// Volume reads one volume of a kind.
func (c *Client) Volume(ctx context.Context, namespace, kind, id string) (VolumeDetail, error) {
	switch kind {
	case VolumeCSI:
		return c.csiVolume(ctx, namespace, id)
	case VolumeHost:
		return c.hostVolume(ctx, namespace, id)
	}

	return VolumeDetail{}, noKind(kind)
}

// DescribeVolume is what the cluster knows about a volume.
func (c *Client) DescribeVolume(ctx context.Context, namespace, kind, id string) (string, error) {
	switch kind {
	case VolumeCSI:
		v, _, err := c.api.CSIVolumes().Info(id, c.query(ctx, namespace))
		if err != nil {
			return "", err
		}

		return asJSON(v)

	case VolumeHost:
		v, _, err := c.api.HostVolumes().Get(id, c.query(ctx, namespace))
		if err != nil {
			return "", err
		}

		return asJSON(v)
	}

	return "", noKind(kind)
}

func noKind(kind string) error { return fmt.Errorf("no volume of kind %q", kind) }

func (c *Client) csiVolume(ctx context.Context, namespace, id string) (VolumeDetail, error) {
	v, _, err := c.api.CSIVolumes().Info(id, c.query(ctx, namespace))
	if err != nil {
		return VolumeDetail{}, err
	}

	return VolumeDetail{
		Volume: Volume{
			ID:            v.ID,
			Name:          v.Name,
			Namespace:     v.Namespace,
			Kind:          VolumeCSI,
			PluginID:      v.PluginID,
			CapacityBytes: v.Capacity,
			State:         csiState(v.Schedulable),
			Readers:       len(v.ReadAllocs),
			Writers:       len(v.WriteAllocs),
			Controllers:   Health{Healthy: v.ControllersHealthy, Expected: v.ControllersExpected},
			PluginNodes:   Health{Healthy: v.NodesHealthy, Expected: v.NodesExpected},
			Created:       unixTime(v.CreateTime),
		},
		Provider:        v.Provider,
		ProviderVersion: v.ProviderVersion,
		AccessMode:      string(v.AccessMode),
		AttachmentMode:  string(v.AttachmentMode),
		ExternalID:      v.ExternalID,
		Topologies:      topologies(v.Topologies),
		Allocs:          allocsOf(v.Allocations),
	}, nil
}

func (c *Client) hostVolume(ctx context.Context, namespace, id string) (VolumeDetail, error) {
	v, _, err := c.api.HostVolumes().Get(id, c.query(ctx, namespace))
	if err != nil {
		return VolumeDetail{}, err
	}

	// A node that is gone leaves the volume without a name for it, not
	// without the volume.
	names, err := c.nodeNames(ctx)
	if err != nil {
		return VolumeDetail{}, err
	}

	claim, err := c.claimOf(ctx, namespace, v)
	if err != nil {
		return VolumeDetail{}, err
	}

	volume := newHostVolume(&api.HostVolumeStub{
		ID: v.ID, Name: v.Name, Namespace: v.Namespace, PluginID: v.PluginID,
		NodeID: v.NodeID, NodePool: v.NodePool, CapacityBytes: v.CapacityBytes,
		State: v.State, CreateTime: v.CreateTime,
	})
	volume.NodeName = names[v.NodeID]

	return VolumeDetail{
		Volume:      volume,
		HostPath:    v.HostPath,
		CapacityMin: v.RequestedCapacityMinBytes,
		CapacityMax: v.RequestedCapacityMaxBytes,
		Claim:       claim,
		Allocs:      allocsOf(v.Allocations),
	}, nil
}

// claimOf is the task group that claims a host volume. Claims are listed by
// the name of the volume, which other volumes share.
func (c *Client) claimOf(ctx context.Context, namespace string, v *api.HostVolume) (Claim, error) {
	claims, _, err := c.api.TaskGroupHostVolumeClaims().List(
		&api.TaskGroupHostVolumeClaimsListRequest{VolumeName: v.Name}, c.query(ctx, namespace))
	if err != nil {
		return Claim{}, err
	}

	for _, claim := range claims {
		if claim != nil && claim.VolumeID == v.ID {
			return Claim{ID: claim.ID, JobID: claim.JobID, TaskGroup: claim.TaskGroupName}, nil
		}
	}

	return Claim{}, nil
}

// topologies are where a volume can be reached, one line each, its
// segments in order.
func topologies(list []*api.CSITopology) []string {
	out := make([]string, 0, len(list))

	for _, topology := range list {
		if topology == nil {
			continue
		}

		segments := make([]string, 0, len(topology.Segments))
		for key, value := range topology.Segments {
			segments = append(segments, key+"="+value)
		}

		slices.Sort(segments)
		out = append(out, strings.Join(segments, ", "))
	}

	return out
}

func csiState(schedulable bool) string {
	if schedulable {
		return "schedulable"
	}

	return "unschedulable"
}

// DetachVolume detaches a CSI volume from a node, which releases the claim
// an allocation there holds on it.
func (c *Client) DetachVolume(ctx context.Context, namespace, volumeID, nodeID string) error {
	return c.api.CSIVolumes().Detach(volumeID, nodeID, c.write(ctx, namespace))
}

// ReleaseClaim deletes the claim of a task group on a host volume: its next
// allocation may be placed with another one.
func (c *Client) ReleaseClaim(ctx context.Context, namespace, claimID string) error {
	_, err := c.api.TaskGroupHostVolumeClaims().Delete(claimID, c.write(ctx, namespace))

	return err
}
