package main

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/require"
)

// flagsFrom parses a command line with a namespace flag whose default came
// from the environment.
func flagsFrom(t *testing.T, args ...string) *flag.FlagSet {
	t.Helper()

	flags := flag.NewFlagSet("urga", flag.ContinueOnError)
	flags.String("namespace", "from-env", "")
	require.NoError(t, flags.Parse(args))

	return flags
}

func TestGiven(t *testing.T) {
	r := require.New(t)

	// A default is not a choice: it comes from the environment of every run.
	r.False(given(flagsFrom(t), "namespace"))

	r.True(given(flagsFrom(t, "-namespace", "production"), "namespace"))

	// Naming the default is a choice all the same.
	r.True(given(flagsFrom(t, "-namespace", "from-env"), "namespace"))
}
