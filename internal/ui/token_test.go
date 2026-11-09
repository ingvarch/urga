package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestTokenStatus(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour

	bot := func(left time.Duration) nomad.Token {
		return nomad.Token{Name: "deploy-bot", Type: "client", Expires: now.Add(left)}
	}

	unnamed := nomad.Token{ID: "3fa85f64-5717-4562-b3fc-2c963f66afa6", Type: "client"}
	unnamedFor := func(left time.Duration) nomad.Token {
		token := unnamed
		token.Expires = now.Add(left)

		return token
	}

	for name, tc := range map[string]struct {
		token nomad.Token
		value string
		style lipgloss.Style
	}{
		"no expiry":         {nomad.Token{Name: "deploy-bot", Type: "client"}, "deploy-bot", styleValue},
		"more than 30 days": {bot(40 * day), "deploy-bot", styleValue},
		"14 to 30 days":     {bot(20 * day), "deploy-bot", stylePending},
		"no name":           {unnamed, "3fa85f64", styleValue},
		"no name, expiring": {unnamedFor(20 * day), "3fa85f64", stylePending},
		"5 to 14 days":      {bot(10 * day), "deploy-bot", styleWarn},
		"4 days":            {bot(4*day + 12*time.Hour), "expires in 4 days", styleError},
		"1 day":             {bot(day + 3*time.Hour), "expires in 1 day", styleError},
		"hours":             {bot(5 * time.Hour), "expires in 5 hours", styleError},
		"an hour":           {bot(time.Hour + time.Minute), "expires in 1 hour", styleError},
		"minutes":           {bot(12 * time.Minute), "expires in 12 minutes", styleError},
		"under a minute":    {bot(30 * time.Second), "expires in 1 minute", styleError},
		"expired":           {bot(-time.Minute), "expired", styleError},
		"refused":           {nomad.Token{Refused: true}, "not valid", styleError},
		"anonymous":         {nomad.Token{Anonymous: true, Name: "Anonymous Token"}, "anonymous", styleMuted},
		"management":        {nomad.Token{Name: "ops", Type: "management"}, "ops", styleValue},
	} {
		t.Run(name, func(t *testing.T) {
			value, style, ok := tokenStatus(tc.token, now)
			require.True(t, ok)
			require.Equal(t, tc.value, value)
			require.Equal(t, opening(tc.style), opening(style))
		})
	}

	// A cluster without ACLs has no token to show.
	_, _, ok := tokenStatus(nomad.Token{ACLsOff: true}, now)
	require.False(t, ok)
}

// statusLine is the last line of the screen, in its colours.
func statusLine(m Model) string {
	rows := strings.Split(m.render(), "\n")

	return rows[len(rows)-1]
}

func withToken(token nomad.Token, width int) Model {
	m := New(&fakeClient{token: token}, Options{Version: "v-test"})
	m, _ = m.update(tea.WindowSizeMsg{Width: width, Height: 30})
	m, _ = m.update(tokenMsg(token))

	return m
}

func TestStatusLine_TheTokenOnTheRight(t *testing.T) {
	r := require.New(t)

	m := withToken(nomad.Token{Name: "deploy-bot", Type: "client"}, 120)
	line := plain(statusLine(m))

	// The keys on the left as ever, the token where the header ends.
	r.True(strings.HasPrefix(line, "  <:> command"), line)
	r.True(strings.HasSuffix(line, "Token: deploy-bot"), line)
	r.Equal(120-headerPadX, ansi.StringWidth(statusLine(m)))

	// Labelled the way the header labels what it shows, the value in the
	// colour of how long the token has left.
	r.Contains(statusLine(m), opening(styleLabel)+"Token:")
	r.Contains(statusLine(m), opening(styleValue)+"deploy-bot")
}

func TestStatusLine_ANarrowScreenKeepsTheToken(t *testing.T) {
	r := require.New(t)

	m := withToken(nomad.Token{Name: "deploy-bot", Type: "client"}, 50)
	line := plain(statusLine(m))

	r.Contains(line, "Token: deploy-bot")
	r.Contains(line, "…")
	r.LessOrEqual(ansi.StringWidth(statusLine(m)), 50)
}

func TestStatusLine_AMessageAndTheToken(t *testing.T) {
	r := require.New(t)

	m := withToken(nomad.Token{Name: "deploy-bot", Type: "client"}, 120)
	m = m.say("Job web submitted.")

	line := plain(statusLine(m))
	r.Contains(line, "Job web submitted.")
	r.Contains(line, "Token: deploy-bot")
}

func TestStatusLine_NoTokenToSpeakOf(t *testing.T) {
	r := require.New(t)

	// Before the cluster answered, and on a cluster without ACLs.
	m := New(&fakeClient{}, Options{Version: "v-test"})
	m, _ = m.update(sizeMsg())
	r.NotContains(plain(statusLine(m)), "Token")

	m, _ = m.update(tokenMsg(nomad.Token{ACLsOff: true}))
	r.NotContains(plain(statusLine(m)), "Token")
}

func TestToken_AskedWithTheCluster(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{token: nomad.Token{Name: "deploy-bot", Type: "client"}}
	m := New(client, Options{Version: "v-test"})
	m, _ = m.update(sizeMsg())
	m = playOut(m, m.Init())

	r.Equal(1, client.tokenCalls)
	r.Contains(plain(statusLine(m)), "Token: deploy-bot")
}

func TestToken_OfTheClusterLeftGoesWithIt(t *testing.T) {
	r := require.New(t)

	m, clusters := onDev(t)
	m, _ = m.update(tokenMsg(nomad.Token{Name: "dev-bot", Type: "client"}))
	r.Contains(plain(statusLine(m)), "Token: dev-bot")

	// Connected to prod, before prod said whose token it is.
	m, _ = m.update(connectedMsg{conn: Connection{Name: "prod", Client: clusters.prod}})

	r.NotContains(plain(statusLine(m)), "dev-bot")
}

func TestToken_ARefusalOfTheClusterLeftIsNotExplainedByTheNext(t *testing.T) {
	r := require.New(t)

	// dev refused a request before it said whose token it is.
	m, clusters := onDev(t)
	m, _ = m.update(errMsg{err: forbidden(t)})

	m, _ = m.update(connectedMsg{conn: Connection{Name: "prod", Client: clusters.prod}})
	m, _ = m.update(tokenMsg(nomad.Token{Anonymous: true}))

	r.NotContains(plain(statusLine(m)), "no token is set")
}

// onHTTP is a session on a cluster the token goes to over plain HTTP, before
// its first answer.
func onHTTP(client *fakeClient) Model {
	client.address, client.inClear, client.changes = "http://nomad.example:4646", true, newChanges()

	m := New(client, Options{Version: "v-test"})
	m, _ = m.update(sizeMsg())

	return m
}

const inClearWarning = "! the token goes to http://nomad.example:4646 without TLS"

func TestToken_AWarningWhenItGoesInClear(t *testing.T) {
	r := require.New(t)

	m := onHTTP(&fakeClient{jobs: twoJobs()})
	r.NotContains(plain(statusLine(m)), "without TLS")

	m = drain(m, m.fetch())
	r.Contains(plain(statusLine(m)), inClearWarning)

	// Once for the connection: the next answer does not show it again.
	m = m.quiet()
	m = drain(m, m.fetch())
	r.NotContains(plain(statusLine(m)), "without TLS")
}

func TestToken_NoWarningWhenItDoesNotGoInClear(t *testing.T) {
	m := New(&fakeClient{jobs: twoJobs(), changes: newChanges()}, Options{Version: "v-test"})
	m, _ = m.update(sizeMsg())
	m = drain(m, m.fetch())

	require.NotContains(t, plain(statusLine(m)), "without TLS")
}

func TestToken_TheWarningInClearDoesNotReplaceAnError(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{err: errors.New("no route to host")}
	m := onHTTP(client)

	// The first read fails: its error stays.
	m = drain(m, m.fetch())
	r.Contains(plain(statusLine(m)), "no route to host")
	r.NotContains(plain(statusLine(m)), "without TLS")

	// The warning comes with the first answer.
	client.err, client.jobs = nil, twoJobs()
	m = drain(m, m.fetch())
	r.Contains(plain(statusLine(m)), inClearWarning)
}

func TestToken_TheWarningInClearWaitsForTheLineToClear(t *testing.T) {
	for name, shown := range map[string]func(Model) Model{
		"another warning":   func(m Model) Model { return m.warn("not following the cluster") },
		"an action error":   func(m Model) Model { return m.failedLate("job not found") },
		"a running command": func(m Model) Model { return m.busy("Stopping web...") },
	} {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			m := shown(onHTTP(&fakeClient{jobs: twoJobs()}))
			text := m.flash.text

			m = drain(m, m.fetch())
			r.Contains(plain(statusLine(m)), text)

			m = m.quiet()
			m = drain(m, m.fetch())
			r.Contains(plain(statusLine(m)), inClearWarning)
		})
	}
}
