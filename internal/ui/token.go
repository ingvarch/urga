package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// tokenMsg is the token the session sends, as the cluster sees it.
type tokenMsg nomad.Token

// tokenLabel labels the token the way the header labels what it shows.
const tokenLabel = "Token: "

// How long a token has left before its colour warns, and before the status
// line says when it expires instead of its name.
const (
	tokenCalm  = 30 * 24 * time.Hour
	tokenSoon  = 14 * 24 * time.Hour
	tokenClose = 5 * 24 * time.Hour
)

// fetchToken asks the cluster whose token the session sends.
func fetchToken(client Client) tea.Cmd {
	return request(client.Token, func(token nomad.Token) tea.Msg { return tokenMsg(token) })
}

// keepToken keeps what the cluster said about the token.
func (m Model) keepToken(msg tokenMsg) Model {
	token := nomad.Token(msg)
	m.token = &token

	return m
}

// tokenStatus is what the status line says about the token after its
// label: its name, in a colour that warns as it gets close to expiring, and
// under five days how long it has left. A cluster without ACLs has no token
// to speak of.
func tokenStatus(token nomad.Token, now time.Time) (string, lipgloss.Style, bool) {
	switch {
	case token.ACLsOff:
		return "", lipgloss.Style{}, false
	case token.Refused:
		return "not valid", styleError, true
	case token.Anonymous:
		return "anonymous", styleMuted, true
	case token.Expires.IsZero():
		return token.Name, styleValue, true
	}

	left := token.Expires.Sub(now)

	switch {
	case left <= 0:
		return "expired", styleError, true
	case left < tokenClose:
		return "expires in " + timeLeft(left), styleError, true
	case left < tokenSoon:
		return token.Name, styleWarn, true
	case left < tokenCalm:
		return token.Name, stylePending, true
	}

	return token.Name, styleValue, true
}

// timeLeft is how long is left, in the largest whole unit: days, hours, or
// minutes, and never less than a minute.
func timeLeft(left time.Duration) string {
	switch {
	case left >= 24*time.Hour:
		return plural(int(left/(24*time.Hour)), "day")
	case left >= time.Hour:
		return plural(int(left/time.Hour), "hour")
	}

	return plural(max(int(left/time.Minute), 1), "minute")
}
