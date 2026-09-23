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
