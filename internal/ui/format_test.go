package ui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAge(t *testing.T) {
	tests := []struct {
		name string
		age  time.Duration
		want string
	}{
		{name: "seconds", age: 45 * time.Second, want: "45s"},
		{name: "a minute", age: 90 * time.Second, want: "1m"},
		{name: "minutes", age: 12 * time.Minute, want: "12m"},
		{name: "an hour", age: 90 * time.Minute, want: "1h"},
		{name: "hours", age: 5 * time.Hour, want: "5h"},
		// Past a day the hours stop being useful, days are what is read.
		{name: "a day", age: 25 * time.Hour, want: "1d"},
		{name: "two days", age: 49 * time.Hour, want: "2d"},
		{name: "days", age: 259 * 24 * time.Hour, want: "259d"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, age(test.age))
		})
	}
}

func TestAgeOf(t *testing.T) {
	r := require.New(t)

	r.Equal("2h", ageOf(time.Now().Add(-2*time.Hour)))

	// A job with no submit time has no age either.
	r.Equal("-", ageOf(time.Time{}))
}

func TestTruncate(t *testing.T) {
	r := require.New(t)

	r.Equal("short", truncate("short", 10))
	r.Equal("exactly10c", truncate("exactly10c", 10))

	// A long value is eaten from the right, wrapping it breaks the row.
	r.Equal("https://no…", truncate("https://nomad.example.com", 11))
	r.Equal("…", truncate("anything", 1))
	r.Equal("", truncate("anything", 0))
}
