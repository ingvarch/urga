package nomad

import (
	"context"

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

// Changes is a stream of changes from the cluster. They arrive on C until
// Close is called or the stream ends; Err says why it ended.
type Changes struct {
	C   <-chan Change
	Err <-chan error

	cancel func()
}

// Close stops the stream and the request behind it.
func (c *Changes) Close() {
	if c.cancel != nil {
		c.cancel()
	}
}

// NewChanges is a stream of changes that did not come from a cluster, so a
// test can use it in place of one.
func NewChanges(c <-chan Change, errs <-chan error, onClose func()) *Changes {
	return &Changes{C: c, Err: errs, cancel: onClose}
}

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

	ctx, cancel := context.WithCancel(ctx)

	// The stream starts with the next change: anything before it is already
	// in the list the screen read.
	events, err := c.api.EventStream().Stream(ctx, watch, 0, c.query(ctx, namespace))
	if err != nil {
		cancel()

		return nil, err
	}

	changes := make(chan Change)
	errs := make(chan error, 1)

	go func() {
		defer close(changes)

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
				select {
				case changes <- Change{
					Topic: string(event.Topic),
					Type:  event.Type,
					Key:   event.Key,
				}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return &Changes{C: changes, Err: errs, cancel: cancel}, nil
}
