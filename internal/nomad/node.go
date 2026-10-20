package nomad

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
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

// Driver is one task driver of a client, as the client reports it.
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

// NodeDetail is what one client reports beyond the list: its events, task
// drivers, host volumes and attributes.
type NodeDetail struct {
	ID   string
	Name string

	Events     []NodeEvent
	Drivers    []Driver
	Volumes    []HostVolume
	Attributes map[string]string
}

// NodeIdentity is the identity the servers gave a client: when it expires,
// and its claims as JSON, the way a describe shows them.
type NodeIdentity struct {
	Expires time.Time
	Claims  string
}

// ErrNoIdentity says the cluster has no identity to read for the client: it
// is older than Nomad 1.11, or the client is not connected.
var ErrNoIdentity = errors.New("the client has no identity to read")

// NodeIdentity reads the identity of a client. A client belongs to no
// namespace.
func (c *Client) NodeIdentity(ctx context.Context, nodeID string) (NodeIdentity, error) {
	got, err := c.api.Nodes().Identity().Get(&api.NodeIdentityGetRequest{NodeID: nodeID}, c.query(ctx, ""))
	if answered(err, http.StatusNotFound) {
		return NodeIdentity{}, ErrNoIdentity
	}

	if err != nil {
		return NodeIdentity{}, err
	}

	claims, err := asJSON(got.Claims)
	if err != nil {
		return NodeIdentity{}, err
	}

	identity := NodeIdentity{Claims: claims}
	if exp, ok := got.Claims["exp"].(float64); ok {
		identity.Expires = time.Unix(int64(exp), 0)
	}

	return identity, nil
}

// RenewNodeIdentity asks a client to get a new identity at its next
// heartbeat.
func (c *Client) RenewNodeIdentity(ctx context.Context, nodeID string) error {
	_, err := c.api.Nodes().Identity().Renew(&api.NodeIdentityRenewRequest{NodeID: nodeID}, c.query(ctx, ""))

	return err
}

// Node reads one client from the cluster instead of from the list, so a
// screen open on it shows its current state.
func (c *Client) Node(ctx context.Context, nodeID string) (Node, error) {
	node, _, err := c.api.Nodes().Info(nodeID, c.query(ctx, ""))
	if err != nil {
		return Node{}, err
	}

	// The answer holds more than a list entry; copying it into a list stub
	// lets newNode build both.
	full := newNode(&api.NodeListStub{
		ID:                    node.ID,
		Name:                  node.Name,
		Datacenter:            node.Datacenter,
		NodePool:              node.NodePool,
		Version:               node.Attributes["nomad.version"],
		Status:                node.Status,
		SchedulingEligibility: node.SchedulingEligibility,
		Drain:                 node.Drain,
		Address:               node.HTTPAddr,
		NodeResources:         node.NodeResources,
	})
	full.MaxAllocs = node.NodeMaxAllocs

	return full, nil
}

// WithAllocLimits returns the clients with the allocation limit of those that
// have one, and how many allocations hold a place on them. The list does not
// carry the limit, so it reads the clients the cluster says have one, and
// the allocations of those only.
func (c *Client) WithAllocLimits(ctx context.Context, nodes []Node) ([]Node, error) {
	q := c.query(ctx, "")
	q.Filter = "NodeMaxAllocs != 0"

	stubs, _, err := c.api.Nodes().List(q)
	if err != nil {
		// A cluster older than 1.10.2 may refuse a filter on a field it lacks.
		if answered(err, http.StatusBadRequest) {
			return nodes, nil
		}

		return nil, err
	}

	limits, err := c.allocLimits(ctx, nodes, stubs)
	if err != nil {
		return nil, err
	}

	if len(limits) == 0 {
		return nodes, nil
	}

	held, err := c.heldPlaces(ctx, limits)
	// A token that reads clients but no job in any namespace cannot count
	// the allocations; the clients go without a limit.
	if Forbidden(err) {
		return nodes, nil
	}

	if err != nil {
		return nil, err
	}

	out := slices.Clone(nodes)
	for i, node := range out {
		if limit, ok := limits[node.ID]; ok {
			out[i].MaxAllocs, out[i].Allocs = limit, held[node.ID]
		}
	}

	return out, nil
}

// limitReaders is how many clients are read at once.
const limitReaders = 8

// allocLimits reads the limit of each client of nodes that the filtered list
// named, a few at a time. A client that is gone since the list is left out.
func (c *Client) allocLimits(ctx context.Context, nodes []Node, stubs []*api.NodeListStub) (map[string]int, error) {
	ours := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		ours[node.ID] = true
	}

	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		firstErr error
		limits   = map[string]int{}
		slots    = make(chan struct{}, limitReaders)
	)

	for _, stub := range stubs {
		if !ours[stub.ID] {
			continue
		}

		slots <- struct{}{}

		wg.Go(func() {
			defer func() { <-slots }()

			node, _, err := c.api.Nodes().Info(stub.ID, c.query(ctx, ""))

			mu.Lock()
			defer mu.Unlock()

			switch {
			case answered(err, http.StatusNotFound):
			case err != nil:
				firstErr = cmp.Or(firstErr, err)
			case node.NodeMaxAllocs != 0:
				limits[stub.ID] = node.NodeMaxAllocs
			}
		})
	}

	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	return limits, nil
}

// heldPlaces counts, per client of limits, the allocations that hold a place
// on it. One request asks for the allocations of those clients only.
func (c *Client) heldPlaces(ctx context.Context, limits map[string]int) (map[string]int, error) {
	ids := slices.Sorted(maps.Keys(limits))

	terms := make([]string, 0, len(ids))
	for _, id := range ids {
		terms = append(terms, fmt.Sprintf("NodeID == %q", id))
	}

	q := c.query(ctx, AllNamespaces)
	q.Filter = strings.Join(terms, " or ")

	allocs, _, err := c.api.Allocations().List(q)
	if err != nil {
		return nil, err
	}

	held := map[string]int{}

	for _, alloc := range allocs {
		if holdsPlace(alloc.DesiredStatus, alloc.ClientStatus) {
			held[alloc.NodeID]++
		}
	}

	return held, nil
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
