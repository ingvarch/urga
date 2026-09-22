package version_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/version"
)

func TestHuman(t *testing.T) {
	r := require.New(t)

	// A build from source says so instead of pretending to be a release.
	r.Equal("dev", version.Human("dev", ""))
	r.Equal("v0.1.0", version.Human("v0.1.0", ""))
	r.Equal("v0.1.0 (a1b2c3d)", version.Human("v0.1.0", "a1b2c3d"))
	r.Equal("dev (a1b2c3d)", version.Human("dev", "a1b2c3d"))

	// "none" is what the build stamps when there is no git to ask.
	r.Equal("dev", version.Human("dev", "none"))
	r.Equal("dev", version.Human("", ""))
}
