// Package version is what the build stamps into the binary.
package version

import "fmt"

// What a build says when the linker stamped nothing in.
const (
	noVersion = "dev"
	noCommit  = "none"
	noDate    = "unknown"
)

// Set by the linker, see the Makefile.
var (
	Version = noVersion
	Commit  = noCommit
	Date    = noDate
)

// Current is this build, the way the header shows it.
func Current() string {
	return Human(Version, Commit)
}

// Full is this build with the date it was made, the way --version prints it.
func Full() string {
	return Long(Version, Commit, Date)
}

// Human reads a version and a commit as one line.
func Human(version, commit string) string {
	if version == "" {
		version = noVersion
	}

	if commit == "" || commit == noCommit {
		return version
	}

	return fmt.Sprintf("%s (%s)", version, commit)
}

// Long adds the date the build was made, when there is one.
func Long(version, commit, date string) string {
	built := Human(version, commit)

	if date == "" || date == noDate {
		return built
	}

	return fmt.Sprintf("%s, built %s", built, date)
}
