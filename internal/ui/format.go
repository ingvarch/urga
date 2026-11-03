package ui

import (
	"fmt"
	"strings"
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

// exactly is a duration as it was set, without zero units at its end:
// 1h30m, 1h, 45s.
func exactly(d time.Duration) string {
	text := d.String()

	for _, zero := range []struct{ unit, tail string }{{"m0s", "0s"}, {"h0m", "0m"}} {
		if strings.HasSuffix(text, zero.unit) {
			text = strings.TrimSuffix(text, zero.tail)
		}
	}

	return text
}

// ageOf is the age of a moment, a dash when there is none.
func ageOf(moment, now time.Time) string {
	if moment.IsZero() {
		return "-"
	}

	return age(now.Sub(moment))
}

// truncate cuts a value to the width of its column. The tail is cut off: a
// value that wraps breaks the layout of the whole row.
func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}

	return ansi.Truncate(value, width, "…")
}

// sprintf is fmt.Sprintf, named short because the titles read better that
// way.
func sprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }
