package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestDescribe_OpensForAJob(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), describe: "{\n  \"ID\": \"web\"\n}"}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, cmd := m.update(key('d'))
	r.NotNil(cmd)

	m = drain(m, cmd)

	// What the cluster knows, under a title that says what it is.
	out := plain(m.render())
	r.Contains(out, "Job: web")
	r.Contains(out, "\"ID\": \"web\"")

	// It is asked for in the namespace of that job.
	r.Equal("production", client.askedNamespace)
	r.Equal("web", client.askedID)
}

func TestDescribe_ForAnAllocation(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), describe: "{}"}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(enter())
	m, _ = m.update(allocsMsg(twoAllocs()))

	m, cmd := m.update(key('d'))
	m = drain(m, cmd)

	r.Contains(plain(m.render()), "Allocation: af1f37df")
	r.Equal("af1f37df-7b19-6b1c-da67-5e8f482b5a15", client.askedID)
}

func TestDescribe_JobSpec(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), spec: nomad.JobSource{Source: "job \"web\" {\n  type = \"service\"\n}"}}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, cmd := m.update(key('h'))
	m = drain(m, cmd)

	out := plain(m.render())
	r.Contains(out, "Job spec: web")
	r.Contains(out, "type = \"service\"")
}

func TestDescribe_Scrolls(t *testing.T) {
	r := require.New(t)

	long := []string{}
	for i := range 200 {
		long = append(long, fmt.Sprintf("line-%03d", i))
	}

	client := &fakeClient{jobs: twoJobs(), describe: strings.Join(long, "\n")}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))
	described, cmd := m.update(key('d'))
	m = drain(described, cmd)

	first := plain(m.render())
	r.Contains(first, long[0])

	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	r.NotContains(plain(m.render()), long[0])

	m, _ = m.update(key('G'))
	r.Contains(plain(m.render()), long[len(long)-1])

	m, _ = m.update(key('g'))
	r.Contains(plain(m.render()), long[0])
}

func TestDescribe_EndReachesTheLastWrappedRow(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), describe: strings.Join(longLines(40), "\n")}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))
	described, cmd := m.update(key('d'))
	m = drain(described, cmd)

	m, _ = m.update(key('w'))

	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyEnd})
	r.Contains(plain(m.render()), "end-039")
}

func TestDescribe_EscapeGoesBack(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), describe: "{}"}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, cmd := m.update(key('d'))
	m = drain(m, cmd)
	r.Equal(screenDescribe, m.screen.kind)

	m, _ = m.update(escape())

	r.Equal(screenJobs, m.screen.kind)
	r.Contains(plain(m.render()), "Jobs (production) [2]")
}

func TestDescribe_DoesNotPoll(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), describe: "{}"}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, cmd := m.update(key('d'))
	m = drain(m, cmd)

	// A description is a snapshot, it does not go back to the cluster on a
	// timer.
	_, cmd = m.update(pollMsg{})
	r.Nil(cmd)
}

func TestDescribe_TheNextOneOpensAtTheTop(t *testing.T) {
	r := require.New(t)

	long := []string{}
	for i := range 200 {
		long = append(long, fmt.Sprintf("line-%03d", i))
	}

	client := &fakeClient{jobs: twoJobs(), describe: strings.Join(long, "\n")}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	described, cmd := m.update(key('d'))
	m = drain(described, cmd)
	m, _ = m.update(key('G'))
	r.NotContains(plain(m.render()), long[0])

	// Read to the end, left, and another one asked: it starts where it
	// starts, not where the last one was left.
	m, _ = m.update(escape())
	described, cmd = m.update(key('d'))
	m = drain(described, cmd)

	r.Contains(plain(m.render()), long[0])
}
