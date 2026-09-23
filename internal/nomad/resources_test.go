package nomad_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// Every list call names the namespace it asks in, and asks the endpoint that
// holds the resource.
func TestResources_AskTheRightEndpoint(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		path      string
		namespace string
		call      func(*nomad.Client) error
	}{
		{
			name: "deployments", body: `[]`, path: "/v1/deployments", namespace: "production",
			call: func(c *nomad.Client) error {
				_, err := c.Deployments(context.Background(), "production")
				return err
			},
		},
		{
			name: "namespaces", body: `[]`, path: "/v1/namespaces",
			call: func(c *nomad.Client) error {
				_, err := c.Namespaces(context.Background())
				return err
			},
		},
		{
			name: "services", body: `[]`, path: "/v1/services", namespace: "*",
			call: func(c *nomad.Client) error {
				_, err := c.Services(context.Background(), nomad.AllNamespaces)
				return err
			},
		},
		{
			name: "evaluations", body: `[]`, path: "/v1/evaluations", namespace: "production",
			call: func(c *nomad.Client) error {
				_, err := c.Evaluations(context.Background(), "production")
				return err
			},
		},
		{
			name: "nodes", body: `[]`, path: "/v1/nodes",
			call: func(c *nomad.Client) error {
				_, err := c.Nodes(context.Background())
				return err
			},
		},
		{
			name: "variables", body: `[]`, path: "/v1/vars", namespace: "production",
			call: func(c *nomad.Client) error {
				_, err := c.Variables(context.Background(), "production")
				return err
			},
		},
		{
			name: "node pools", body: `[]`, path: "/v1/node/pools",
			call: func(c *nomad.Client) error {
				_, err := c.NodePools(context.Background())
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := require.New(t)

			client, asked := recorder(t, test.body)

			r.NoError(test.call(client))
			r.Equal(test.path, asked.URL.Path)
			r.Equal(test.namespace, asked.URL.Query().Get("namespace"))
		})
	}
}

func TestDeployments_Read(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `[{"ID": "dep-1", "JobID": "web", "Namespace": "production", "Status": "running", "StatusDescription": "Deployment is running", "JobVersion": 3}]`)

	deployments, err := client.Deployments(context.Background(), "production")
	r.NoError(err)
	r.Len(deployments, 1)

	r.Equal("dep-1", deployments[0].ID)
	r.Equal("web", deployments[0].JobID)
	r.Equal("running", deployments[0].Status)
	r.Equal(uint64(3), deployments[0].JobVersion)
}

func TestNamespaces_Read(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `[{"Name": "production", "Description": "live", "Quota": "prod"}]`)

	namespaces, err := client.Namespaces(context.Background())
	r.NoError(err)
	r.Len(namespaces, 1)

	r.Equal("production", namespaces[0].Name)
	r.Equal("live", namespaces[0].Description)
	r.Equal("prod", namespaces[0].Quota)
}

func TestServices_Read(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `[{"Namespace": "production", "Services": [{"ServiceName": "api", "Tags": ["http", "urlprefix-/api"]}]}]`)

	services, err := client.Services(context.Background(), nomad.AllNamespaces)
	r.NoError(err)
	r.Len(services, 1)

	// Nomad answers with the services grouped by namespace, the list shows
	// them one by one.
	r.Equal("api", services[0].Name)
	r.Equal("production", services[0].Namespace)
	r.Equal([]string{"http", "urlprefix-/api"}, services[0].Tags)
}

func TestEvaluations_Read(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `[{"ID": "eval-1", "JobID": "web", "Namespace": "production", "Type": "service", "TriggeredBy": "job-register", "Status": "complete", "CreateTime": 1758499200000000000}]`)

	evals, err := client.Evaluations(context.Background(), "production")
	r.NoError(err)
	r.Len(evals, 1)

	r.Equal("eval-1", evals[0].ID)
	r.Equal("job-register", evals[0].TriggeredBy)
	r.Equal("complete", evals[0].Status)
	r.False(evals[0].Created.IsZero())
}

func TestNodes_Read(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `[{"ID": "node-1", "Name": "nomad-server-01", "Datacenter": "dc1", "NodePool": "default", "Version": "1.11.1", "Status": "ready", "SchedulingEligibility": "eligible", "Drain": false, "Address": "10.0.0.5"}]`)

	nodes, err := client.Nodes(context.Background())
	r.NoError(err)
	r.Len(nodes, 1)

	r.Equal("nomad-server-01", nodes[0].Name)
	r.Equal("dc1", nodes[0].Datacenter)
	r.Equal("ready", nodes[0].Status)
	r.Equal("eligible", nodes[0].Eligibility)
	r.Equal("10.0.0.5", nodes[0].Address)
}

func TestNodes_ReadTheCapacity(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `[{
		"ID": "node-1",
		"Name": "nomad-server-01",
		"Status": "ready",
		"NodeResources": {"Cpu": {"CpuShares": 4000}, "Memory": {"MemoryMB": 3820}}
	}]`)

	nodes, err := client.Nodes(context.Background())
	r.NoError(err)
	r.Len(nodes, 1)

	// Nomad leaves the resources out of the list unless it is asked for them,
	// and what a machine has is what its readings are measured against.
	r.Equal("true", asked.URL.Query().Get("resources"))
	r.Equal(4000, nodes[0].CPUShares)
	r.Equal(3820, nodes[0].MemoryMB)
}

func TestVariables_Read(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `[{"Path": "nomad/jobs/web", "Namespace": "production", "CreateTime": 1758499200000000000, "ModifyTime": 1758585600000000000}]`)

	variables, err := client.Variables(context.Background(), "production")
	r.NoError(err)
	r.Len(variables, 1)

	r.Equal("nomad/jobs/web", variables[0].Path)
	r.False(variables[0].Created.IsZero())
	r.False(variables[0].Modified.IsZero())
}

func TestNodePools_Read(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `[{"Name": "default", "Description": "the default pool", "SchedulerConfiguration": {"SchedulerAlgorithm": "binpack"}}]`)

	pools, err := client.NodePools(context.Background())
	r.NoError(err)
	r.Len(pools, 1)

	r.Equal("default", pools[0].Name)
	r.Equal("the default pool", pools[0].Description)
	r.Equal("binpack", pools[0].Scheduler)
}

func TestTaskGroups_Read(t *testing.T) {
	r := require.New(t)

	// The job says what it asks for, the summary says what is running. Both
	// come from the same answer here.
	client, asked := recorder(t, `{
		"ID": "web",
		"TaskGroups": [{"Name": "frontend", "Count": 3}, {"Name": "backend", "Count": 1}],
		"Summary": {"frontend": {"Running": 2, "Starting": 1}, "backend": {"Running": 1}}
	}`)

	groups, err := client.TaskGroups(context.Background(), "production", "web")
	r.NoError(err)
	r.Len(groups, 2)

	r.Equal("/v1/job/web/summary", asked.URL.Path)
	r.Equal("production", asked.URL.Query().Get("namespace"))

	r.Equal("frontend", groups[0].Name)
	r.Equal("web", groups[0].JobID)
	r.Equal(3, groups[0].Count)
	r.Equal(2, groups[0].Running)
	r.Equal(1, groups[0].Starting)

	r.Equal("backend", groups[1].Name)
	r.Equal(1, groups[1].Count)
}
