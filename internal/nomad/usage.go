package nomad

// ResourceUse is what something takes right now, next to what it is allowed.
type ResourceUse struct {
	CPUTicks        int
	CPUTicksAllowed int
	CPUPercent      int

	MemoryMB        int
	MemoryMBAllowed int
	MemoryPercent   int
}

// Known says whether the cluster answered with anything.
func (u ResourceUse) Known() bool {
	return u.CPUTicksAllowed > 0 || u.MemoryMBAllowed > 0 || u.CPUTicks > 0 || u.MemoryMB > 0
}
