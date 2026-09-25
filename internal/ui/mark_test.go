package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func space() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "} }

// onAllocations opens the allocations of a job, where marks are of use.
func onAllocations(t *testing.T) (Model, *fakeClient) {
	t.Helper()

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(enter())
	m, _ = m.update(allocsMsg(twoAllocs()))

	return m, client
}

// cursorOnMark says the row under the cursor is drawn as a marked row: the
// cursor takes the color of the mark instead of covering it.
func cursorOnMark(m Model) bool {
	return strings.Contains(m.render(), styleCode(styleSelectedMark))
}

// markedRows are the rows of the screen drawn in the color of a mark.
func markedRows(m Model) []string {
	want := colorCode(colorMark)

	out := []string{}

	for _, line := range strings.Split(m.render(), "\n") {
		if strings.Contains(line, want) {
			out = append(out, strings.TrimSpace(plain(line)))
		}
	}

	return out
}

func TestMarks_SpaceMarksTheRowUnderTheCursor(t *testing.T) {
	r := require.New(t)

	m, _ := onAllocations(t)

	m, _ = m.update(key('j'))
	m, _ = m.update(space())

	// The row under the cursor is drawn as the cursor, so step off it to
	// see the color of a mark. Nothing is added to the row and no column
	// moves for it.
	m, _ = m.update(key('k'))

	marked := markedRows(m)
	r.Len(marked, 1)
	r.Contains(marked[0], "b2222222")

	// The same key lets it go again.
	m, _ = m.update(key('j'))
	m, _ = m.update(space())
	m, _ = m.update(key('k'))

	r.Empty(markedRows(m))
}

func TestMarks_AMarkOutranksTheStateOfTheRow(t *testing.T) {
	r := require.New(t)

	m, _ := onAllocations(t)

	// The second allocation is pending, which has a color of its own. A
	// mark is what the eye is looking for, so it wins.
	m, _ = m.update(key('j'))
	m, _ = m.update(space())
	m, _ = m.update(key('k'))

	marked := markedRows(m)
	r.Len(marked, 1)
	r.Contains(marked[0], "pending")
}

func TestMarks_AnActionTakesEveryMarkedRow(t *testing.T) {
	r := require.New(t)

	m, client := onAllocations(t)

	m, _ = m.update(space())
	m, _ = m.update(key('j'))
	m, _ = m.update(space())

	m, _ = m.update(key('r'))
	r.Contains(plain(m.render()), "restart 2 allocations")

	_, cmd := answerYes(m)
	drain(m, cmd)

	r.Equal(2, client.restarted)
}

func TestMarks_WithoutMarksTheCursorIsTheRow(t *testing.T) {
	r := require.New(t)

	m, client := onAllocations(t)

	m, _ = m.update(key('r'))
	r.Contains(plain(m.render()), "restart the allocation af1f37df")

	_, cmd := answerYes(m)
	drain(m, cmd)

	r.Equal(1, client.restarted)
}

func TestMarks_SurviveTheListBeingAskedAgain(t *testing.T) {
	r := require.New(t)

	m, _ := onAllocations(t)

	m, _ = m.update(space())

	// The cluster answers again with the same allocations; a mark belongs
	// to the allocation, not to the row it happened to be on.
	m, _ = m.update(allocsMsg(twoAllocs()))

	// The cursor has a color of its own and stands over a mark, so step
	// off the row to see it.
	m, _ = m.update(key('j'))

	r.Len(markedRows(m), 1)
}

func TestMarks_AreLetGoOfWithTheScreen(t *testing.T) {
	r := require.New(t)

	m, _ := onAllocations(t)

	m, _ = m.update(space())
	m, _ = m.update(escape())

	// Back on the jobs, and into the allocations again: what was marked
	// before belonged to the screen that was left.
	m, _ = m.update(enter())
	m, _ = m.update(allocsMsg(twoAllocs()))

	r.Empty(markedRows(m))
}

func TestMarks_MarkEveryRowAndNoneAgain(t *testing.T) {
	r := require.New(t)

	m, _ := onAllocations(t)

	m, _ = m.update(ctrlKey('a'))

	// Both are taken; the one under the cursor is drawn as the cursor.
	r.Len(m.list.marks, 2)
	r.Len(markedRows(m), 1)

	m, _ = m.update(ctrlKey('a'))
	r.Empty(m.list.marks)
	r.Empty(markedRows(m))
}

func TestMarks_AScreenWithoutThemTakesTheKeyQuietly(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{namespaces: twoNamespaces()}

	m := newTestModel(client)
	m, _ = m.update(key(':'))
	m = typeIn(m, "namespaces")
	m, _ = m.update(enter())
	m, _ = m.update(namespacesMsg(twoNamespaces()))

	before := plain(m.render())
	m, _ = m.update(space())

	// Nothing acts on a marked namespace, so the screen does not take one.
	r.Equal(before, plain(m.render()))
}

func TestMarks_StopTakesEveryMarkedRowToo(t *testing.T) {
	r := require.New(t)

	m, client := onAllocations(t)

	m, _ = m.update(ctrlKey('a'))
	m, _ = m.update(ctrlKey('k'))

	r.Contains(plain(m.render()), "stop 2 allocations")

	_, cmd := answerYes(m)
	drain(m, cmd)

	r.Equal(2, client.stoppedAllocs)
}

func TestMarks_AreLetGoOfWhenTheActionIsDone(t *testing.T) {
	r := require.New(t)

	m, _ := onAllocations(t)

	m, _ = m.update(space())
	r.Len(m.list.marks, 1)

	m, _ = m.update(key('r'))

	m, cmd := answerYes(m)
	m = drain(m, cmd)

	// What was marked has been acted on; leaving the marks up would take
	// them along into the next action by surprise.
	r.Empty(m.list.marks)
	r.Empty(markedRows(m))
}

func TestMarks_AreActedOnEvenWhenTheFilterHidesThem(t *testing.T) {
	r := require.New(t)

	m, client := onAllocations(t)

	m, _ = m.update(space())

	m, _ = m.update(key('/'))
	m = typeIn(m, "backend")
	m, _ = m.update(enter())

	// The mark is on an allocation, not on a line of the screen. A key that
	// silently does nothing is worse than either answer.
	m, _ = m.update(key('r'))
	r.Contains(plain(m.render()), "restart the allocation af1f37df")

	_, cmd := answerYes(m)
	drain(m, cmd)

	r.Equal(1, client.restarted)
}

func TestMarks_AnActionThatFailsPartWaySaysHowFar(t *testing.T) {
	r := require.New(t)

	m, client := onAllocations(t)
	client.actionErr = errors.New("connection refused")

	m, _ = m.update(ctrlKey('a'))
	m, _ = m.update(key('r'))

	m, cmd := answerYes(m)
	m = drain(m, cmd)

	// Every marked row is tried, not only the ones before the first
	// failure, and the screen says what came of it.
	r.Equal(2, client.restarted)

	out := plain(m.render())
	r.Contains(out, "0 of 2")
	r.Contains(out, "connection refused")
}

func TestMarks_AreKeptWhenTheActionFails(t *testing.T) {
	r := require.New(t)

	m, client := onAllocations(t)
	client.actionErr = errors.New("connection refused")

	m, _ = m.update(space())
	m, _ = m.update(key('r'))

	m, cmd := answerYes(m)
	m = drain(m, cmd)

	// What did not happen is still marked, so the same key tries it again.
	r.Len(m.list.marks, 1)
}

func TestMarks_MarkingAllUnderAFilterTakesWhatIsShown(t *testing.T) {
	r := require.New(t)

	m, _ := onAllocations(t)

	m, _ = m.update(space())

	m, _ = m.update(key('/'))
	m = typeIn(m, "backend")
	m, _ = m.update(enter())

	m, _ = m.update(ctrlKey('a'))

	// One row is shown and one mark is hidden: marking all of what is shown
	// adds it, rather than clearing what is not.
	r.Len(m.list.marks, 2)
}

func TestMarks_StopEveryMarkedJob(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs()}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, _ = m.update(ctrlKey('a'))
	m, _ = m.update(ctrlKey('s'))

	// One of the two runs and one is dead, so the question says what is
	// about to happen to each of them.
	r.Contains(plain(m.render()), "start or stop 2 jobs")

	m, cmd := answerYes(m)
	drain(m, cmd)

	r.Equal(1, client.stopped)
	r.Equal(1, client.started)
}

func TestMarks_TheQuestionSaysWhatTheyHaveInCommon(t *testing.T) {
	r := require.New(t)

	running := []nomad.Job{
		{ID: "web", Name: "web", Namespace: "production", Status: "running"},
		{ID: "api", Name: "api", Namespace: "production", Status: "running"},
	}

	client := &fakeClient{jobs: running}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(running))

	m, _ = m.update(ctrlKey('a'))
	m, _ = m.update(ctrlKey('s'))

	r.Contains(plain(m.render()), "stop 2 jobs")
}

func TestMarks_DrainEveryMarkedClient(t *testing.T) {
	r := require.New(t)

	m, client := nodeModel(t, readyNode())

	m, _ = m.update(space())
	m, _ = m.update(ctrlKey('d'))

	r.Contains(plain(m.render()), "drain the client server-01")

	m, cmd := answerYes(m)
	drain(m, cmd)

	r.True(client.drained)
}

func TestMarks_TakeEveryMarkedClientOffWork(t *testing.T) {
	r := require.New(t)

	nodes := []nomad.Node{
		{ID: "node-1", Name: "server-01", Status: "ready", Eligibility: "eligible"},
		{ID: "node-2", Name: "server-02", Status: "ready", Eligibility: "eligible"},
	}

	m, client := nodeModel(t, nodes)

	m, _ = m.update(ctrlKey('a'))
	m, _ = m.update(key('i'))

	r.Contains(plain(m.render()), "2 clients")

	m, cmd := answerYes(m)
	drain(m, cmd)

	r.Equal(2, client.eligibleCalls)
}

// pickyClient refuses everything about the job or the client it is told to.
type pickyClient struct {
	*fakeClient

	refuse string
	acted  []string
}

func (p *pickyClient) StopJob(_ context.Context, _, jobID string) error {
	return p.act(jobID, "stop "+jobID)
}

func (p *pickyClient) StartJob(_ context.Context, _, jobID string) error {
	return p.act(jobID, "start "+jobID)
}

func (p *pickyClient) DrainNode(_ context.Context, nodeID string, drain bool) error {
	return p.act(nodeID, fmt.Sprintf("drain %s %t", nodeID, drain))
}

func (p *pickyClient) act(id, what string) error {
	if id == p.refuse {
		return errors.New("connection refused")
	}

	p.acted = append(p.acted, what)

	return nil
}

func TestMarks_OnlyWhatDidNotHappenStaysMarked(t *testing.T) {
	r := require.New(t)

	running := []nomad.Job{
		{ID: "web", Name: "web", Namespace: "production", Status: "running"},
		{ID: "cron", Name: "cron", Namespace: "production", Status: "running"},
	}

	client := &pickyClient{fakeClient: &fakeClient{jobs: running}, refuse: "cron"}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(running))

	m, _ = m.update(ctrlKey('a'))
	m, _ = m.update(ctrlKey('s'))

	m, cmd := answerYes(m)
	m = drain(m, cmd)

	// One went through and one did not. Pressing the key again must try
	// the one that did not, and leave alone the one that did: it is in the
	// other state now, and the same key would put it back.
	r.Equal([]string{"stop web"}, client.acted)
	r.Len(m.list.marks, 1)

	m, _ = m.update(jobsMsg([]nomad.Job{
		{ID: "web", Name: "web", Namespace: "production", Status: "dead"},
		{ID: "cron", Name: "cron", Namespace: "production", Status: "running"},
	}))

	m, _ = m.update(ctrlKey('s'))
	r.Contains(plain(m.render()), "stop the job cron")
}

func TestMarks_EveryClientOfABatchIsTried(t *testing.T) {
	r := require.New(t)

	nodes := []nomad.Node{
		{ID: "node-1", Name: "server-01", Status: "ready", Eligibility: "eligible"},
		{ID: "node-2", Name: "server-02", Status: "ready", Eligibility: "eligible"},
	}

	client := &pickyClient{fakeClient: &fakeClient{nodes: nodes}, refuse: "node-1"}

	m, _ := nodeModelOf(client.fakeClient)
	m.client = client

	m, _ = m.update(ctrlKey('a'))
	m, _ = m.update(ctrlKey('d'))

	m, cmd := answerYes(m)
	m = drain(m, cmd)

	// The one that would not answer does not stop the other, and the
	// screen says how far it got.
	r.Equal([]string{"drain node-2 true"}, client.acted)
	r.Contains(plain(m.render()), "1 of 2")
}

func TestMarks_AQuestionAboutRowsThatDisagreeReadsAsOne(t *testing.T) {
	r := require.New(t)

	nodes := []nomad.Node{
		{ID: "node-1", Name: "server-01", Status: "ready", Eligibility: "eligible"},
		{ID: "node-2", Name: "server-02", Status: "ready", Eligibility: "ineligible", Drain: true},
	}

	for _, press := range []struct {
		key      tea.KeyPressMsg
		question string
	}{
		{ctrlKey('d'), "Really change the draining of 2 clients?"},
		{key('i'), "Really change the work of 2 clients?"},
	} {
		m, _ := nodeModel(t, nodes)
		m, _ = m.update(ctrlKey('a'))
		m, _ = m.update(press.key)

		// One is draining and one is not, so neither verb is true of both.
		// A question is a sentence, not two of them stuck together.
		r.Contains(plain(m.render()), press.question)

		m, cmd := answerYes(m)
		m = drain(m, cmd)

		// What came of it reads as a sentence too.
		r.Regexp(`Changed (the (draining|work) of )?2 clients\.`, plain(m.render()))
	}
}

func TestMarks_AQuestionAboutJobsThatDisagree(t *testing.T) {
	r := require.New(t)

	jobs := []nomad.Job{
		{ID: "web", Name: "web", Namespace: "production", Status: "running"},
		{ID: "cron", Name: "cron", Namespace: "production", Status: "dead"},
	}

	m := newTestModel(&fakeClient{jobs: jobs})
	m, _ = m.update(jobsMsg(jobs))

	m, _ = m.update(ctrlKey('a'))
	m, _ = m.update(ctrlKey('s'))

	r.Contains(plain(m.render()), "Really start or stop 2 jobs?")

	m, cmd := answerYes(m)
	m = drain(m, cmd)

	r.Contains(plain(m.render()), "Changed 2 jobs.")
}

func TestMarks_TheCursorTakesTheColorOfAMark(t *testing.T) {
	r := require.New(t)

	m, _ := onAllocations(t)

	r.False(cursorOnMark(m))

	m, _ = m.update(space())

	// Standing on a marked row must not hide the mark: the cursor is drawn
	// in the color of the mark, the whole row over.
	r.True(cursorOnMark(m))

	m, _ = m.update(space())
	r.False(cursorOnMark(m))
}
