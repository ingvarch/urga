package version_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/version"
)

func TestHuman(t *testing.T) {
	r := require.New(t)

	// A build from source shows as dev instead of as a release.
	r.Equal("dev", version.Human("dev", ""))
	r.Equal("v0.1.0", version.Human("v0.1.0", ""))
	r.Equal("v0.1.0 (a1b2c3d)", version.Human("v0.1.0", "a1b2c3d"))
	r.Equal("dev (a1b2c3d)", version.Human("dev", "a1b2c3d"))

	// "none" is what the build stamps when there is no git to ask.
	r.Equal("dev", version.Human("dev", "none"))
	r.Equal("dev", version.Human("", ""))
}

func TestLong(t *testing.T) {
	r := require.New(t)

	// The line urga prints for --version says what was built and when, which
	// is what the build stamps in.
	r.Equal("v0.1.0 (a1b2c3d), built 2026-09-22T21:35:21Z",
		version.Long("v0.1.0", "a1b2c3d", "2026-09-22T21:35:21Z"))

	// A build from source shows no date.
	r.Equal("dev", version.Long("dev", "none", "unknown"))
	r.Equal("dev", version.Long("", "", ""))
}
