package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/stretchr/testify/require"
)

// slowlyWatched is a model whose screen watches the stream and has the slow
// tick pending, and the message that tick will send.
func slowlyWatched(t *testing.T, client *fakeClient) (Model, pollMsg) {
	t.Helper()

	m := watching(t, client)

	// The timer set before the stream opened has fired, so the next
	// answer sets the slow one.
	m, _ = m.update(pollMsg{tick: m.pollTick})
	m, cmd := m.update(jobsMsg(twoJobs()))

	require.NotNil(t, cmd)
	require.Equal(t, slowPoll, m.pollEvery())

	return m, pollMsg{tick: m.pollTick}
}

// pollIn is the poll timer among what a model asked for. Waiting for it
// takes PollEvery, which tests keep short; a slower timer returns nothing.
func pollIn(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()

	if cmd == nil {
		return nil
	}

	msg := within(cmd, time.Second)

	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, next := range batch {
			if poll, ok := pollIn(t, next).(pollMsg); ok {
				return poll
			}
		}

		return nil
	}

	return msg
}

func TestPoll_ANewScreenIsPolledAtItsOwnPaceAndTheOldSlowTickAsksForNothing(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), changes: newChanges()}

	m, slow := slowlyWatched(t, client)

	m, _ = m.update(enter())

	m, cmd := m.update(m.fetch()())
	r.NotNil(cmd)

	// The first poll of the new screen comes at PollEvery, not after the
	// 30 seconds the screen before it was waiting.
	fresh := pollIn(t, cmd)
	r.IsType(pollMsg{}, fresh)

	calls := client.calls

	_, cmd = m.update(slow)
	r.Nil(cmd)
	r.Equal(calls, client.calls)

	_, cmd = m.update(fresh)
	r.NotNil(cmd)
}

func TestPoll_AStreamThatEndsStartsAPollAtPollEvery(t *testing.T) {
	r := require.New(t)

	m, slow := slowlyWatched(t, &fakeClient{jobs: twoJobs(), changes: newChanges()})

	m, cmd := m.update(watchEndedMsg{id: m.watch.id})
	r.NotNil(cmd)

	fresh := pollIn(t, cmd)
	r.IsType(pollMsg{}, fresh)

	_, cmd = m.update(slow)
	r.Nil(cmd)

	_, cmd = m.update(fresh)
	r.NotNil(cmd)
}

func TestPoll_AScreenPollsOncePerTickHoweverOftenTheScreenChanged(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := newTestModel(client)

	var ticks []tea.Msg

	collect := func(cmd tea.Cmd) {
		if tick, ok := pollIn(t, cmd).(pollMsg); ok {
			ticks = append(ticks, tick)
		}
	}

	var cmd tea.Cmd

	for range 3 {
		m, _ = m.update(enter())
		m, cmd = m.update(m.fetch()())
		collect(cmd)

		m, _ = m.update(escape())
		m, cmd = m.update(m.fetch()())
		collect(cmd)
	}

	r.NotEmpty(ticks)

	polls := 0

	for _, tick := range ticks {
		var next tea.Cmd

		m, next = m.update(tick)
		if next != nil {
			polls++
		}
	}

	r.Equal(1, polls)
}
