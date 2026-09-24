package ui

import (
	"fmt"
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

// keepToken keeps what the cluster said about the token. A refusal still on
// the status line is said again with what the token explains of it; a
// message said since is left alone.
func (m Model) keepToken(msg tokenMsg) Model {
	token := nomad.Token(msg)
	m.token = &token

	refused := m.refused
	m.refused = nil

	if refused == nil || !m.flash.fresh() || m.flash.text != refused.Error() {
		return m
	}

	if why, ok := m.refusedBecause(refused); ok {
		return m.flashed(why, flashErr)
	}

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

// tokenHint is what an empty list says about the token: it may be why the
// list is empty. A token that reads only some namespaces is answered an
// empty list for the others, and no error. Until the list is answered, or
// when the filter hid what it holds, there is nothing to say.
func (m Model) tokenHint() string {
	if !m.answered || m.held > 0 || m.token == nil || m.readsAsText() {
		return ""
	}

	token := *m.token

	switch {
	case token.Anonymous:
		return "Nothing here: no token is set, and anonymous access may not allow it."
	case token.ACLsOff, token.Refused, token.Type == managementToken:
		return ""
	}

	return fmt.Sprintf("Nothing here that %s can read: its policies may not allow it.", token.Name)
}

// withHint puts the hint about the token under the column titles of an
// empty list.
func (m Model) withHint(table string, width int) string {
	hint := m.tokenHint()
	if hint == "" {
		return table
	}

	return table + "\n " + styleMuted.Render(truncate(hint, width-1))
}

// managementToken is the type of a token that may do anything.
const managementToken = "management"

// refusedBecause says why the cluster refused a request, when the token is
// the likely reason. What a management token is refused, the cluster says
// best itself.
func (m Model) refusedBecause(err error) (string, bool) {
	if !nomad.Forbidden(err) || m.token == nil {
		return "", false
	}

	token := *m.token

	switch {
	case token.Refused:
		return "Permission denied: the token is not valid", true
	case token.Anonymous:
		return "Permission denied: no token is set", true
	case token.ACLsOff, token.Type == managementToken:
		return "", false
	}

	return fmt.Sprintf("Permission denied: %s may not do this", token.Name), true
}
