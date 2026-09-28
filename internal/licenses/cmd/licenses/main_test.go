package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRun_WritesTheNoticesOfUrga(t *testing.T) {
	r := require.New(t)

	notices := filepath.Join(t.TempDir(), "THIRD_PARTY_NOTICES")
	r.NoError(run(t.Context(), notices, []string{"github.com/ingvarch/urga/cmd/urga"}))

	data, err := os.ReadFile(notices)
	r.NoError(err)

	// The Nomad client is MPL-2.0, which asks to tell where its source is.
	r.Contains(string(data), "Third-party notices for urga")
	r.Regexp(`(?m)^github\.com/hashicorp/nomad/api v\S+\nLicenses: MPL-2\.0\nSource: https://proxy\.golang\.org/`,
		string(data))
}
