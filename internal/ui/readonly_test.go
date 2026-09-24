package ui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// readOnly is a model of a screen, started read-only, with an editor and a
// shell a write key would reach if it got through.
func readOnly(open Model, shell Shell) Model {
	m := open.quiet()
	m.opts.ReadOnly = true
	m.opts.Editor = &fakeEditor{replace: "changed"}
	m.opts.Shell = shell

	return m
}

func TestReadOnly_OffersOnlyWhatChangesNothing(t *testing.T) {
	r := require.New(t)

	for name, open := range everyScreen(t) {
		m := readOnly(open, &fakeShell{})

		want := []hint{}
		for _, b := range m.screen.bindings() {
			if !b.writes && (b.offered == nil || b.offered(m)) {
				want = append(want, b.hint())
			}
		}

		// The header and help show what a key can do here. Read-only, that
		// is everything but a change to the cluster.
		r.Equal(want, m.hints(), "the %s screen", name)
	}
}

func TestReadOnly_AWriteKeyChangesNothing(t *testing.T) {
	r := require.New(t)

	t.Chdir(t.TempDir())
	t.Setenv("TMPDIR", t.TempDir())

	for name, open := range everyScreen(t) {
		for _, b := range open.screen.bindings() {
			if !b.writes {
				continue
			}

			fake, ok := open.client.(*fakeClient)
			r.True(ok)

			fake.writes = nil
			fake.logs = &nomad.LogStream{Lines: make(chan string)}
			shell := &fakeShell{}

			m, cmd := readOnly(open, shell).handleKey(keyOf(b.hint().Key))
			m = playOut(m, cmd)

			r.Empty(fake.writes, "%s on the %s screen", b.press, name)
			r.Equal(shellCommand{}, shell.opened, "%s on the %s screen", b.press, name)

			// A key that does nothing with no word said reads as a broken
			// key.
			r.Contains(plain(m.render()), "read-only", "%s on the %s screen", b.press, name)
		}
	}
}

func TestReadOnly_HelpLeavesTheWritesOut(t *testing.T) {
	r := require.New(t)

	m := readOnly(loadedModel(t), &fakeShell{})
	m, _ = m.update(key('?'))

	out := plain(m.render())
	r.Contains(out, "Describe")
	r.NotContains(out, "Start or stop")
	r.NotContains(out, "Revert")
}

func TestReadOnly_TheHeaderSaysSo(t *testing.T) {
	r := require.New(t)

	out := ansi.Strip(renderHeader(header{address: "https://nomad.example.com", readOnly: true}, 120))
	r.Contains(out, "https://nomad.example.com")
	r.Contains(out, "read-only")

	// It sits next to the cluster it is about, and a long address that is
	// cut short does not take it along.
	long := "https://nomad-servers-of-the-production-cluster.internal.example.com:4646"
	out = ansi.Strip(renderHeader(header{address: long, readOnly: true}, 120))
	r.Contains(out, "read-only")
	r.Contains(out, "https://nomad")
}

func TestReadOnly_AWritingSessionSaysNothing(t *testing.T) {
	r := require.New(t)

	out := ansi.Strip(renderHeader(header{address: "https://nomad.example.com"}, 120))
	r.NotContains(out, "read-only")
}
