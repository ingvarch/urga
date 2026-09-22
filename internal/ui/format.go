package ui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// age is how long ago something happened, in the one unit that reads at a
// glance.
func age(since time.Duration) string {
	switch {
	case since < time.Minute:
		return fmt.Sprintf("%ds", int(since.Seconds()))
	case since < time.Hour:
		return fmt.Sprintf("%dm", int(since.Minutes()))
	case since < 24*time.Hour:
		return fmt.Sprintf("%dh", int(since.Hours()))
	default:
		return fmt.Sprintf("%dd", int(since.Hours())/24)
	}
}

// ageOf is the age of a moment, empty when there is none.
func ageOf(moment time.Time) string {
	if moment.IsZero() {
		return "-"
	}

	return age(time.Since(moment))
}

// truncate cuts a value to the width of its column. The tail is eaten, a
// value that wraps pushes the whole row out of shape.
func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}

	return ansi.Truncate(value, width, "…")
}
