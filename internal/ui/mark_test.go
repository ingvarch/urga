package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
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

func TestMarks_SpaceMarksTheRowUnderTheCursor(t *testing.T) {
	r := require.New(t)

	m, _ := onAllocations(t)

	m, _ = m.update(space())

	// A marked row carries a mark where the list has its margin, so the
	// columns do not move.
	r.Contains(plain(m.render()), "•af1f37df")

	// The same key lets it go again.
	m, _ = m.update(space())
	r.NotContains(plain(m.render()), "•")
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

	r.Contains(plain(m.render()), "•af1f37df")
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

	r.NotContains(plain(m.render()), "•")
}

func TestMarks_MarkEveryRowAndNoneAgain(t *testing.T) {
	r := require.New(t)

	m, _ := onAllocations(t)

	m, _ = m.update(ctrlKey('a'))
	r.Equal(2, strings.Count(plain(m.render()), "•"))

	m, _ = m.update(ctrlKey('a'))
	r.NotContains(plain(m.render()), "•")
}

func TestMarks_AScreenWithoutThemTakesTheKeyQuietly(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs()}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	before := plain(m.render())
	m, _ = m.update(space())

	// Nothing on the jobs answers a mark, so nothing happens.
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
	r.Contains(plain(m.render()), "•")

	m, _ = m.update(key('r'))
	m, _ = answerYes(m)

	// What was marked was acted on; leaving the marks up would take them
	// along into the next action by surprise.
	r.NotContains(plain(m.render()), "•")
}
