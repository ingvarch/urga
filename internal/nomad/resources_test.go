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
	ctx := context.Background()

	tests := []struct {
		name      string
		path      string
		namespace string
		call      func(*nomad.Client) error
	}{
		{
			name: "deployments", path: "/v1/deployments", namespace: "production",
			call: func(c *nomad.Client) error { return errOf(c.Deployments(ctx, "production")) },
		},
		{
			name: "namespaces", path: "/v1/namespaces",
			call: func(c *nomad.Client) error { return errOf(c.Namespaces(ctx)) },
		},
		{
			name: "services", path: "/v1/services", namespace: "*",
			call: func(c *nomad.Client) error { return errOf(c.Services(ctx, nomad.AllNamespaces)) },
		},
		{
			name: "evaluations", path: "/v1/evaluations", namespace: "production",
			call: func(c *nomad.Client) error { return errOf(c.Evaluations(ctx, "production")) },
		},
		{
			name: "nodes", path: "/v1/nodes",
			call: func(c *nomad.Client) error { return errOf(c.Nodes(ctx)) },
		},
		{
			name: "variables", path: "/v1/vars", namespace: "production",
			call: func(c *nomad.Client) error { return errOf(c.Variables(ctx, "production")) },
		},
		{
			name: "node pools", path: "/v1/node/pools",
			call: func(c *nomad.Client) error { return errOf(c.NodePools(ctx)) },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := require.New(t)

			client, asked := recorder(t, `[]`)

			r.NoError(test.call(client))
			r.Equal(test.path, asked.URL.Path)
			r.Equal(test.namespace, asked.URL.Query().Get("namespace"))
		})
	}
}

// errOf drops the result of a call and returns its error: the test checks
// only the request.
func errOf[T any](_ T, err error) error { return err }

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
	// and the usage of a client is shown against them.
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
	r.Nil(variables[0].Lock)
}

func TestVariables_SayWhoHoldsTheLock(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `[{"Path": "locks/leader", "Namespace": "default",
		"Lock": {"ID": "874ae5d0-f47a-afb0-804f-fcdb04a14a0b", "TTL": "30m0s", "LockDelay": "15s"}}]`)

	variables, err := client.Variables(context.Background(), "default")
	r.NoError(err)
	r.Len(variables, 1)

	r.Equal(&nomad.VariableLock{ID: "874ae5d0-f47a-afb0-804f-fcdb04a14a0b", TTL: "30m0s", Delay: "15s"}, variables[0].Lock)
}

func TestVariable_ReadsItsValues(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{"Path": "nomad/jobs/web", "Namespace": "production", "ModifyIndex": 769,
		"Items": {"DB_HOST": "10.0.0.5", "CERT": "line1\nline2\n"}}`)

	variable, err := client.Variable(context.Background(), "production", "nomad/jobs/web")
	r.NoError(err)

	r.Equal("/v1/var/nomad/jobs/web", asked.URL.Path)
	r.Equal("production", asked.URL.Query().Get("namespace"))
	r.Equal("nomad/jobs/web", variable.Path)
	r.Equal(map[string]string{"DB_HOST": "10.0.0.5", "CERT": "line1\nline2\n"}, variable.Items)
	r.Nil(variable.Lock)
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
