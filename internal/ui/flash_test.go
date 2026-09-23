package ui

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFlash_SaysWhatCameOfAnAction(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)

	m = m.say("Job web stopped.")

	r.Contains(plain(m.render()), "Job web stopped.")
	r.Equal(flashInfo, m.flash.level)
}

func TestFlash_SaysWhatWentWrong(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)

	m = m.fail(errors.New("connection refused"))

	out := plain(m.render())
	r.Contains(out, "! connection refused")
	r.Equal(flashErr, m.flash.level)
}

func TestFlash_SaysWhatIsWorthKnowing(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)

	m = m.warn("the cluster will not stream events")

	r.Contains(plain(m.render()), "the cluster will not stream events")
	r.Equal(flashWarn, m.flash.level)
}

func TestFlash_GoesAwayOnItsOwn(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)

	m = m.fail(errors.New("connection refused"))
	r.Contains(plain(m.render()), "connection refused")

	// A message from a minute ago is not news. It clears itself, the way
	// k9s clears its own.
	m.flash.at = time.Now().Add(-flashFor - time.Second)

	out := plain(m.render())
	r.NotContains(out, "connection refused")
	r.Contains(out, "<:> command")
}

func TestFlash_ATimerComesWithEveryMessage(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)

	_, cmd := m.Update(errMsg{err: errors.New("connection refused")})

	// Nothing else would redraw the screen in time for the message to go
	// away, so setting one asks for the redraw that clears it.
	r.NotNil(cmd)
}

func TestFlash_TheOneThatClearsItLeavesANewerOneAlone(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)

	m = m.fail(errors.New("first"))
	m = m.say("second")

	m, _ = m.update(flashOverMsg{at: m.flash.at.Add(-time.Minute)})

	// The timer of a message that was replaced must not take the one that
	// replaced it off the screen.
	r.Contains(plain(m.render()), "second")
}
