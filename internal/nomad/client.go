// Package nomad talks to a Nomad cluster. Every call takes the namespace it
// asks in, nothing here reads a current namespace from somewhere else.
package nomad

import (
	"context"
	"time"

	"github.com/hashicorp/nomad/api"
)

// AllNamespaces is what Nomad understands as every namespace at once.
const AllNamespaces = "*"

// Config says which cluster to talk to. An empty address or region falls
// back to the environment, the same variables the nomad command reads,
// which is also where the token comes from.
type Config struct {
	Address string
	Region  string
}

// Client is the cluster, asked in one region. An empty region is the one
// of the agent it talks to.
type Client struct {
	api    *api.Client
	region string
}

// New builds a client for the cluster the config points at.
func New(cfg Config) (*Client, error) {
	c := api.DefaultConfig()

	if cfg.Address != "" {
		c.Address = cfg.Address
	}

	if cfg.Region != "" {
		c.Region = cfg.Region
	}

	client, err := api.NewClient(c)
	if err != nil {
		return nil, err
	}

	return &Client{api: client, region: c.Region}, nil
}

// InRegion is the same cluster asked in another region. The client it came
// from is left as it is: a request still out keeps the region it was
// asked in.
func (c *Client) InRegion(region string) *Client {
	return &Client{api: c.api, region: region}
}

// Address is the cluster the client talks to.
func (c *Client) Address() string {
	return c.api.Address()
}

// Region is the region the client asks in, empty for the one of the agent.
func (c *Client) Region() string {
	return c.region
}

// Job is one entry of the job list.
type Job struct {
	ID        string
	Name      string
	Namespace string
	Type      string
	Status    string

	// Running and Desired are allocations: how many are up out of how many
	// the job asks for.
	Running int
	Desired int

	// Queued are the allocations that wait for a place.
	Queued int

	SubmitTime time.Time

	// Datacenters are where the job may be placed, stars included.
	Datacenters []string
}

// Jobs lists the jobs of a namespace. AllNamespaces lists the whole cluster.
func (c *Client) Jobs(ctx context.Context, namespace string) ([]Job, error) {
	stubs, _, err := c.api.Jobs().List(c.query(ctx, namespace))
	if err != nil {
		return nil, err
	}

	jobs := make([]Job, 0, len(stubs))
	for _, stub := range stubs {
		jobs = append(jobs, newJob(stub))
	}

	return jobs, nil
}

// Agent is what the agent the client talks to says about itself.
type Agent struct {
	Version string

	// Region is where a request that names none is answered.
	Region string
}

// Agent asks the agent the client talks to. The context is here for the
// shape of the API, the agent endpoint of Nomad takes none.
func (c *Client) Agent(_ context.Context) (Agent, error) {
	self, err := c.api.Agent().Self()
	if err != nil {
		return Agent{}, err
	}

	region, _ := self.Config["Region"].(string)

	return Agent{Version: self.Member.Tags["build"], Region: region}, nil
}

func (c *Client) query(ctx context.Context, namespace string) *api.QueryOptions {
	return (&api.QueryOptions{Namespace: namespace, Region: c.region}).WithContext(ctx)
}

func newJob(stub *api.JobListStub) Job {
	job := Job{
		ID:          stub.ID,
		Name:        stub.Name,
		Namespace:   stub.Namespace,
		Type:        stub.Type,
		Status:      stub.Status,
		Datacenters: stub.Datacenters,
	}

	job.SubmitTime = unixTime(stub.SubmitTime)

	job.Running, job.Desired, job.Queued = allocationCounts(stub.JobSummary)

	return job
}

// unixTime reads a Nomad timestamp. Nomad says "never" with a zero, which is
// not 1970: a job with no submit time would read as twenty thousand days old.
func unixTime(nanos int64) time.Time {
	if nanos == 0 {
		return time.Time{}
	}

	return time.Unix(0, nanos)
}

// allocationCounts adds up the task groups of a job. Allocations in a state
// the job does not wait for are left out of the numbers.
func allocationCounts(summary *api.JobSummary) (running, desired, queued int) {
	if summary == nil {
		return 0, 0, 0
	}

	for _, group := range summary.Summary {
		running += group.Running
		desired += group.Running + group.Starting + group.Queued
		queued += group.Queued
	}

	return running, desired, queued
}
