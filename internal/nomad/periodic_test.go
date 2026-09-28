package nomad_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNextLaunch(t *testing.T) {
	r := require.New(t)

	client, asked := recorder(t, `{"ID": "backup", "Periodic": {"Specs": ["* * * * *"]}}`)

	before := time.Now()

	next, err := client.NextLaunch(context.Background(), "production", "backup")
	r.NoError(err)

	r.Equal("/v1/job/backup", asked.URL.Path)
	r.Equal("production", asked.URL.Query().Get("namespace"))

	// Every minute: the start of the next one.
	r.True(next.After(before), next)
	r.LessOrEqual(next.Sub(before), time.Minute)
	r.Zero(next.Second())
}

func TestNextLaunch_InTheTimeZoneOfTheJob(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `{"ID": "backup", "Periodic": {"Specs": ["0 3 * * *"], "TimeZone": "Asia/Tokyo"}}`)

	next, err := client.NextLaunch(context.Background(), "production", "backup")
	r.NoError(err)

	tokyo, err := time.LoadLocation("Asia/Tokyo")
	r.NoError(err)

	r.Equal(3, next.In(tokyo).Hour())
	r.Zero(next.In(tokyo).Minute())
}

func TestNextLaunch_None(t *testing.T) {
	for name, body := range map[string]string{
		"stopped":      `{"ID": "backup", "Stop": true, "Periodic": {"Specs": ["* * * * *"]}}`,
		"disabled":     `{"ID": "backup", "Periodic": {"Enabled": false, "Specs": ["* * * * *"]}}`,
		"not periodic": `{"ID": "backup"}`,
	} {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			client, _ := recorder(t, body)

			next, err := client.NextLaunch(context.Background(), "production", "backup")
			r.NoError(err)
			r.True(next.IsZero(), next)
		})
	}
}

func TestNextLaunch_ABrokenSchedule(t *testing.T) {
	r := require.New(t)

	client, _ := recorder(t, `{"ID": "backup", "Periodic": {"Specs": ["every tuesday"]}}`)

	_, err := client.NextLaunch(context.Background(), "production", "backup")
	r.Error(err)
}
