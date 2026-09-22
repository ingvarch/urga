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

// Config says which cluster to talk to. An empty field falls back to the
// environment, the same variables the nomad command reads.
type Config struct {
	Address string
	Region  string
	Token   string
}

// Client is the cluster.
type Client struct {
	api *api.Client
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

	if cfg.Token != "" {
		c.SecretID = cfg.Token
	}

	client, err := api.NewClient(c)
	if err != nil {
		return nil, err
	}

	return &Client{api: client}, nil
}

// Address is the cluster the client talks to.
func (c *Client) Address() string {
	return c.api.Address()
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

	SubmitTime time.Time
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

// Version is the build of the agent the client talks to. The context is here
// for the shape of the API, the agent endpoint of Nomad takes none.
func (c *Client) Version(_ context.Context) (string, error) {
	self, err := c.api.Agent().Self()
	if err != nil {
		return "", err
	}

	return self.Member.Tags["build"], nil
}

func (c *Client) query(ctx context.Context, namespace string) *api.QueryOptions {
	return (&api.QueryOptions{Namespace: namespace}).WithContext(ctx)
}

func newJob(stub *api.JobListStub) Job {
	job := Job{
		ID:        stub.ID,
		Name:      stub.Name,
		Namespace: stub.Namespace,
		Type:      stub.Type,
		Status:    stub.Status,
	}

	if stub.SubmitTime != 0 {
		job.SubmitTime = time.Unix(0, stub.SubmitTime)
	}

	job.Running, job.Desired = allocationCounts(stub.JobSummary)

	return job
}

// allocationCounts adds up the task groups of a job. Allocations in a state
// the job does not wait for are left out of both numbers.
func allocationCounts(summary *api.JobSummary) (running, desired int) {
	if summary == nil {
		return 0, 0
	}

	for _, group := range summary.Summary {
		running += group.Running
		desired += group.Running + group.Starting + group.Queued
	}

	return running, desired
}
