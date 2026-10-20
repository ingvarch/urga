package nomad_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestAlloc_HoldsPlace(t *testing.T) {
	for _, tc := range []struct {
		desired, status string
		holds           bool
	}{
		{"run", "running", true},
		{"run", "pending", true},
		{"run", "unknown", true},
		{"run", "complete", false},
		{"run", "failed", false},
		{"run", "lost", false},
		{"stop", "running", false},
		{"evict", "running", false},
	} {
		alloc := nomad.Alloc{DesiredStatus: tc.desired, Status: tc.status}

		require.Equal(t, tc.holds, alloc.HoldsPlace(), "%s %s", tc.desired, tc.status)
	}
}

func TestNode_ReadsTheAllocationLimit(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `{"ID": "node-1", "NodeMaxAllocs": 10}`)

	node, err := client.Node(context.Background(), "node-1")
	r.NoError(err)
	r.Equal(10, node.MaxAllocs)
}

// limitsCluster answers the three requests WithAllocLimits makes, and keeps
// what was asked.
type limitsCluster struct {
	// limits is NodeMaxAllocs by client id; a client not in it has none.
	limits map[string]int

	// listStatus and gone make the filtered list and one client answer with
	// an error.
	listStatus int
	gone       string

	// infoStatus makes the read of every client answer with an error.
	infoStatus int

	// allocsStatus makes the allocations list answer with an error.
	allocsStatus int

	// together holds the reads of the clients until that many are asked at
	// once, or until readsTimeout has passed and stalled is set.
	together int
	opened   chan struct{}
	open     sync.Once
	stalled  bool

	// inFlight are the reads of clients asked and not answered yet, and
	// most the most of them at once.
	inFlight, most int

	allocs string

	// mu guards asked and the counts of reads: the client reads several
	// clients at once.
	mu    sync.Mutex
	asked []url.URL
}

// readsTimeout is how long the reads of the clients wait to be asked
// together before the test fails. A cluster that reads them one at a time
// never gets there.
const readsTimeout = 10 * time.Second

// heldOpen is how long the reads stay held once together of them are in
// flight, so that one more sent with them is counted too. It cannot fail a
// client that keeps to the limit.
const heldOpen = 200 * time.Millisecond

// read counts a read of a client in flight and holds it until together of
// them are, and for heldOpen after that.
func (l *limitsCluster) read() {
	l.mu.Lock()
	l.inFlight++
	l.most = max(l.most, l.inFlight)
	all := l.inFlight == l.together
	l.mu.Unlock()

	switch {
	case l.together == 0:
		l.open.Do(func() { close(l.opened) })
	case all:
		time.AfterFunc(heldOpen, func() { l.open.Do(func() { close(l.opened) }) })
	}

	select {
	case <-l.opened:
	case <-time.After(readsTimeout):
		l.mu.Lock()
		l.stalled = true
		l.mu.Unlock()
		l.open.Do(func() { close(l.opened) })
	}

	// The read is over before its answer is sent: the next one cannot be
	// asked before then.
	l.mu.Lock()
	l.inFlight--
	l.mu.Unlock()
}

// reads are the most reads of clients in flight at once, and whether they
// waited too long for that many.
func (l *limitsCluster) reads() (most int, stalled bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.most, l.stalled
}

func (l *limitsCluster) client(t *testing.T) *nomad.Client {
	t.Helper()

	l.opened = make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		l.mu.Lock()
		l.asked = append(l.asked, *req.URL)
		l.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		switch path := req.URL.Path; {
		case path == "/v1/nodes":
			if l.listStatus != 0 {
				http.Error(w, "bad filter", l.listStatus)

				return
			}

			stubs := []string{}
			for id := range l.limits {
				stubs = append(stubs, fmt.Sprintf(`{"ID": %q}`, id))
			}

			_, _ = w.Write([]byte("[" + strings.Join(stubs, ",") + "]"))
		case strings.HasPrefix(path, "/v1/node/"):
			id := strings.TrimPrefix(path, "/v1/node/")
			l.read()

			if id == l.gone {
				http.Error(w, "node not found", http.StatusNotFound)

				return
			}

			if l.infoStatus != 0 {
				http.Error(w, "rpc error", l.infoStatus)

				return
			}

			_, _ = fmt.Fprintf(w, `{"ID": %q, "NodeMaxAllocs": %d}`, id, l.limits[id])
		case path == "/v1/allocations":
			if l.allocsStatus != 0 {
				http.Error(w, "permission denied", l.allocsStatus)

				return
			}

			_, _ = w.Write([]byte(l.allocs))
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	require.NoError(t, err)

	return client
}

func (l *limitsCluster) requests() []url.URL {
	l.mu.Lock()
	defer l.mu.Unlock()

	return slices.Clone(l.asked)
}

func (l *limitsCluster) askedPaths() []string {
	asked := l.requests()

	paths := make([]string, 0, len(asked))
	for _, u := range asked {
		paths = append(paths, u.Path)
	}

	return paths
}

const limitAllocs = `[
	{"ID": "a1", "NodeID": "node-1", "DesiredStatus": "run", "ClientStatus": "running"},
	{"ID": "a2", "NodeID": "node-1", "DesiredStatus": "run", "ClientStatus": "pending"},
	{"ID": "a3", "NodeID": "node-1", "DesiredStatus": "run", "ClientStatus": "complete"},
	{"ID": "a4", "NodeID": "node-1", "DesiredStatus": "stop", "ClientStatus": "running"},
	{"ID": "a5", "NodeID": "node-2", "DesiredStatus": "run", "ClientStatus": "running"}
]`

func TestWithAllocLimits_CountsWhatHoldsAPlace(t *testing.T) {
	r := require.New(t)

	cluster := &limitsCluster{limits: map[string]int{"node-1": 2}, allocs: limitAllocs}
	client := cluster.client(t)

	nodes := []nomad.Node{{ID: "node-1"}, {ID: "node-2"}}

	got, err := client.WithAllocLimits(context.Background(), nodes)
	r.NoError(err)

	// The limited client has its limit and the allocations that hold a place;
	// the complete one and the one that is being stopped do not count.
	r.Equal([]nomad.Node{{ID: "node-1", MaxAllocs: 2, Allocs: 2}, {ID: "node-2"}}, got)

	// The nodes it was given are not changed.
	r.Zero(nodes[0].MaxAllocs)
}

func TestWithAllocLimits_AsksForTheLimitedClientsOnly(t *testing.T) {
	r := require.New(t)

	cluster := &limitsCluster{limits: map[string]int{"node-1": 2}, allocs: limitAllocs}
	client := cluster.client(t)

	_, err := client.WithAllocLimits(context.Background(), []nomad.Node{{ID: "node-1"}, {ID: "node-2"}})
	r.NoError(err)

	r.Equal([]string{"/v1/nodes", "/v1/node/node-1", "/v1/allocations"}, cluster.askedPaths())

	asked := cluster.requests()
	r.Equal("NodeMaxAllocs != 0", asked[0].Query().Get("filter"))
	r.Equal("*", asked[2].Query().Get("namespace"))
	r.Equal(`NodeID == "node-1"`, asked[2].Query().Get("filter"))
}

func TestWithAllocLimits_FiltersOnEveryLimitedClient(t *testing.T) {
	r := require.New(t)

	cluster := &limitsCluster{limits: map[string]int{"node-1": 2}, allocs: `[]`}
	client := cluster.client(t)

	// A cluster that answers for node-2 as well, with a limit of its own.
	cluster.limits["node-2"] = 5

	got, err := client.WithAllocLimits(context.Background(), []nomad.Node{{ID: "node-1"}, {ID: "node-2"}})
	r.NoError(err)
	r.Equal(5, got[1].MaxAllocs)

	asked := cluster.requests()
	r.Equal(`NodeID == "node-1" or NodeID == "node-2"`, asked[len(asked)-1].Query().Get("filter"))
}

func TestWithAllocLimits_NoLimitedClientAsksForNothingMore(t *testing.T) {
	r := require.New(t)

	cluster := &limitsCluster{limits: map[string]int{}}
	client := cluster.client(t)

	nodes := []nomad.Node{{ID: "node-1"}}

	got, err := client.WithAllocLimits(context.Background(), nodes)
	r.NoError(err)
	r.Equal(nodes, got)
	r.Equal([]string{"/v1/nodes"}, cluster.askedPaths())
}

func TestWithAllocLimits_AClusterThatIgnoresTheFilterMarksNothing(t *testing.T) {
	r := require.New(t)

	// An older cluster lists every client, and each read shows no limit.
	cluster := &limitsCluster{limits: map[string]int{"node-1": 0}}
	client := cluster.client(t)

	nodes := []nomad.Node{{ID: "node-1"}}

	got, err := client.WithAllocLimits(context.Background(), nodes)
	r.NoError(err)
	r.Equal(nodes, got)
	r.Equal([]string{"/v1/nodes", "/v1/node/node-1"}, cluster.askedPaths())
}

func TestWithAllocLimits_AClusterThatRefusesTheFilterLeavesTheNodes(t *testing.T) {
	r := require.New(t)

	cluster := &limitsCluster{listStatus: http.StatusBadRequest}
	client := cluster.client(t)

	nodes := []nomad.Node{{ID: "node-1"}}

	got, err := client.WithAllocLimits(context.Background(), nodes)
	r.NoError(err)
	r.Equal(nodes, got)
}

func TestWithAllocLimits_OtherErrorsAreReturned(t *testing.T) {
	limits := map[string]int{"node-1": 2}

	for name, cluster := range map[string]*limitsCluster{
		"the filtered list": {listStatus: http.StatusInternalServerError},
		"a client":          {limits: limits, infoStatus: http.StatusInternalServerError},
		"the allocations":   {limits: limits, allocsStatus: http.StatusInternalServerError},
	} {
		_, err := cluster.client(t).WithAllocLimits(context.Background(), []nomad.Node{{ID: "node-1"}})
		require.Error(t, err, name)
	}
}

func TestWithAllocLimits_ReadsOnlyTheClientsItWasGiven(t *testing.T) {
	r := require.New(t)

	// node-9 is of another pool: the filtered list names it, the list it
	// was given does not.
	cluster := &limitsCluster{limits: map[string]int{"node-1": 2, "node-9": 5}, allocs: limitAllocs}

	got, err := cluster.client(t).WithAllocLimits(context.Background(), []nomad.Node{{ID: "node-1"}, {ID: "node-2"}})
	r.NoError(err)
	r.Equal([]nomad.Node{{ID: "node-1", MaxAllocs: 2, Allocs: 2}, {ID: "node-2"}}, got)

	r.NotContains(cluster.askedPaths(), "/v1/node/node-9")

	asked := cluster.requests()
	r.Equal(`NodeID == "node-1"`, asked[len(asked)-1].Query().Get("filter"))
}

func TestWithAllocLimits_AClientThatIsGoneHasNoLimit(t *testing.T) {
	r := require.New(t)

	cluster := &limitsCluster{
		limits: map[string]int{"node-1": 2, "node-2": 3},
		gone:   "node-2",
		allocs: limitAllocs,
	}
	client := cluster.client(t)

	got, err := client.WithAllocLimits(context.Background(), []nomad.Node{{ID: "node-1"}, {ID: "node-2"}})
	r.NoError(err)
	r.Equal([]nomad.Node{{ID: "node-1", MaxAllocs: 2, Allocs: 2}, {ID: "node-2"}}, got)
}

func TestWithAllocLimits_ATokenThatMayNotListAllocationsLeavesTheNodes(t *testing.T) {
	r := require.New(t)

	cluster := &limitsCluster{limits: map[string]int{"node-1": 2}, allocsStatus: http.StatusForbidden}
	client := cluster.client(t)

	nodes := []nomad.Node{{ID: "node-1"}}

	got, err := client.WithAllocLimits(context.Background(), nodes)
	r.NoError(err)
	r.Equal(nodes, got)
}

func TestWithAllocLimits_ManyLimitedClientsAreReadTogether(t *testing.T) {
	r := require.New(t)

	limits := map[string]int{}
	nodes := []nomad.Node{}

	for i := range 40 {
		id := fmt.Sprintf("node-%02d", i)
		limits[id] = 2
		nodes = append(nodes, nomad.Node{ID: id})
	}

	// Eight is how many clients are read at once.
	cluster := &limitsCluster{limits: limits, allocs: `[]`, together: 8}
	client := cluster.client(t)

	got, err := client.WithAllocLimits(context.Background(), nodes)
	r.NoError(err)

	for _, node := range got {
		r.Equal(2, node.MaxAllocs, node.ID)
	}

	most, stalled := cluster.reads()
	r.False(stalled, "fewer than 8 clients were read at once")
	r.Equal(8, most)
}
