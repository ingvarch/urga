package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/release"
)

// threeReleases are the notes of three releases, the oldest last.
func threeReleases() []release.Notes {
	return []release.Notes{
		{
			Version: "v0.9.0", Published: time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC),
			Changes: release.Changes{Added: []string{"an about page (#36)"}, Fixed: []string{"keep the cursor on the row (#37)"}},
		},
		{
			Version: "v0.8.0", Published: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC),
			Changes: release.Changes{Other: []string{"split the client of the cluster (#30)"}},
		},
		{
			Version: "v0.7.2", Published: time.Date(2026, 9, 28, 13, 33, 0, 0, time.UTC),
			Changes: release.Changes{Fixed: []string{"ship third-party license notices (#34)"}},
		},
	}
}

// readsReleases answers with notes and counts how often it was asked.
func readsReleases(notes []release.Notes, err error, asked *int) func(context.Context) ([]release.Notes, error) {
	return func(context.Context) ([]release.Notes, error) {
		*asked++

		return notes, err
	}
}

// aboutOf opens the about screen of a build.
func aboutOf(t *testing.T, opts Options) Model {
	t.Helper()

	m := New(&fakeClient{jobs: twoJobs()}, opts)
	m, _ = m.update(tea.WindowSizeMsg{Width: 120, Height: 60})

	return typeCommand(m, "about")
}

// released is a build of v0.7.2 installed with Homebrew.
func released(read func(context.Context) ([]release.Notes, error)) Options {
	return Options{
		Version:    "v0.7.2",
		Commit:     "adb6c04",
		Built:      time.Date(2026, 9, 28, 13, 33, 4, 0, time.UTC),
		UpdateHint: "brew upgrade ingvarch/tap/urga",
		Releases:   read,
	}
}

func TestAbout_UpToDate(t *testing.T) {
	r := require.New(t)

	asked := 0
	m := aboutOf(t, released(readsReleases(threeReleases()[2:], nil, &asked)))

	r.IsType(aboutPage{}, m.screen.page)
	r.Equal("About urga", m.title())

	// Nothing to update to: no way to update, and what came with this one.
	r.Equal([]string{
		"Version   v0.7.2",
		"Commit    adb6c04",
		"Built     2026-09-28 13:33 UTC",
		"Latest    up to date",
		"",
		"v0.7.2  2026-09-28",
		"FIXED",
		"  • ship third-party license notices (#34)",
	}, m.text.lines)
}

func TestAbout_AnUpdateIsReady(t *testing.T) {
	r := require.New(t)

	asked := 0
	m := aboutOf(t, released(readsReleases(threeReleases(), nil, &asked)))

	// What the update brings, the newest first; what this one brought is
	// behind.
	r.Equal([]string{
		"Version   v0.7.2",
		"Commit    adb6c04",
		"Built     2026-09-28 13:33 UTC",
		"Latest    v0.9.0, update ready",
		"Update    brew upgrade ingvarch/tap/urga",
		"",
		"v0.9.0  2026-10-05",
		"ADDED",
		"  • an about page (#36)",
		"FIXED",
		"  • keep the cursor on the row (#37)",
		"",
		"v0.8.0  2026-10-01",
		"OTHER",
		"  • split the client of the cluster (#30)",
	}, m.text.lines)

	// The update, the versions and the kinds of change stand out from the
	// changes.
	for _, line := range []int{3, 6, 7, 9, 12, 13} {
		r.Contains(m.text.paint, line, m.text.lines[line])
	}

	r.NotContains(m.text.paint, 8)
}

func TestAbout_WhileTheReleasesAreRead(t *testing.T) {
	r := require.New(t)

	asked := 0
	m := New(&fakeClient{jobs: twoJobs()}, released(readsReleases(threeReleases(), nil, &asked)))
	m, _ = m.update(sizeMsg())

	// The answer has not arrived yet.
	m, _ = runLine(m, "about")

	r.Contains(m.text.lines, "Latest    checking")
	r.NotContains(m.text.lines, "Update    brew upgrade ingvarch/tap/urga")
}

func TestAbout_NotChecked(t *testing.T) {
	r := require.New(t)

	opts := released(nil)
	opts.Unchecked = "URGA_NO_UPDATE_CHECK is set"

	m := aboutOf(t, opts)

	r.Equal([]string{
		"Version   v0.7.2",
		"Commit    adb6c04",
		"Built     2026-09-28 13:33 UTC",
		"Latest    not checked: URGA_NO_UPDATE_CHECK is set",
	}, m.text.lines)
}

func TestAbout_TheReleasesCannotBeRead(t *testing.T) {
	r := require.New(t)

	asked := 0
	m := aboutOf(t, released(readsReleases(nil, errors.New("releases: 403 Forbidden"), &asked)))

	r.Contains(m.text.lines, "Latest    not checked: releases: 403 Forbidden")

	// It is not the cluster's trouble: the status line stays clear.
	r.NotContains(plain(statusLine(m)), "403")
}

func TestAbout_WhatTheBuildDidNotStamp(t *testing.T) {
	r := require.New(t)

	m := aboutOf(t, Options{Version: "dev", Unchecked: "a build from source"})

	r.Equal([]string{
		"Version   dev",
		"Latest    not checked: a build from source",
	}, m.text.lines)
}

func TestAbout_AsksForTheReleasesOnce(t *testing.T) {
	r := require.New(t)

	asked := 0
	m := aboutOf(t, released(readsReleases(threeReleases(), nil, &asked)))
	r.Equal(1, asked)

	// A screen without events is polled; the releases are not asked for
	// every few seconds.
	for range 3 {
		var cmd tea.Cmd

		m, cmd = m.update(pollMsg{})
		m = playOut(m, cmd)
	}

	r.Equal(1, asked)
}

func TestAbout_OpensAsVersion(t *testing.T) {
	r := require.New(t)

	m := New(&fakeClient{jobs: twoJobs()}, Options{Version: "v0.7.2"})
	m, _ = m.update(sizeMsg())
	m = typeCommand(m, "version")

	r.IsType(aboutPage{}, m.screen.page)
}

func TestHeader_TheVersionWithoutTheCommit(t *testing.T) {
	r := require.New(t)

	m := newTestModel(&fakeClient{jobs: twoJobs()})
	m.opts.Version, m.opts.Commit = "v0.7.2", "adb6c04"

	header := headerOf(m)
	r.Contains(header, "v0.7.2")
	r.NotContains(header, "adb6c04")
}
