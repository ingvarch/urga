package ui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestRegion_TheAgentSaysWhichOneIsInUse(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{})
	r.Contains(headerOf(m), "Region:    n/a")

	// A session that names no region is answered in the one of the agent.
	m, _ = m.update(agentMsg(nomad.Agent{Version: "1.11.1", Region: "global"}))

	r.Contains(headerOf(m), "Region:    global")
	r.Contains(headerOf(m), "DC:        all")
}

func TestRegion_TheOneAskedForWins(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{region: "eu"})
	m, _ = m.update(agentMsg(nomad.Agent{Region: "global"}))

	// The agent answers for its own region, the session asks in another.
	r.Contains(headerOf(m), "Region:    eu")
}

func TestRegion_TheSessionKnowsTheRegionsAndTheDatacenters(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{regions: []string{"eu", "us"}, datacenters: []string{"dc1", "dc2"}}
	m := newTestModel(client)

	m = drain(m, m.Init())

	// The command line checks a name against these.
	r.Equal([]string{"eu", "us"}, m.regions)
	r.Equal([]string{"dc1", "dc2"}, m.datacenters)
}

func TestRegion_DatacentersOfAnotherRegionAreDropped(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{region: "eu"})

	// Asked before the session moved to eu: the names belong to us.
	m, _ = m.update(datacentersMsg{region: "us", names: []string{"us-east"}})

	r.Empty(m.datacenters)
}

func TestDatacenter_NarrowsTheJobs(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{})
	m.datacenter = "dc1"

	m, _ = m.update(jobsMsg([]nomad.Job{
		{ID: "web", Datacenters: []string{"dc1"}},
		{ID: "api", Datacenters: []string{"dc2"}},
		{ID: "agent", Datacenters: []string{"*"}},
	}))

	// A job that may be placed in the datacenter stays, star or not.
	r.Equal([]string{"web", "agent"}, idsOf(m.jobs, func(j nomad.Job) string { return j.ID }))
	r.NotContains(plain(m.render()), "api")
}

func TestDatacenter_NarrowsTheClients(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{})
	m.datacenter = "dc1"
	m, _ = m.show(screenNodes)

	m, _ = m.update(nodesMsg([]nomad.Node{
		{ID: "n1", Name: "node-01", Datacenter: "dc1"},
		{ID: "n2", Name: "node-02", Datacenter: "dc2"},
	}))

	r.Equal([]string{"n1"}, idsOf(m.nodes, func(n nomad.Node) string { return n.ID }))
}

func TestDatacenter_NarrowsTheServers(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{region: "eu"})
	m.datacenter = "dc1"
	m, _ = m.show(screenServers)

	// The cluster answers with the servers of the region in use.
	m, _ = m.update(serversMsg([]nomad.Server{
		{Name: "eu-1", Region: "eu", Datacenter: "dc1"},
		{Name: "eu-2", Region: "eu", Datacenter: "dc2"},
	}))

	r.Equal([]string{"eu-1"}, idsOf(m.servers, func(s nomad.Server) string { return s.Name }))
}

func TestDatacenter_EveryOneOfThem(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{})
	m, _ = m.show(screenServers)

	m, _ = m.update(serversMsg([]nomad.Server{
		{Name: "eu-1", Region: "eu", Datacenter: "dc1"},
		{Name: "us-1", Region: "us", Datacenter: "dc2"},
	}))

	// No datacenter chosen: nothing is left out.
	r.Len(m.servers, 2)
}

// idsOf names what a list holds, in its order.
func idsOf[T any](items []T, id func(T) string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, id(item))
	}

	return out
}

func TestUsage_IsReadForTheDatacenterInUse(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{region: "eu", usage: nomad.Usage{CPUPercent: 15, MemoryPercent: 31}}
	m := newTestModel(client)
	m.datacenter = "dc1"

	_, cmd := m.update(pollUsageMsg{})
	r.NotNil(cmd)

	m, _ = m.update(cmd())

	// The numbers under the name of the datacenter are about it.
	r.Equal("dc1", client.usageDatacenter)
	r.Contains(headerOf(m), "CPU:       15%")
}

func TestUsage_AReadingOfAnotherPlaceIsDropped(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{region: "eu"})
	m.datacenter = "dc1"

	// Read before the session moved on.
	m, _ = m.update(usageMsg{region: "eu", datacenter: "dc2", usage: nomad.Usage{CPUPercent: 50}})
	m, _ = m.update(usageMsg{region: "us", datacenter: "dc1", usage: nomad.Usage{CPUPercent: 60}})

	r.Contains(headerOf(m), "CPU:       n/a")
}

func TestUsage_KeepsOneTimer(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{})

	m, first := m.update(usageMsg{})
	r.NotNil(first, "the next reading is due")

	// A reading asked out of turn, after a switch, lands while the timer is
	// still on its way; a second timer would double the asking for good.
	_, second := m.update(usageMsg{})
	r.Nil(second)
}

// regionalModel is a session that can switch regions: the fake takes on the
// region it is asked in, and knows two of them.
func regionalModel(t *testing.T, client *fakeClient) Model {
	t.Helper()

	m := newTestModel(client)
	m.opts.InRegion = func(region string) Client {
		client.region = region

		return client
	}

	m, _ = m.update(regionsMsg([]string{"eu", "us"}))

	return m
}

// runLine types a line into the command line and runs it.
func runLine(m Model, line string) (Model, tea.Cmd) {
	m, _ = m.update(key(':'))
	m = typeIn(m, line)

	return m.update(enter())
}

func TestRegionCommand_SwitchesTheSession(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), datacenters: []string{"us-east"}}
	m := regionalModel(t, client)
	m.datacenter = "dc1"

	m, cmd := runLine(m, "region us")

	r.Equal("us", m.client.Region())
	r.Contains(headerOf(m), "Region:    us")

	// Datacenters are named per region: dc1 of eu means nothing in us.
	r.Contains(headerOf(m), "DC:        all")

	client.calls = 0
	m = drain(m, cmd)

	// What is on the screen and what the next region holds are asked again.
	r.Equal(1, client.calls)
	r.Equal([]string{"us-east"}, m.datacenters)
}

func TestRegionCommand_GoesBackToTheList(t *testing.T) {
	r := require.New(t)

	m := regionalModel(t, &fakeClient{jobs: twoJobs(), allocs: twoAllocs()})
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(enter())
	r.Equal(screenAllocations, m.screen.kind)

	m, _ = runLine(m, "region us")

	// The allocations of a job in one region say nothing about another.
	r.Equal(screenJobs, m.screen.kind)
	r.Empty(m.history)
}

func TestRegionCommand_ClosesTheLogs(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), logs: &nomad.LogStream{Lines: make(chan string)}}
	m := openTasks(t, client)

	m, cmd := m.update(enter())
	m = drain(m, cmd)
	r.Equal(screenLogs, m.screen.kind)

	m.opts.InRegion = func(string) Client { return client }
	m, _ = m.update(regionsMsg([]string{"eu", "us"}))

	m, _ = runLine(m, "region us")

	r.True(client.logsClosed)
	r.Equal(screenJobs, m.screen.kind)
}

func TestRegionCommand_ClosesTheLogsOfEveryAllocation(t *testing.T) {
	r := require.New(t)

	m, client := oneTask(t)
	r.Equal(screenJobLogs, m.screen.kind)

	m.opts.InRegion = func(string) Client { return client }
	m, _ = m.update(regionsMsg([]string{"eu", "us"}))

	closed := countClosed(client.logsByAlloc)
	m, _ = runLine(m, "region us")

	// Each one is a request held open to a client of the region left.
	r.Equal(2, *closed)
	r.Equal(screenJobs, m.screen.kind)
}

func TestRegionCommand_ReadsTheUsageAtOnce(t *testing.T) {
	r := require.New(t)

	m := regionalModel(t, &fakeClient{usage: nomad.Usage{CPUPercent: 15, MemoryPercent: 31}})

	m, cmd := runLine(m, "region us")
	m = drain(m, cmd)

	r.Contains(headerOf(m), "CPU:       15%")
}

func TestRegionCommand_ARegionTheClusterDoesNotKnow(t *testing.T) {
	r := require.New(t)

	m := regionalModel(t, &fakeClient{})

	m, _ = runLine(m, "region mars")

	r.Contains(plain(m.render()), "no such region: mars (eu, us)")
	r.Empty(m.client.Region())
}

func TestRegionCommand_OpensTheRegions(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{region: "eu", jobs: twoJobs(), regions: []string{"eu", "us"}}
	m := regionalModel(t, client)

	m, cmd := runLine(m, "region")
	m = drain(m, cmd)

	// Every region on a list to pick from, the one in use marked.
	r.Equal(screenRegions, m.screen.kind)

	out := plain(m.render())
	r.Contains(out, "Regions [2]")
	r.Regexp(`eu\s+in use`, out)
	r.Contains(out, "us")
}

func TestRegions_EnterSwitchesToTheRegion(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{region: "eu", jobs: twoJobs(), regions: []string{"eu", "us"}}
	m := regionalModel(t, client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, cmd := runLine(m, "region")
	m = drain(m, cmd)

	m, _ = m.update(down())
	m, _ = m.update(enter())

	// The list it was opened from comes back, asked in the new region.
	r.Equal("us", m.client.Region())
	r.Equal(screenJobs, m.screen.kind)
	r.Empty(m.history)
}

func TestRegions_TheOneInUseGoesBack(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{region: "eu", jobs: twoJobs(), regions: []string{"eu", "us"}}
	m := regionalModel(t, client)

	m, cmd := runLine(m, "region")
	m = drain(m, cmd)

	m, _ = m.update(enter())

	// Nothing to switch, and nothing to stay on the list for either.
	r.Equal("eu", m.client.Region())
	r.Equal(screenJobs, m.screen.kind)
}

func TestRegions_EscapeKeepsTheRegion(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{region: "eu", regions: []string{"eu", "us"}}
	m := regionalModel(t, client)

	m, cmd := runLine(m, "region")
	m = drain(m, cmd)

	m, _ = m.update(down())
	m, _ = m.update(escape())

	r.Equal("eu", m.client.Region())
	r.Equal(screenJobs, m.screen.kind)
}

func TestRegionCommand_WithoutAWayToSwitch(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{})
	m, _ = m.update(regionsMsg([]string{"eu", "us"}))

	m, _ = runLine(m, "region us")

	r.Contains(plain(m.render()), "this session cannot switch regions")
}

func TestDatacenterCommand_NarrowsTheSession(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: []nomad.Job{
		{ID: "web", Datacenters: []string{"dc1"}},
		{ID: "api", Datacenters: []string{"dc2"}},
	}}
	m := newTestModel(client)
	m, _ = m.update(datacentersMsg{names: []string{"dc1", "dc2"}})
	m, _ = m.update(jobsMsg(client.jobs))

	m, cmd := runLine(m, "dc dc2")
	r.Contains(headerOf(m), "DC:        dc2")

	m = drain(m, cmd)

	// The lists and the numbers of the header are about that datacenter.
	r.Equal([]string{"api"}, idsOf(m.jobs, func(j nomad.Job) string { return j.ID }))
	r.Equal("dc2", client.usageDatacenter)

	// And all of them again.
	m, cmd = runLine(m, "dc all")
	m = drain(m, cmd)

	r.Contains(headerOf(m), "DC:        all")
	r.Len(m.jobs, 2)
}

func TestDatacenterCommand_ADatacenterTheRegionDoesNotHave(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{})
	m, _ = m.update(datacentersMsg{names: []string{"dc1", "dc2"}})

	m, _ = runLine(m, "dc dc9")

	r.Contains(plain(m.render()), "no such datacenter: dc9 (dc1, dc2)")
	r.Empty(m.datacenter)
}

func TestDatacenterCommand_OpensTheDatacenters(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{datacenters: []string{"dc1", "dc2"}})

	m, cmd := runLine(m, "dc")
	m = drain(m, cmd)

	r.Equal(screenDatacenters, m.screen.kind)

	// Every one of them is a choice of its own, and the one in use while
	// none was chosen.
	out := plain(m.render())
	r.Contains(out, "Datacenters [3]")
	r.Regexp(`all\s+in use`, out)
	r.Contains(out, "dc1")
	r.Contains(out, "dc2")
}

func TestDatacenters_EnterNarrowsTheScreenItCameFrom(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		jobs: []nomad.Job{
			{ID: "web", Datacenters: []string{"dc1"}},
			{ID: "api", Datacenters: []string{"dc2"}},
		},
		datacenters: []string{"dc1", "dc2"},
	}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(client.jobs))

	m, cmd := runLine(m, "dc")
	m = drain(m, cmd)

	m, _ = m.update(down())
	m, cmd = m.update(enter())

	r.Equal("dc1", m.datacenter)
	r.Equal(screenJobs, m.screen.kind)

	m = drain(m, cmd)

	r.Equal([]string{"web"}, idsOf(m.jobs, func(j nomad.Job) string { return j.ID }))
	r.Equal("dc1", client.usageDatacenter)
}

func TestDatacenters_AllBringsEveryOneBack(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{datacenters: []string{"dc1", "dc2"}})
	m.datacenter = "dc1"

	m, cmd := runLine(m, "dc")
	m = drain(m, cmd)

	m, _ = m.update(enter())

	r.Empty(m.datacenter)
	r.Contains(headerOf(m), "DC:        all")
}

func TestInRegionOf_AsksTheClusterInThatRegion(t *testing.T) {
	r := require.New(t)

	client, err := nomad.New(nomad.Config{Address: "https://nomad.example.com", Region: "us"})
	r.NoError(err)

	eu := InRegionOf(client)("eu")

	r.Equal("eu", eu.Region())
	r.Equal("us", client.Region())
}

func TestRegionCommand_LetsGoOfWhatTheRegionItLeftSaid(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs()}
	m := regionalModel(t, client)
	m, _ = m.update(jobsMsg(twoJobs()))

	// The next region does not answer: a token of one region is refused in
	// another.
	client.err = errors.New("ACL token not found")

	m, cmd := runLine(m, "region us")
	m = drain(m, cmd)

	// The jobs of eu must not stand under the name of us, and keys on them
	// would act in us.
	r.Empty(m.jobs)

	out := plain(m.render())
	r.NotContains(out, "cron")
	r.Contains(out, "ACL token not found")
}

func TestRegionCommand_LetsGoOfEveryAnswerOfTheRegionItLeft(t *testing.T) {
	r := require.New(t)

	m := regionalModel(t, &fakeClient{})

	// What the screens of eu held when they were left.
	m.jobs, m.allocs, m.versions = twoJobs(), twoAllocs(), []nomad.JobVersion{{Version: 3}}
	m.groups = []nomad.TaskGroup{{Name: "web", JobID: "web"}}
	m.deployments = []nomad.Deployment{{ID: "d1"}}
	m.services = []nomad.Service{{Name: "web"}}
	m.evaluations = []nomad.Evaluation{{ID: "e1"}}
	m.nodes = []nomad.Node{{ID: "n1"}}
	m.variables = []nomad.Variable{{Path: "app/db"}}
	m.nodePools = []nomad.NodePool{{Name: "default"}}
	m.servers = []nomad.Server{{Name: "eu-1"}}
	m.host = hostModel{node: nomad.Node{ID: "n1"}, trail: []nomad.ResourceUse{{CPUPercent: 40}}}
	m.nodeDetail = nomad.NodeDetail{ID: "n1", Name: "node-01"}
	m.nodeMeta = []nomad.MetaEntry{{Key: "rack", Value: "r1"}}
	m.server = nomad.Server{Name: "eu-1"}
	m.raft, m.raftErr = []nomad.RaftPeer{{Node: "eu-1", Leader: true}}, errors.New("Permission denied")

	m, _ = runLine(m, "region us")

	// A screen of us opened before us answers must not show eu under its
	// name.
	r.Zero(m.clusterData)
}

func TestRegionCommand_LetsGoOfTheMarks(t *testing.T) {
	r := require.New(t)

	m := regionalModel(t, &fakeClient{jobs: twoJobs()})
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(space())
	r.NotEmpty(m.list.marks)

	m, _ = runLine(m, "region us")

	// A mark names a job of eu; in us the same name is another job.
	r.Empty(m.list.marks)
}

func TestRegionCommand_TheNumbersOfTheRegionLeftAreGone(t *testing.T) {
	r := require.New(t)

	m := regionalModel(t, &fakeClient{})
	m, _ = m.update(usageMsg{usage: nomad.Usage{CPUPercent: 15, MemoryPercent: 31}})
	r.Contains(headerOf(m), "CPU:       15%")

	m, _ = runLine(m, "region us")

	r.Contains(headerOf(m), "CPU:       n/a")
	r.Contains(headerOf(m), "MEM:       n/a")
}

func TestDatacenterCommand_NarrowsWhatIsOnTheScreenAtOnce(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: []nomad.Job{
		{ID: "web", Datacenters: []string{"dc1"}},
		{ID: "api", Datacenters: []string{"dc2"}},
	}}
	m := newTestModel(client)
	m, _ = m.update(datacentersMsg{names: []string{"dc1", "dc2"}})
	m, _ = m.update(jobsMsg(client.jobs))

	// What the other screens held when they were left.
	m.nodes = []nomad.Node{{ID: "n1", Datacenter: "dc1"}, {ID: "n2", Datacenter: "dc2"}}
	m.servers = []nomad.Server{{Name: "s1", Datacenter: "dc1"}, {Name: "s2", Datacenter: "dc2"}}

	client.err = errors.New("connection refused")

	m, cmd := runLine(m, "dc dc2")
	m = drain(m, cmd)

	// Nothing of dc1 stands under the name of dc2, whether or not the
	// cluster answers.
	r.Equal([]string{"api"}, idsOf(m.jobs, func(j nomad.Job) string { return j.ID }))
	r.Equal([]string{"n2"}, idsOf(m.nodes, func(n nomad.Node) string { return n.ID }))
	r.Equal([]string{"s2"}, idsOf(m.servers, func(s nomad.Server) string { return s.Name }))
}

func TestDatacenterCommand_TheNumbersOfTheDatacenterLeftAreGone(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{})
	m, _ = m.update(datacentersMsg{names: []string{"dc1", "dc2"}})
	m, _ = m.update(usageMsg{usage: nomad.Usage{CPUPercent: 15, MemoryPercent: 31}})

	m, _ = runLine(m, "dc dc1")

	r.Contains(headerOf(m), "CPU:       n/a")
	r.Contains(headerOf(m), "MEM:       n/a")
}

func TestRegionState_NarrowsToItsDatacenter(t *testing.T) {
	r := require.New(t)

	s := regionState{datacenters: []string{"dc1", "dc2"}, datacenter: "dc2"}

	r.Equal([]string{"all", "dc1", "dc2"}, s.datacenterChoices())

	jobs := s.jobsInView([]nomad.Job{
		{ID: "web", Datacenters: []string{"dc1"}},
		{ID: "api", Datacenters: []string{"dc2"}},
	})
	r.Equal([]string{"api"}, names(jobs, func(j nomad.Job) string { return j.ID }))

	nodes := s.nodesInView([]nomad.Node{{ID: "n1", Datacenter: "dc1"}, {ID: "n2", Datacenter: "dc2"}})
	r.Equal([]string{"n2"}, names(nodes, nodeMark))

	servers := s.serversInView([]nomad.Server{{Name: "s1", Datacenter: "dc1"}, {Name: "s2", Datacenter: "dc2"}})
	r.Len(servers, 1)
	r.Equal("s2", servers[0].Name)

	// No datacenter chosen is every one of them.
	r.True(regionState{}.inDatacenter("dc1"))
}
