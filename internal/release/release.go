// Package release checks whether a newer release of urga is published.
package release

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
)

// LatestURL is the latest release of urga. Drafts and pre-releases are not
// in it.
const LatestURL = "https://api.github.com/repos/ingvarch/urga/releases/latest"

// tag matches a release. A build from source adds a suffix to it, like
// v0.5.0-3-gabc1234-dirty.
var tag = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)

// IsRelease says a version is one that was released.
func IsRelease(version string) bool {
	return tag.MatchString(version)
}

// Newer says latest is a release that came after current. Each part counts
// only when the ones before it are equal, and as a number.
func Newer(current, latest string) bool {
	now, ok := parts(current)
	if !ok {
		return false
	}

	next, ok := parts(latest)
	if !ok {
		return false
	}

	return slices.Compare(next, now) > 0
}

// parts are the major, minor and patch of a release.
func parts(version string) ([]int, bool) {
	match := tag.FindStringSubmatch(version)
	if match == nil {
		return nil, false
	}

	out := make([]int, 0, 3)

	// Digits only; a number too long for an int reads as the largest one,
	// which still sorts after the rest.
	for _, part := range match[1:] {
		n, _ := strconv.Atoi(part)
		out = append(out, n)
	}

	return out, true
}

// Latest asks url for the tag of the latest release: the name of a release
// is free text, the tag is the version.
func Latest(ctx context.Context, client *http.Client, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("latest release: %s", resp.Status)
	}

	var latest struct {
		Tag string `json:"tag_name"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&latest); err != nil {
		return "", fmt.Errorf("latest release: %w", err)
	}

	return latest.Tag, nil
}

// NewerThan is the latest release when it came after current, and empty
// when it did not.
func NewerThan(ctx context.Context, client *http.Client, url, current string) (string, error) {
	latest, err := Latest(ctx, client, url)
	if err != nil || !Newer(current, latest) {
		return "", err
	}

	return latest, nil
}
