package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// flashFor is how long a message stays before it clears itself. A message
// from a minute ago is not news, and a screen that keeps one looks as if it
// just happened.
const flashFor = 6 * time.Second

// What a message is: what came of an action, something worth knowing, or
// something that went wrong.
type flashLevel int

const (
	flashInfo flashLevel = iota
	flashWarn
	flashErr
)

// flashOverMsg arrives when the time of a message is up.
type flashOverMsg struct{ at time.Time }

// flash is the one message the status line shows.
type flash struct {
	text  string
	level flashLevel
	at    time.Time
}

// say reports what came of something the person asked for.
func (m Model) say(text string) Model {
	return m.flashed(text, flashInfo)
}

// warn shows something worth knowing that nobody asked about, like the
// cluster refusing the event stream.
func (m Model) warn(text string) Model {
	return m.flashed(text, flashWarn)
}

// fail shows what went wrong. No error is dropped: a request that failed
// shows on the screen with the error text of the cluster.
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

// forget clears an error once the cluster has answered: the error is out of
// date. A message about an action stays for its full time, so the next poll
// does not clear it.
func (m Model) forget() Model {
	if m.flash.level == flashErr {
		return m.quiet()
	}

	return m
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
