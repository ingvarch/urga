package nomad_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestServerHealth(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{
		"/v1/operator/autopilot/health": `{
			"Healthy": false,
			"FailureTolerance": 0,
			"Servers": [
				{"ID": "a", "Name": "server-1.global", "Address": "10.0.0.1:4647", "Leader": true, "Voter": true, "Healthy": true,
				 "LastContact": "0s", "LastIndex": 120, "StableSince": "2026-09-28T10:00:00Z"},
				{"ID": "b", "Name": "server-2.global", "Address": "10.0.0.2:4647", "Voter": true, "Healthy": false,
				 "LastContact": "12.5s", "LastIndex": 95, "StableSince": "2026-09-28T11:00:00Z"}
			]
		}`,
	})

	health, err := client.ServerHealth(context.Background())
	r.NoError(err)
	r.Equal("/v1/operator/autopilot/health", (*asked)[0].path)

	r.False(health.Healthy)
	r.Zero(health.FailureTolerance)

	r.Equal([]nomad.ServerHealth{
		{
			Name: "server-1.global", Address: "10.0.0.1:4647", Leader: true, Voter: true, Healthy: true,
			LastIndex: 120, StableSince: time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC),
		},
		{
			Name: "server-2.global", Address: "10.0.0.2:4647", Voter: true,
			LastContact: 12500 * time.Millisecond, LastIndex: 95, StableSince: time.Date(2026, 9, 28, 11, 0, 0, 0, time.UTC),
		},
	}, health.Servers)
}
