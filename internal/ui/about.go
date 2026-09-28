package ui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/release"
)

// buildInfo is this urga as the about screen shows it: what the build
// stamped in, how to update it, and how to read its releases, or why not.
type buildInfo struct {
	version, commit string
	built           time.Time

	updateHint string

	releases  func(ctx context.Context) ([]release.Notes, error)
	unchecked string
}

// releasesMsg is the notes of the releases of urga, or why they could not be
// read.
type releasesMsg struct {
	notes []release.Notes
	err   error
}

// aboutPage is this urga: its build, whether a newer one is out, and what
// the releases changed.
type aboutPage struct {
	// read says the releases answered. They are asked for once: a screen
	// without events is polled, and the releases allow few requests.
	read  bool
	notes []release.Notes
	err   error
}

var aboutKeys = textKeys[aboutPage]()

func (aboutPage) title(env, int) string    { return "About urga" }
func (aboutPage) titles() []string         { return nil }
func (aboutPage) topics() []string         { return nil }
func (aboutPage) rows(env) []tableRow      { return nil }
func (aboutPage) saveAs() (string, string) { return "about", "txt" }

func (p aboutPage) keys(e env) []keyHint { return hintsOf(p, e, aboutKeys) }

func (p aboutPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, aboutKeys, k)
}

// fetch reads the releases, unless they answered or must not be read. A
// failure is shown on the page, so it is an answer too.
func (p aboutPage) fetch(e env) tea.Cmd {
	read := e.build.releases
	if p.read || read == nil {
		return nil
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		notes, err := read(ctx)

		return releasesMsg{notes: notes, err: err}
	}
}

func (p aboutPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	answer, ok := msg.(releasesMsg)
	if !ok {
		return p, outcome{}, false
	}

	p.read, p.notes, p.err = true, answer.notes, answer.err

	return p, outcome{}, true
}

func (p aboutPage) text(e env) textContent {
	b := e.build
	lines := []paintedLine{aboutLine("Version", b.version)}

	if b.commit != "" {
		lines = append(lines, aboutLine("Commit", b.commit))
	}

	if !b.built.IsZero() {
		lines = append(lines, aboutLine("Built", b.built.UTC().Format("2006-01-02 15:04 UTC")))
	}

	shown := release.Changelog(p.notes, b.version)
	newer := len(shown) > 0 && release.Newer(b.version, shown[0].Version)

	if newer {
		lines = append(lines,
			paintedLine{text: aboutText("Latest", shown[0].Version+", update ready"), style: &styleNewer},
			aboutLine("Update", b.updateHint))
	} else {
		lines = append(lines, aboutLine("Latest", p.latest(b)))
	}

	for _, notes := range shown {
		lines = append(lines, paintedLine{})
		lines = append(lines, notesLines(notes)...)
	}

	return paintedText(lines).textContent
}

// latest is where this urga stands, when no update is ready.
func (p aboutPage) latest(b buildInfo) string {
	switch {
	case b.releases == nil:
		return "not checked: " + b.unchecked
	case p.err != nil:
		return "not checked: " + p.err.Error()
	case !p.read:
		return "checking"
	}

	return "up to date"
}

// aboutLine is a line of the build: its name in a column, then the value.
func aboutLine(name, value string) paintedLine { return paintedLine{text: aboutText(name, value)} }

func aboutText(name, value string) string { return fmt.Sprintf("%-10s%s", name, value) }

// notesLines are what one release changed, under its version, by kind. A
// kind without changes is left out.
func notesLines(notes release.Notes) []paintedLine {
	heading := notes.Version
	if !notes.Published.IsZero() {
		heading += "  " + notes.Published.UTC().Format("2006-01-02")
	}

	lines := []paintedLine{{text: heading, style: &styleTitle}}

	for _, kind := range []struct {
		name    string
		changes []string
	}{
		{"ADDED", notes.Added},
		{"FIXED", notes.Fixed},
		{"OTHER", notes.Other},
	} {
		if len(kind.changes) == 0 {
			continue
		}

		lines = append(lines, paintedLine{text: kind.name, style: &styleLabel})

		for _, change := range kind.changes {
			lines = append(lines, paintedLine{text: "  • " + change})
		}
	}

	return lines
}
