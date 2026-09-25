package ui

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// releases answers whether a newer urga is out, in turn, and counts the
// asks. Past the last answer it keeps giving that one.
type releases struct {
	answers []string
	errs    []error
	asked   int
}

func (r *releases) newer(context.Context) (string, error) {
	i := min(r.asked, len(r.answers)-1)
	r.asked++

	var err error
	if i < len(r.errs) {
		err = r.errs[i]
	}

	return r.answers[i], err
}

// startedAt is a session of this version that has done what it does on
// start.
func startedAt(t *testing.T, version string, out *releases) Model {
	t.Helper()

	m := New(&fakeClient{}, Options{Version: version, NewerRelease: out.newer})
	m, _ = m.update(sizeMsg())

	return playOut(m, m.Init())
}

func TestNewerRelease_TheHeaderSaysItIsOut(t *testing.T) {
	r := require.New(t)

	out := &releases{answers: []string{"v0.5.1"}}
	m := startedAt(t, "v0.5.0 (40f73c8)", out)

	r.Equal(1, out.asked)
	r.Contains(headerOf(m), "v0.5.0 (40f73c8) ↑ v0.5.1")
}

func TestNewerRelease_NothingWhenThisIsTheLatest(t *testing.T) {
	r := require.New(t)

	m := startedAt(t, "v0.5.1 (9a3e5d1)", &releases{answers: []string{""}})

	r.Contains(headerOf(m), "v0.5.1 (9a3e5d1)")
	r.NotContains(headerOf(m), "↑")
}

func TestNewerRelease_AFailedAskSaysNothing(t *testing.T) {
	r := require.New(t)

	// A network that cannot reach the releases is not something to report.
	m := startedAt(t, "v0.5.0 (40f73c8)", &releases{answers: []string{""}, errs: []error{errors.New("dial tcp: i/o timeout")}})

	r.NotContains(headerOf(m), "↑")
	r.NotContains(plain(m.render()), "timeout")
}

func TestNewerRelease_AskedAgainLater(t *testing.T) {
	r := require.New(t)

	out := &releases{answers: []string{"", "v0.5.1", "v0.5.2"}, errs: []error{nil, nil, errors.New("rate limit")}}
	m := startedAt(t, "v0.5.0 (40f73c8)", out)

	// Each answer sets the next ask off.
	m, cmd := m.update(newerReleaseMsg{})
	r.NotNil(cmd)

	m, cmd = m.update(checkReleaseMsg{})
	m = drain(m, cmd)
	r.Equal(2, out.asked)
	r.Contains(headerOf(m), "↑ v0.5.1")

	// What a failed ask did not find out, it does not take away.
	m, cmd = m.update(checkReleaseMsg{})
	m = drain(m, cmd)
	r.Equal(3, out.asked)
	r.Contains(headerOf(m), "↑ v0.5.1")
}

func TestNewerRelease_StaysAcrossAClusterSwitch(t *testing.T) {
	r := require.New(t)

	m, _ := onDev(t)
	m, _ = m.update(newerReleaseMsg{version: "v0.5.1"})

	m = typeCommand(m, "ctx prod")
	r.Equal("prod", m.opts.Cluster)
	r.Contains(headerOf(m), "↑ v0.5.1")
}

func TestHeader_TheNewerReleaseIsNotCut(t *testing.T) {
	r := require.New(t)

	// The version gives way to the width of the column, the release after
	// it does not.
	out := renderHeader(header{address: "https://nomad.example.com", version: "v0.5.0 (40f73c8)", newer: "v0.5.1"}, 44)

	r.Contains(plain(out), "↑ v0.5.1")
	r.NotContains(plain(out), "v0.5.0 (40f73c8)")
}
