// Package version is what the build stamps into the binary.
package version

import (
	"fmt"
	"time"
)

// What a build shows when the linker stamped nothing in.
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

// Full is this build with the date it was made, the way --version prints it.
func Full() string {
	return Long(Version, Commit, Date)
}

// Human formats a version and a commit as one line.
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

// StampedCommit is the commit the build stamped in, empty when it stamped
// none.
func StampedCommit() string {
	if Commit == noCommit {
		return ""
	}

	return Commit
}

// StampedDate is when the build was made, zero when it stamped no date.
func StampedDate() time.Time {
	date, err := time.Parse(time.RFC3339, Date)
	if err != nil {
		return time.Time{}
	}

	return date
}
