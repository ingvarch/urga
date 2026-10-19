package nomad

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"time"

	"github.com/hashicorp/nomad/api"
)

// The topics of the cluster a screen can watch.
const (
	TopicJob        = "Job"
	TopicAllocation = "Allocation"
	TopicDeployment = "Deployment"
	TopicEvaluation = "Evaluation"
	TopicNode       = "Node"
	TopicNodePool   = "NodePool"
	TopicService    = "Service"

	// Nomad's api module has no constant for these.
	TopicCSIVolume  = "CSIVolume"
	TopicCSIPlugin  = "CSIPlugin"
	TopicHostVolume = "HostVolume"
	TopicVariable   = "Variable"
)

// since is the first version of Nomad that sends the events of a topic. A
// topic not in it is older than any cluster urga reads.
var since = map[string]string{
	TopicCSIVolume:  "1.10.0",
	TopicCSIPlugin:  "1.10.0",
	TopicHostVolume: "1.10.0",
	TopicVariable:   "2.0.0",
}

// Streams says a cluster of that version sends the events of every one of
// the topics. A cluster that does not know a topic opens the stream anyway
// and never sends anything for it, so the version is checked before it is
// asked.
func Streams(version string, topics []string) bool {
	for _, topic := range topics {
		if minimum, ok := since[topic]; ok && !AtLeast(version, minimum) {
			return false
		}
	}

	return true
}

var numbers = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)`)

// AtLeast says version reads as major.minor.patch at its start, and is at or
// above the minimum. What follows the patch (-beta.1, +ent) is not compared.
func AtLeast(version, minimum string) bool {
	have, ok := triple(version)
	if !ok {
		return false
	}

	need, ok := triple(minimum)
	if !ok {
		return false
	}

	return have.compare(need) >= 0
}

type release [3]int

func (a release) compare(b release) int {
	for i := range a {
		if a[i] != b[i] {
			return a[i] - b[i]
		}
	}

	return 0
}

func triple(version string) (release, bool) {
	parts := numbers.FindStringSubmatch(version)
	if parts == nil {
		return release{}, false
	}

	var r release

	for i := range r {
		n, err := strconv.Atoi(parts[i+1])
		if err != nil {
			return release{}, false
		}

		r[i] = n
	}

	return r, true
}

// Change reports that something in the cluster changed. It carries no new
// state: the screen reads that with its usual request.
type Change struct {
	Topic string
	Type  string

	// Key is what changed: the id of a job, an allocation, a node. The
	// namespace is not in it; the stream is asked for one namespace at a
	// time.
	Key string
}

// Stream is what the cluster sends as it happens. It arrives on C until
// Close is called or the stream ends; Err says why it ended.
type Stream[T any] struct {
	C   <-chan T
	Err <-chan error

	cancel func()
}

// Close stops the stream and the request behind it.
func (s *Stream[T]) Close() {
	if s.cancel != nil {
		s.cancel()
	}
}

// NewStream is a stream that did not come from a cluster, so a test can use
// it in place of one.
func NewStream[T any](c <-chan T, errs <-chan error, onClose func()) *Stream[T] {
	return &Stream[T]{C: c, Err: errs, cancel: onClose}
}

// Changes is a stream of changes from the cluster.
type Changes = Stream[Change]

// Events follows what happens in the cluster. The caller closes the stream
// when it stops reading, otherwise the request stays open.
//
// A cluster that does not stream returns an error at once: the caller then
// keeps polling the way it did before.
func (c *Client) Events(ctx context.Context, namespace string, topics []string) (*Changes, error) {
	watch := map[api.Topic][]string{}
	for _, topic := range topics {
		watch[api.Topic(topic)] = nil
	}

	// The stream starts with the next change: anything before it is already
	// in the list the screen read.
	return follow(ctx, c, namespace, watch, 0, func(event api.Event) (Change, bool) {
		return Change{Topic: string(event.Topic), Type: event.Type, Key: event.Key}, true
	})
}

// Event is one thing that happened in the cluster: what the event names, and
// the state and the time of change the object had.
type Event struct {
	Index                       uint64
	Topic, Type, Namespace, Key string

	Name, State string
	At          time.Time
}

// Feed is what happens in the cluster, event by event.
type Feed = Stream[Event]

// Subject is what a feed follows: one job, one allocation, or, when it is
// empty, the whole namespace.
type Subject struct{ JobID, AllocID string }

// topics are what the cluster is asked for. An allocation, an evaluation and
// a deployment carry the ID of their job as a filter key, so the cluster
// sends the work of a job by the job's ID. Services are left out: their
// filter keys hold the service name, which another job's service can share.
func (s Subject) topics() map[api.Topic][]string {
	switch {
	case s.AllocID != "":
		return map[api.Topic][]string{api.TopicAllocation: {s.AllocID}}
	case s.JobID != "":
		key := []string{s.JobID}

		return map[api.Topic][]string{
			api.TopicJob: key, api.TopicAllocation: key, api.TopicEvaluation: key, api.TopicDeployment: key,
		}
	}

	return map[api.Topic][]string{api.TopicAll: nil}
}

// Feed follows the subject in a namespace, from the event of an index on: 1
// is the oldest the cluster still keeps. The caller closes it when it stops
// reading.
func (c *Client) Feed(ctx context.Context, namespace string, subject Subject, from uint64) (*Feed, error) {
	// Asked for an index past its newest event, the cluster starts at that
	// event again: what came before the index was read already.
	return follow(ctx, c, namespace, subject.topics(), from, func(event api.Event) (Event, bool) {
		return newEvent(event), event.Index >= from
	})
}

// newEvent reads an event: what it names, and the namespace, the state and
// the time of change of the object it carries. A topic urga reads nothing of
// is named by its key.
func newEvent(e api.Event) Event {
	event := Event{Index: e.Index, Topic: string(e.Topic), Type: e.Type, Key: e.Key, Name: e.Key}

	switch e.Topic {
	case api.TopicAllocation:
		if a, err := e.Allocation(); err == nil && a != nil {
			event.Namespace, event.Name, event.State, event.At = a.Namespace, a.Name, a.ClientStatus, unixTime(a.ModifyTime)
		}
	case api.TopicJob:
		if j, err := e.Job(); err == nil && j != nil {
			event.Namespace, event.Name = valueOf(j.Namespace), valueOf(j.ID)
			event.State, event.At = valueOf(j.Status), unixTime(valueOf(j.SubmitTime))
		}
	case api.TopicDeployment:
		if d, err := e.Deployment(); err == nil && d != nil {
			event.Namespace, event.Name, event.State, event.At = d.Namespace, d.JobID, d.Status, unixTime(d.ModifyTime)
		}
	case api.TopicEvaluation:
		if ev, err := e.Evaluation(); err == nil && ev != nil {
			event.Namespace, event.Name, event.State, event.At = ev.Namespace, ev.JobID, ev.Status, unixTime(ev.ModifyTime)
		}
	case api.TopicNode:
		if n, err := e.Node(); err == nil && n != nil {
			event.Name, event.State = n.Name, n.Status
		}
	case api.TopicNodePool:
		if p, err := e.NodePool(); err == nil && p != nil {
			event.Name = p.Name
		}
	case api.TopicService:
		if sr, err := e.Service(); err == nil && sr != nil {
			event.Namespace, event.Name = sr.Namespace, sr.ServiceName
		}
	default:
		event.readStored(e)
	}

	return event
}

// readStored names an event of a topic the api module cannot decode.
func (event *Event) readStored(e api.Event) {
	switch e.Topic {
	case TopicCSIVolume:
		if v, ok := payloadOf(e, "Volume"); ok {
			event.Namespace, event.Name, event.State, event.At = v.Namespace, v.Name, csiState(v.Schedulable), unixTime(v.ModifyTime)
		}
	case TopicHostVolume:
		if v, ok := payloadOf(e, "Volume"); ok {
			event.Namespace, event.Name, event.State, event.At = v.Namespace, v.Name, v.State, unixTime(v.ModifyTime)

			// A deleted volume has no key of its own.
			if event.Key == "" {
				event.Key = v.ID
			}
		}
	case TopicCSIPlugin:
		if v, ok := payloadOf(e, "Plugin"); ok {
			event.Name, event.At = v.ID, unixTime(v.ModifyTime)
		}
	case TopicVariable:
		if v, ok := payloadOf(e, "Metadata"); ok {
			event.Namespace, event.Name, event.At = v.Namespace, v.Path, unixTime(v.ModifyTime)
		}
	}
}

// stored is the fields urga reads of an object the api module cannot decode.
type stored struct {
	ID, Name, Namespace, Path, State string
	Schedulable                      bool
	ModifyTime                       int64
}

// payloadOf reads the object an event carries under a key of its payload.
func payloadOf(e api.Event, key string) (stored, bool) {
	raw, ok := e.Payload[key]
	if !ok {
		return stored{}, false
	}

	data, err := json.Marshal(raw)
	if err != nil {
		return stored{}, false
	}

	var v stored

	return v, json.Unmarshal(data, &v) == nil
}

// streamBuffer is how many events of a stream wait for the reader.
const streamBuffer = 256

// follow streams the topics of a namespace from an index, and sends what
// read makes of each event it keeps until the stream ends or is closed.
func follow[T any](ctx context.Context, c *Client, namespace string, topics map[api.Topic][]string, index uint64,
	read func(api.Event) (T, bool),
) (*Stream[T], error) {
	ctx, cancel := context.WithCancel(ctx)

	events, err := c.api.EventStream().Stream(ctx, topics, index, c.query(ctx, namespace))
	if err != nil {
		cancel()

		return nil, err
	}

	// What arrives while nobody reads waits here, so that the reader takes it
	// in one go.
	out := make(chan T, streamBuffer)
	errs := make(chan error, 1)

	go func() {
		defer close(out)

		for batch := range events {
			if batch == nil {
				continue
			}

			if batch.Err != nil {
				select {
				case errs <- batch.Err:
				default:
				}

				return
			}

			for _, event := range batch.Events {
				item, keep := read(event)
				if !keep {
					continue
				}

				select {
				case out <- item:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return &Stream[T]{C: out, Err: errs, cancel: cancel}, nil
}
