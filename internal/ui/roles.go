package ui

import (
	"context"
	"time"

	"github.com/ingvarch/urga/internal/nomad"
)

// Client is what the screens need from the cluster. Every call names the
// namespace it asks in. It combines the roles below: a function takes the
// role it uses rather than the whole client.
type Client interface {
	clusterClient
	eventsClient
	jobsClient
	allocsClient
	filesClient
	deploymentsClient
	nodesClient
	namespacesClient
	servicesClient
	evaluationsClient
	variablesClient
	serversClient
	volumesClient
	searchClient
}

// The roles the cluster plays for the screens, one per kind of resource.
type (
	// clusterClient is the cluster as a whole: where it is, what it runs,
	// whose token it is sent, and how busy it is.
	clusterClient interface {
		Address() string
		Region() string
		Agent(ctx context.Context) (nomad.Agent, error)
		Regions(ctx context.Context) ([]string, error)
		Datacenters(ctx context.Context) ([]string, error)
		Token(ctx context.Context) (nomad.Token, error)
		Usage(ctx context.Context, datacenter string) (nomad.Usage, error)
	}

	// eventsClient opens the event stream that signals a screen is stale.
	eventsClient interface {
		Events(ctx context.Context, namespace string, topics []string) (*nomad.Changes, error)
	}

	jobsClient interface {
		Jobs(ctx context.Context, namespace string) ([]nomad.Job, error)
		DescribeJob(ctx context.Context, namespace, jobID string) (string, error)
		JobSpec(ctx context.Context, namespace, jobID string) (nomad.JobSource, error)
		TaskGroups(ctx context.Context, namespace, jobID string) ([]nomad.TaskGroup, error)
		PlanJob(ctx context.Context, namespace, source string, vars nomad.JobVariables) (nomad.Plan, error)
		PlanRevert(ctx context.Context, namespace, jobID string, to *uint64) (nomad.Plan, error)
		SubmitJob(ctx context.Context, namespace, source string, vars nomad.JobVariables, index uint64) error
		StartJob(ctx context.Context, namespace, jobID string) error
		StopJob(ctx context.Context, namespace, jobID string) error
		LaunchJob(ctx context.Context, namespace, jobID string) error
		DispatchForm(ctx context.Context, namespace, jobID string) (nomad.DispatchForm, error)
		DispatchJob(ctx context.Context, namespace, jobID string, meta map[string]string, payload []byte) (string, error)
		NextLaunch(ctx context.Context, namespace, jobID string) (time.Time, error)
		JobVersions(ctx context.Context, namespace, jobID string) ([]nomad.JobVersion, error)
		JobVersionDiff(ctx context.Context, namespace, jobID string, version uint64) ([]nomad.DiffLine, error)
		RevertJobTo(ctx context.Context, namespace, jobID string, version, from uint64) error
		ScaleJob(ctx context.Context, namespace, jobID, group string, count int) error
	}

	// allocsClient is the allocations, of a job, a client or a deployment,
	// and the tasks they run.
	allocsClient interface {
		Allocations(ctx context.Context, namespace, jobID string) ([]nomad.Alloc, error)
		NodeAllocations(ctx context.Context, nodeID string) ([]nomad.Alloc, error)
		DeploymentAllocations(ctx context.Context, namespace, deploymentID string) ([]nomad.Alloc, error)
		Allocation(ctx context.Context, namespace, allocID string) (nomad.Alloc, error)
		DescribeAllocation(ctx context.Context, namespace, allocID string) (string, error)
		AllocationUsage(ctx context.Context, namespace, allocID string) (nomad.ResourceUse, error)
		AllocationChecks(ctx context.Context, namespace, allocID string) ([]nomad.Check, error)
		RestartAllocation(ctx context.Context, namespace, allocID string) error
		RestartTask(ctx context.Context, namespace, allocID, task string) error
		SignalTask(ctx context.Context, namespace, allocID, task, signal string) error
		StopAllocation(ctx context.Context, namespace, allocID string) error
	}

	// filesClient is what a task writes and keeps: its logs and the files
	// of its allocation.
	filesClient interface {
		Logs(ctx context.Context, namespace, allocID, task, source string) (*nomad.LogStream, error)
		Files(ctx context.Context, namespace, allocID, path string) ([]nomad.File, error)
		File(ctx context.Context, namespace, allocID, path string) (*nomad.LogStream, error)
	}

	deploymentsClient interface {
		Deployments(ctx context.Context, namespace string) ([]nomad.Deployment, error)
		Deployment(ctx context.Context, namespace, deploymentID string) (nomad.DeploymentDetail, error)
		DescribeDeployment(ctx context.Context, namespace, deploymentID string) (string, error)
		PromoteDeployment(ctx context.Context, namespace, deploymentID string) error
		PromoteGroups(ctx context.Context, namespace, deploymentID string, groups []string) error
		PauseDeployment(ctx context.Context, namespace, deploymentID string, pause bool) error
		FailDeployment(ctx context.Context, namespace, deploymentID string) error
	}

	// nodesClient is the clients of the cluster, the machines, and the pools
	// they are grouped in.
	nodesClient interface {
		Nodes(ctx context.Context) ([]nomad.Node, error)
		Node(ctx context.Context, nodeID string) (nomad.Node, error)
		NodeDetail(ctx context.Context, nodeID string) (nomad.NodeDetail, error)
		NodeMeta(ctx context.Context, nodeID string) ([]nomad.MetaEntry, error)
		NodeMetaSpec(ctx context.Context, nodeID string) (string, error)
		SubmitNodeMeta(ctx context.Context, nodeID, source string) error
		NodeUsage(ctx context.Context, nodeID string) (nomad.ResourceUse, error)
		DrainNode(ctx context.Context, nodeID string, drain bool) error
		SetNodeEligible(ctx context.Context, nodeID string, eligible bool) error
		NodePools(ctx context.Context) ([]nomad.NodePool, error)
	}

	namespacesClient interface {
		Namespaces(ctx context.Context) ([]nomad.Namespace, error)
		NamespaceSpec(ctx context.Context, name string) (string, error)
		SubmitNamespace(ctx context.Context, source string) error
	}

	servicesClient interface {
		Services(ctx context.Context, namespace string) ([]nomad.Service, error)
		DescribeService(ctx context.Context, namespace, name string) (string, error)
		ServiceInstances(ctx context.Context, namespace, name string) ([]nomad.ServiceInstance, error)
		DeleteServiceRegistration(ctx context.Context, namespace, name, id string) error
	}

	// evaluationsClient is the evaluations, and why a job was not placed.
	evaluationsClient interface {
		Evaluations(ctx context.Context, namespace string) ([]nomad.Evaluation, error)
		Evaluation(ctx context.Context, namespace, evalID string) (nomad.EvaluationDetail, error)
		FailedPlacement(ctx context.Context, namespace, jobID string) (nomad.EvaluationDetail, error)
	}

	variablesClient interface {
		Variables(ctx context.Context, namespace string) ([]nomad.Variable, error)
		Variable(ctx context.Context, namespace, path string) (nomad.VariableDetail, error)
		VariableSpec(ctx context.Context, namespace, path string) (nomad.VariableSource, error)
		SubmitVariable(ctx context.Context, namespace, path, source string, index uint64) error
	}

	serversClient interface {
		Servers(ctx context.Context) ([]nomad.Server, error)
		Server(ctx context.Context, name string) (nomad.Server, error)
		RaftPeers(ctx context.Context) ([]nomad.RaftPeer, error)
	}

	// volumesClient is the storage of the cluster: CSI and host volumes,
	// and the CSI plugins that provide them.
	volumesClient interface {
		Volumes(ctx context.Context, namespace string) ([]nomad.Volume, error)
		Volume(ctx context.Context, namespace, kind, id string) (nomad.VolumeDetail, error)
		DescribeVolume(ctx context.Context, namespace, kind, id string) (string, error)
		Plugins(ctx context.Context) ([]nomad.Plugin, error)
		Plugin(ctx context.Context, id string) (nomad.PluginDetail, error)
		DescribePlugin(ctx context.Context, id string) (string, error)
		DetachVolume(ctx context.Context, namespace, volumeID, nodeID string) error
		ReleaseClaim(ctx context.Context, namespace, claimID string) error
	}

	// searchClient finds anything in the cluster by its name or its ID.
	searchClient interface {
		Find(ctx context.Context, text string) (nomad.Found, error)
	}
)
