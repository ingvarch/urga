package ui

// fault is a state of a resource that needs attention: what a line of the
// overview counts, and what a list opened from it keeps.
type fault int

const (
	noFault fault = iota
	faultDead
	faultFailedOrLost
	faultRestarting
	faultOOMKilled
	faultDown
	faultDisconnected
	faultDraining
	faultIneligible
	faultBlocked
	faultFailed
	faultPaused
	faultRunning
)

// String is the state as a title and a line of the overview say it.
func (f fault) String() string {
	switch f {
	case faultDead:
		return "dead"
	case faultFailedOrLost:
		return "failed or lost"
	case faultRestarting:
		return "restarting"
	case faultOOMKilled:
		return "OOM killed"
	case faultDown:
		return "down"
	case faultDisconnected:
		return "disconnected"
	case faultDraining:
		return "draining"
	case faultIneligible:
		return "ineligible"
	case faultBlocked:
		return "blocked"
	case faultFailed:
		return "failed"
	case faultPaused:
		return "paused"
	case faultRunning:
		return "running"
	}

	return ""
}

// after is the state as a title carries it after the namespace.
func (f fault) after() string {
	if f == noFault {
		return ""
	}

	return ", " + f.String()
}

// newestOfEachJob is the newest item of each job, by the job key and the
// order newer gives.
func newestOfEachJob[T any](items []T, job func(T) string, newer func(a, b T) bool) map[string]T {
	newest := map[string]T{}

	for _, item := range items {
		key := job(item)
		if seen, ok := newest[key]; !ok || newer(item, seen) {
			newest[key] = item
		}
	}

	return newest
}
