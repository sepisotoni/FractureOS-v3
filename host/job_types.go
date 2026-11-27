package main

// Windows job object structures (64-bit layout, identical on amd64 and arm64).
// They live in an untagged file so a test on any OS checks the sizes the Win32
// API expects: a wrong size makes SetInformationJobObject fail silently.

type jobBasicLimit struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type jobIOCounters struct {
	ReadOperationCount, WriteOperationCount, OtherOperationCount uint64
	ReadTransferCount, WriteTransferCount, OtherTransferCount    uint64
}

type jobExtendedLimit struct {
	Basic                 jobBasicLimit
	IO                    jobIOCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

const (
	jobLimitKillOnClose        = 0x2000
	jobObjectExtendedLimitInfo = 9
	processSetQuota            = 0x0100
	processTerminate           = 0x0001
)
