package release_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/release"
)

func TestIsRelease(t *testing.T) {
	r := require.New(t)

	r.True(release.IsRelease("v0.5.1"))
	r.True(release.IsRelease("v1.12.0"))

	// Builds from source say how far past a release they are, or nothing.
	r.False(release.IsRelease("dev"))
	r.False(release.IsRelease(""))
	r.False(release.IsRelease("v0.5.0-3-gabc1234"))
	r.False(release.IsRelease("v0.5.0-3-gabc1234-dirty"))
	r.False(release.IsRelease("v0.6.0-rc.1"))
	r.False(release.IsRelease("0.5.1"))
}

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		current, latest string
		newer           bool
	}{
		{"v0.5.0", "v0.5.1", true},
		{"v0.5.1", "v0.6.0", true},
		{"v0.9.9", "v1.0.0", true},
		{"v0.5.1", "v0.5.1", false},
		{"v0.5.1", "v0.5.0", false},

		// Each part only counts when the ones before it are equal.
		{"v1.0.0", "v0.9.9", false},
		{"v0.6.0", "v0.5.9", false},

		// Numbers, not text: 10 comes after 9.
		{"v0.9.0", "v0.10.0", true},
		{"v0.10.0", "v0.9.0", false},

		// Nothing to compare.
		{"dev", "v0.5.1", false},
		{"v0.5.0-3-gabc1234", "v0.5.1", false},
		{"v0.5.0", "", false},
		{"v0.5.0", "nightly", false},
	} {
		require.Equal(t, c.newer, release.Newer(c.current, c.latest), "%s then %s", c.current, c.latest)
	}
}

// published answers as the releases of the repository do, and records the
// request.
func published(t *testing.T, status int, body string) (string, *http.Request) {
	t.Helper()

	asked := &http.Request{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		*asked = *req

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	return server.URL + "/repos/ingvarch/urga/releases/latest", asked
}

func TestLatest(t *testing.T) {
	r := require.New(t)

	url, asked := published(t, http.StatusOK, `{"tag_name": "v0.5.1", "name": "urga 0.5.1", "prerelease": false}`)

	latest, err := release.Latest(context.Background(), http.DefaultClient, url)
	r.NoError(err)

	// The tag, which is what a version is compared with: a name is free text.
	r.Equal("v0.5.1", latest)
	r.Equal("/repos/ingvarch/urga/releases/latest", asked.URL.Path)
	r.Equal("application/vnd.github+json", asked.Header.Get("Accept"))
}

func TestLatest_Refused(t *testing.T) {
	r := require.New(t)

	// What an address that asked too often gets.
	url, _ := published(t, http.StatusForbidden, `{"message": "API rate limit exceeded"}`)

	_, err := release.Latest(context.Background(), http.DefaultClient, url)
	r.ErrorContains(err, "403")
}

func TestNewerThan(t *testing.T) {
	r := require.New(t)

	url, _ := published(t, http.StatusOK, `{"tag_name": "v0.5.1"}`)

	newer, err := release.NewerThan(context.Background(), http.DefaultClient, url, "v0.5.0")
	r.NoError(err)
	r.Equal("v0.5.1", newer)

	newer, err = release.NewerThan(context.Background(), http.DefaultClient, url, "v0.5.1")
	r.NoError(err)
	r.Empty(newer)
}
