package release

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Changes are what a release changed, by kind.
type Changes struct {
	Added, Fixed, Other []string
}

var (
	// entry is an item of a list, with a star or a dash.
	entry = regexp.MustCompile(`^[*-]\s+(.+)$`)

	// title is a conventional title: its type, a scope, and a ! for a
	// breaking change.
	title = regexp.MustCompile(`^(\w+)(\([^)]*\))?!?:\s*(.+)$`)

	// by is what generated notes add after a title: who made the change,
	// and its pull request.
	by = regexp.MustCompile(`\s+by @\S+ in \S+/pull/(\d+)$`)
)

// newContributors is the heading generated notes greet new authors under:
// those items are people, not changes.
const newContributors = "New Contributors"

// ParseChanges reads what a release changed out of its text.
func ParseChanges(body string) Changes {
	var changes Changes

	greeting := false

	for line := range strings.Lines(body) {
		line = strings.TrimSpace(line)

		if heading, ok := strings.CutPrefix(line, "#"); ok {
			greeting = strings.TrimSpace(strings.TrimLeft(heading, "#")) == newContributors

			continue
		}

		if item := entry.FindStringSubmatch(line); item != nil && !greeting {
			changes.add(item[1])
		}
	}

	return changes
}

// add files a change by the type of its title, with the title read without
// the type and the pull request shortened to its number.
func (c *Changes) add(text string) {
	if pull := by.FindStringSubmatch(text); pull != nil {
		text = strings.TrimSuffix(text, pull[0]) + " (#" + pull[1] + ")"
	}

	kind := ""
	if typed := title.FindStringSubmatch(text); typed != nil {
		kind, text = typed[1], typed[3]
	}

	switch kind {
	case "feat":
		c.Added = append(c.Added, text)
	case "fix":
		c.Fixed = append(c.Fixed, text)
	default:
		c.Other = append(c.Other, text)
	}
}

// Notes are what one release changed.
type Notes struct {
	Version   string
	Published time.Time

	Changes
}

// ReleasesURL are the releases of urga.
const ReleasesURL = "https://api.github.com/repos/ingvarch/urga/releases"

// listed is how many releases are asked for: someone that many behind sees
// the newest of them.
const listed = 30

// Releases reads the notes of the releases at url. Drafts, pre-releases and
// tags that are not a version are left out: nobody updates to them.
func Releases(ctx context.Context, client *http.Client, url string) ([]Notes, error) {
	var answer []struct {
		Tag        string    `json:"tag_name"`
		Body       string    `json:"body"`
		Published  time.Time `json:"published_at"`
		Draft      bool      `json:"draft"`
		Prerelease bool      `json:"prerelease"`
	}

	if err := get(ctx, client, fmt.Sprintf("%s?per_page=%d", url, listed), "releases", &answer); err != nil {
		return nil, err
	}

	notes := make([]Notes, 0, len(answer))

	for _, r := range answer {
		if r.Draft || r.Prerelease || !IsRelease(r.Tag) {
			continue
		}

		notes = append(notes, Notes{Version: r.Tag, Published: r.Published, Changes: ParseChanges(r.Body)})
	}

	return notes, nil
}

// Changelog is what someone on current reads: the releases after it, the
// newest first, or, with none, the notes of current itself.
func Changelog(all []Notes, current string) []Notes {
	newer := []Notes{}

	for _, n := range all {
		if Newer(current, n.Version) {
			newer = append(newer, n)
		}
	}

	if len(newer) > 0 {
		// The newest first.
		slices.SortFunc(newer, func(a, b Notes) int {
			x, _ := parts(a.Version)
			y, _ := parts(b.Version)

			return slices.Compare(y, x)
		})

		return newer
	}

	for _, n := range all {
		if n.Version == current {
			return []Notes{n}
		}
	}

	return nil
}
