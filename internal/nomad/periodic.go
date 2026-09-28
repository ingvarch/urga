package nomad

import (
	"context"
	"time"
)

// NextLaunch is when a periodic job launches next. It is zero for a job that
// does not launch: stopped, turned off, or not periodic.
func (c *Client) NextLaunch(ctx context.Context, namespace, jobID string) (time.Time, error) {
	job, _, err := c.api.Jobs().Info(jobID, c.query(ctx, namespace))
	if err != nil {
		return time.Time{}, err
	}

	periodic := job.Periodic
	if periodic == nil || (job.Stop != nil && *job.Stop) {
		return time.Time{}, nil
	}

	// The answer leaves out what was never set, and Next reads the spec
	// type without a check.
	periodic.Canonicalize()

	if !*periodic.Enabled {
		return time.Time{}, nil
	}

	// The schedule is read in the time zone of the job, as the cluster
	// reads it.
	zone, err := periodic.GetLocation()
	if err != nil {
		return time.Time{}, err
	}

	return periodic.Next(time.Now().In(zone))
}
