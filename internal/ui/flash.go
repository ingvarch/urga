package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// flashFor is how long a message stands before it clears itself. A message
// from a minute ago is not news, and a screen that keeps one reads as if it
// just happened. k9s does the same with its own.
const flashFor = 6 * time.Second

// What a message is: what came of an action, something worth knowing, or
// something that went wrong.
type flashLevel int

const (
	flashInfo flashLevel = iota
	flashWarn
	flashErr
)

// flashOverMsg is the end of the time a message stands for.
type flashOverMsg struct{ at time.Time }

// flash is the one thing the status line has to say.
type flash struct {
	text  string
	level flashLevel
	at    time.Time
}

// say reports what came of something the person asked for.
func (m Model) say(text string) Model {
	return m.flashed(text, flashInfo)
}

// warn says something worth knowing that nobody asked about, like the
// cluster refusing to stream what it is doing.
func (m Model) warn(text string) Model {
	return m.flashed(text, flashWarn)
}

// fail says what went wrong. Nothing is swallowed: a request that did not
// go through is on the screen, in the words of the cluster.
func (m Model) fail(err error) Model {
	if err == nil {
		return m
	}

	return m.flashed(err.Error(), flashErr)
}

// quiet takes the message off the screen.
func (m Model) quiet() Model {
	m.flash = flash{}

	return m
}

func (m Model) flashed(text string, level flashLevel) Model {
	m.flash = flash{text: text, level: level, at: time.Now()}

	return m
}

// forget takes an error off the screen once the cluster has answered: what
// went wrong is no longer what is happening. What came of an action stands
// its time, so that it is not swallowed by the next poll.
func (m Model) forget() Model {
	if m.flash.level == flashErr {
		return m.quiet()
	}

	return m
}

// failed says whether what is on the screen went wrong, which is what the
// screen holds on to until the next answer.
func (m Model) failed() bool {
	return m.flash.text != "" && m.flash.level == flashErr && m.flash.fresh()
}

// fresh says the message is still worth reading.
func (f flash) fresh() bool {
	return f.text != "" && time.Since(f.at) < flashFor
}

// view is the message as the status line reads it: what went wrong and what
// is worth knowing are marked, what came of an action is not.
func (f flash) view(width int) string {
	text, style := f.text, styleValue

	switch f.level {
	case flashWarn:
		text, style = "! "+text, styleWarn
	case flashErr:
		text, style = "! "+text, styleError
	}

	return style.Render(truncate(text, width))
}

// clearFlash takes a message off once its time is up, unless a newer one has
// taken its place.
func (m Model) clearFlash(msg flashOverMsg) Model {
	if m.flash.at.After(msg.at) {
		return m
	}

	return m.quiet()
}

// flashTimer asks for the redraw that clears a message. Nothing else would
// redraw the screen in time for it to go away on its own.
func flashTimer(at time.Time) tea.Cmd {
	return tea.Tick(flashFor, func(time.Time) tea.Msg { return flashOverMsg{at: at} })
}
