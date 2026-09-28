package release_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/release"
)

// generated is the text a release gets from the pull requests merged for it.
const generated = `## What's Changed
* feat(ui): show the launches of periodic jobs by @ingvarch in https://github.com/ingvarch/urga/pull/35
* fix(release): ship third-party license notices by @ingvarch in https://github.com/ingvarch/urga/pull/34
* feat: an about page by @someone in https://github.com/ingvarch/urga/pull/36
* refactor(ui)!: make every screen a type of its own by @ingvarch in https://github.com/ingvarch/urga/pull/31
* Update README by @someone in https://github.com/ingvarch/urga/pull/12

## New Contributors
* @someone made their first contribution in https://github.com/ingvarch/urga/pull/36


**Full Changelog**: https://github.com/ingvarch/urga/compare/v0.7.1...v0.7.2
`

func TestParseChanges(t *testing.T) {
	r := require.New(t)

	changes := release.ParseChanges(generated)

	// The kind of a change is the type of its title; the title reads
	// without it, and the pull request is its number.
	r.Equal([]string{"show the launches of periodic jobs (#35)", "an about page (#36)"}, changes.Added)
	r.Equal([]string{"ship third-party license notices (#34)"}, changes.Fixed)

	// Any other type, or none, is another change.
	r.Equal([]string{"make every screen a type of its own (#31)", "Update README (#12)"}, changes.Other)
}

func TestParseChanges_LinesEndedTheWindowsWay(t *testing.T) {
	r := require.New(t)

	changes := release.ParseChanges(strings.ReplaceAll(generated, "\n", "\r\n"))

	r.Equal([]string{"ship third-party license notices (#34)"}, changes.Fixed)
	r.Len(changes.Other, 2)
}

func TestParseChanges_WrittenByHand(t *testing.T) {
	r := require.New(t)

	// No heading, dashes, and no pull request to name.
	changes := release.ParseChanges("- fix(ui): keep the cursor on the row\n- a note\n\nThanks!")

	r.Equal([]string{"keep the cursor on the row"}, changes.Fixed)
	r.Equal([]string{"a note"}, changes.Other)
	r.Empty(changes.Added)
}

func TestParseChanges_Nothing(t *testing.T) {
	r := require.New(t)

	changes := release.ParseChanges("")

	r.Empty(changes.Added)
	r.Empty(changes.Fixed)
	r.Empty(changes.Other)
}

func TestReleases(t *testing.T) {
	r := require.New(t)

	url, asked := releases(t, http.StatusOK, `[
		{"tag_name": "v0.8.0", "published_at": "2026-10-01T10:00:00Z",
		 "body": "## What's Changed\n* feat(ui): an about page by @ingvarch in https://github.com/ingvarch/urga/pull/36\n"},
		{"tag_name": "v0.8.0-rc.1", "prerelease": true, "body": "* feat: soon"},
		{"tag_name": "v0.7.3", "draft": true, "body": "* feat: not yet"},
		{"tag_name": "nightly", "body": "* feat: every night"},
		{"tag_name": "v0.7.2", "published_at": "2026-09-28T13:33:04Z",
		 "body": "* fix(release): ship third-party license notices by @ingvarch in https://github.com/ingvarch/urga/pull/34"}
	]`)

	notes, err := release.Releases(context.Background(), http.DefaultClient, url)
	r.NoError(err)

	r.Equal("/repos/ingvarch/urga/releases", asked.URL.Path)
	r.Equal("30", asked.URL.Query().Get("per_page"))
	r.Equal("application/vnd.github+json", asked.Header.Get("Accept"))

	// Drafts, pre-releases and tags that are not a version are not releases
	// anyone updates to.
	r.Len(notes, 2)

	r.Equal("v0.8.0", notes[0].Version)
	r.Equal(time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC), notes[0].Published.UTC())
	r.Equal([]string{"an about page (#36)"}, notes[0].Added)

	r.Equal("v0.7.2", notes[1].Version)
	r.Equal([]string{"ship third-party license notices (#34)"}, notes[1].Fixed)
}

func TestReleases_Refused(t *testing.T) {
	r := require.New(t)

	url, _ := releases(t, http.StatusForbidden, `{"message": "API rate limit exceeded"}`)

	_, err := release.Releases(context.Background(), http.DefaultClient, url)
	r.ErrorContains(err, "403")
}

// versions are the versions of a list of notes, in its order.
func versions(notes []release.Notes) []string {
	out := []string{}
	for _, n := range notes {
		out = append(out, n.Version)
	}

	return out
}

func TestChangelog(t *testing.T) {
	r := require.New(t)

	all := []release.Notes{{Version: "v0.7.2"}, {Version: "v0.9.0"}, {Version: "v0.8.0"}, {Version: "v0.7.1"}}

	// Behind: what the update brings, the newest first, in any order the
	// releases were listed in.
	r.Equal([]string{"v0.9.0", "v0.8.0"}, versions(release.Changelog(all, "v0.7.2")))

	// Up to date: what came with this one.
	r.Equal([]string{"v0.9.0"}, versions(release.Changelog(all, "v0.9.0")))

	// A version the list does not hold, and a build from source, have no
	// notes of their own.
	r.Empty(release.Changelog(all, "v0.9.1"))
	r.Empty(release.Changelog(all, "v0.7.2-1-gadb6c04-dirty"))
}
