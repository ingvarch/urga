package nomad

import (
	"context"
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
)

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

// Feed follows every topic of the cluster in a namespace, from the event of
// an index on: 1 is the oldest the cluster still keeps. The caller closes it
// when it stops reading.
func (c *Client) Feed(ctx context.Context, namespace string, from uint64) (*Feed, error) {
	// Asked for an index past its newest event, the cluster starts at that
	// event again: what came before the index was read already.
	return follow(ctx, c, namespace, map[api.Topic][]string{api.TopicAll: nil}, from, func(event api.Event) (Event, bool) {
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
	}

	return event
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
