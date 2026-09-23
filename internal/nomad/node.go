package nomad

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/hashicorp/nomad/api"
)

// NodeEvent is something that happened to a client, as the client itself
// reported it.
type NodeEvent struct {
	Time      time.Time
	Subsystem string
	Message   string
	Details   map[string]string
}

// Driver is one task driver of a client and what the client makes of it.
type Driver struct {
	Name        string
	Detected    bool
	Healthy     bool
	Description string
	Updated     time.Time

	// Attributes are what the driver says about itself: its version, what it
	// can run, where its socket is.
	Attributes map[string]string
}

// HostVolume is a directory of the machine that a job can ask for.
type HostVolume struct {
	Name     string
	Path     string
	ReadOnly bool
}

// MetaEntry is one metadata key of a client and where it comes from. Only
// what the API set can be changed from here; the rest is the configuration
// the agent was started with.
type MetaEntry struct {
	Key     string
	Value   string
	Dynamic bool
}

// NodeDetail is what one machine says about itself beyond the list: what
// happened to it, what it can run, what it lends out and how it is built.
type NodeDetail struct {
	ID   string
	Name string

	Events     []NodeEvent
	Drivers    []Driver
	Volumes    []HostVolume
	Attributes map[string]string
}

// NodeDetail reads everything the cluster holds about one client.
func (c *Client) NodeDetail(ctx context.Context, nodeID string) (NodeDetail, error) {
	node, _, err := c.api.Nodes().Info(nodeID, c.query(ctx, ""))
	if err != nil {
		return NodeDetail{}, err
	}

	detail := NodeDetail{
		ID:         node.ID,
		Name:       node.Name,
		Attributes: node.Attributes,
		Events:     newNodeEvents(node.Events),
		Drivers:    newDrivers(node.Drivers),
		Volumes:    newHostVolumes(node.HostVolumes),
	}

	return detail, nil
}

// newNodeEvents puts the newest first, which is the one worth reading.
func newNodeEvents(events []*api.NodeEvent) []NodeEvent {
	out := make([]NodeEvent, 0, len(events))

	for _, event := range events {
		if event == nil {
			continue
		}

		out = append(out, NodeEvent{
			Time:      event.Timestamp,
			Subsystem: event.Subsystem,
			Message:   event.Message,
			Details:   event.Details,
		})
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })

	return out
}

// newDrivers puts the drivers in the order of their names. A map hands them
// over differently every time, and the list would shuffle under the cursor.
func newDrivers(drivers map[string]*api.DriverInfo) []Driver {
	out := make([]Driver, 0, len(drivers))

	for name, info := range drivers {
		driver := Driver{Name: name}

		if info != nil {
			driver.Detected = info.Detected
			driver.Healthy = info.Healthy
			driver.Description = info.HealthDescription
			driver.Updated = info.UpdateTime
			driver.Attributes = info.Attributes
		}

		out = append(out, driver)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out
}

func newHostVolumes(volumes map[string]*api.HostVolumeInfo) []HostVolume {
	out := make([]HostVolume, 0, len(volumes))

	for name, info := range volumes {
		volume := HostVolume{Name: name}

		if info != nil {
			volume.Path = info.Path
			volume.ReadOnly = info.ReadOnly
		}

		out = append(out, volume)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out
}

// NodeMeta is the metadata of a client, each key with where it comes from.
// It is asked of the machine itself, which is the only place that knows
// which keys the API set.
func (c *Client) NodeMeta(ctx context.Context, nodeID string) ([]MetaEntry, error) {
	answer, err := c.api.Nodes().Meta().Read(nodeID, c.query(ctx, ""))
	if err != nil {
		return nil, err
	}

	out := make([]MetaEntry, 0, len(answer.Meta))

	for key, value := range answer.Meta {
		_, dynamic := answer.Dynamic[key]

		out = append(out, MetaEntry{Key: key, Value: value, Dynamic: dynamic})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })

	return out, nil
}

// NodeMetaSpec is the metadata of a client as a file, which is how it is
// edited. Only what the API set is in it: a key of the agent configuration
// would read as editable and change nothing.
func (c *Client) NodeMetaSpec(ctx context.Context, nodeID string) (string, error) {
	answer, err := c.api.Nodes().Meta().Read(nodeID, c.query(ctx, ""))
	if err != nil {
		return "", err
	}

	dynamic := map[string]string{}

	for key, value := range answer.Dynamic {
		if value != nil {
			dynamic[key] = *value
		}
	}

	return asJSON(dynamic)
}

// SubmitNodeMeta sends the metadata file back to the machine. A key that is
// no longer in the file is removed rather than left behind, which is what
// taking it out of the file means.
func (c *Client) SubmitNodeMeta(ctx context.Context, nodeID, source string) error {
	wanted := map[string]string{}
	if err := json.Unmarshal([]byte(source), &wanted); err != nil {
		return fmt.Errorf("the metadata is not valid JSON: %w", err)
	}

	answer, err := c.api.Nodes().Meta().Read(nodeID, c.query(ctx, ""))
	if err != nil {
		return err
	}

	apply := map[string]*string{}

	for key := range answer.Dynamic {
		apply[key] = nil
	}

	for key, value := range wanted {
		apply[key] = &value
	}

	_, err = c.api.Nodes().Meta().Apply(&api.NodeMetaApplyRequest{
		NodeID: nodeID,
		Meta:   apply,
	}, c.query(ctx, ""))

	return err
}
