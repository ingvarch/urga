//go:build unix

package main

import (
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A closed terminal sends SIGHUP: the run ends the way it ends on q, and
// removes what it wrote.
func TestUntilHangUp_EndsOnSIGHUP(t *testing.T) {
	ctx, stop := untilHangUp()
	defer stop()

	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGHUP))

	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		require.FailNow(t, "SIGHUP did not end the context")
	}
}
