package nomad

import (
	"context"
	"time"
)

// Deployment is a rollout of a job version.
type Deployment struct {
	ID                string
	JobID             string
	Namespace         string
	JobVersion        uint64
	Status            string
	StatusDescription string
}

// Deployments lists the rollouts of a namespace.
func (c *Client) Deployments(ctx context.Context, namespace string) ([]Deployment, error) {
	list, _, err := c.api.Deployments().List(c.query(ctx, namespace))
	if err != nil {
		return nil, err
	}

	out := make([]Deployment, 0, len(list))
	for _, d := range list {
		out = append(out, Deployment{
			ID:                d.ID,
			JobID:             d.JobID,
			Namespace:         d.Namespace,
			JobVersion:        d.JobVersion,
			Status:            d.Status,
			StatusDescription: d.StatusDescription,
		})
	}

	return out, nil
}

// Namespace is one namespace of the cluster.
type Namespace struct {
	Name        string
	Description string
	Quota       string
}

// Namespaces lists the namespaces of the cluster.
func (c *Client) Namespaces(ctx context.Context) ([]Namespace, error) {
	list, _, err := c.api.Namespaces().List(c.query(ctx, ""))
	if err != nil {
		return nil, err
	}

	out := make([]Namespace, 0, len(list))
	for _, n := range list {
		out = append(out, Namespace{Name: n.Name, Description: n.Description, Quota: n.Quota})
	}

	return out, nil
}

// Service is one name in the service catalog.
type Service struct {
	Name      string
	Namespace string
	Tags      []string
}

// Services lists the service catalog of a namespace.
func (c *Client) Services(ctx context.Context, namespace string) ([]Service, error) {
	list, _, err := c.api.Services().List(c.query(ctx, namespace))
	if err != nil {
		return nil, err
	}

	// Nomad groups them by namespace, the list shows one service per row.
	out := []Service{}
	for _, group := range list {
		for _, service := range group.Services {
			out = append(out, Service{
				Name:      service.ServiceName,
				Namespace: group.Namespace,
				Tags:      service.Tags,
			})
		}
	}

	return out, nil
}

// Evaluation is a scheduling decision: why a job runs, or why it does not.
type Evaluation struct {
	ID                string
	JobID             string
	Namespace         string
	Type              string
	TriggeredBy       string
	Status            string
	StatusDescription string
	Created           time.Time
}

// Evaluations lists the scheduling decisions of a namespace.
func (c *Client) Evaluations(ctx context.Context, namespace string) ([]Evaluation, error) {
	list, _, err := c.api.Evaluations().List(c.query(ctx, namespace))
	if err != nil {
		return nil, err
	}

	out := make([]Evaluation, 0, len(list))
	for _, e := range list {
		eval := Evaluation{
			ID:                e.ID,
			JobID:             e.JobID,
			Namespace:         e.Namespace,
			Type:              e.Type,
			TriggeredBy:       e.TriggeredBy,
			Status:            e.Status,
			StatusDescription: e.StatusDescription,
		}

		if e.CreateTime != 0 {
			eval.Created = time.Unix(0, e.CreateTime)
		}

		out = append(out, eval)
	}

	return out, nil
}

// Node is a client of the cluster, the machine work is placed on.
type Node struct {
	ID          string
	Name        string
	Datacenter  string
	NodePool    string
	Version     string
	Status      string
	Eligibility string
	Drain       bool
	Address     string
}

// Nodes lists the clients of the cluster.
func (c *Client) Nodes(ctx context.Context) ([]Node, error) {
	list, _, err := c.api.Nodes().List(c.query(ctx, ""))
	if err != nil {
		return nil, err
	}

	out := make([]Node, 0, len(list))
	for _, n := range list {
		out = append(out, Node{
			ID:          n.ID,
			Name:        n.Name,
			Datacenter:  n.Datacenter,
			NodePool:    n.NodePool,
			Version:     n.Version,
			Status:      n.Status,
			Eligibility: n.SchedulingEligibility,
			Drain:       n.Drain,
			Address:     n.Address,
		})
	}

	return out, nil
}

// Variable is an entry of the variable store. Only what the list says, the
// values themselves are not read.
type Variable struct {
	Path      string
	Namespace string
	Created   time.Time
	Modified  time.Time
}

// Variables lists what the variable store of a namespace holds.
func (c *Client) Variables(ctx context.Context, namespace string) ([]Variable, error) {
	list, _, err := c.api.Variables().List(c.query(ctx, namespace))
	if err != nil {
		return nil, err
	}

	out := make([]Variable, 0, len(list))
	for _, v := range list {
		variable := Variable{Path: v.Path, Namespace: v.Namespace}

		if v.CreateTime != 0 {
			variable.Created = time.Unix(0, v.CreateTime)
		}

		if v.ModifyTime != 0 {
			variable.Modified = time.Unix(0, v.ModifyTime)
		}

		out = append(out, variable)
	}

	return out, nil
}

// NodePool groups the nodes a job can be placed on.
type NodePool struct {
	Name        string
	Description string
	Scheduler   string
}

// NodePools lists the pools nodes are grouped into.
func (c *Client) NodePools(ctx context.Context) ([]NodePool, error) {
	list, _, err := c.api.NodePools().List(c.query(ctx, ""))
	if err != nil {
		return nil, err
	}

	out := make([]NodePool, 0, len(list))
	for _, p := range list {
		pool := NodePool{Name: p.Name, Description: p.Description}

		if p.SchedulerConfiguration != nil {
			pool.Scheduler = string(p.SchedulerConfiguration.SchedulerAlgorithm)
		}

		out = append(out, pool)
	}

	return out, nil
}

// TaskGroup is one group of a job: how many allocations it asks for and how
// many of them are up.
type TaskGroup struct {
	Name  string
	JobID string
	Count int

	Running  int
	Starting int
	Queued   int
	Complete int
	Failed   int
	Lost     int
}

// TaskGroups lists the groups of a job with what the cluster made of them.
func (c *Client) TaskGroups(ctx context.Context, namespace, jobID string) ([]TaskGroup, error) {
	job, _, err := c.api.Jobs().Info(jobID, c.query(ctx, namespace))
	if err != nil {
		return nil, err
	}

	summary, _, err := c.api.Jobs().Summary(jobID, c.query(ctx, namespace))
	if err != nil {
		return nil, err
	}

	groups := make([]TaskGroup, 0, len(job.TaskGroups))
	for _, group := range job.TaskGroups {
		if group == nil || group.Name == nil {
			continue
		}

		tg := TaskGroup{Name: *group.Name, JobID: jobID}

		if group.Count != nil {
			tg.Count = *group.Count
		}

		if summary != nil {
			if s, ok := summary.Summary[tg.Name]; ok {
				tg.Running, tg.Starting = s.Running, s.Starting
				tg.Queued, tg.Complete = s.Queued, s.Complete
				tg.Failed, tg.Lost = s.Failed, s.Lost
			}
		}

		groups = append(groups, tg)
	}

	return groups, nil
}
