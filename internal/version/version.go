// Package version is what the build stamps into the binary.
package version

import "fmt"

// Set by the linker, see the Makefile.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// Current is this build, the way the header shows it.
func Current() string {
	return Human(Version, Commit)
}

// Human reads a version and a commit as one line.
func Human(version, commit string) string {
	if version == "" {
		version = "dev"
	}

	if commit == "" || commit == "none" {
		return version
	}

	return fmt.Sprintf("%s (%s)", version, commit)
}
