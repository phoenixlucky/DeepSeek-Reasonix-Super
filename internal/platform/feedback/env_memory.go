package feedback

import "runtime"

// ProcessMemory is one row of the memory a report carries: a process type from
// a fixed list and two sizes. It names no path, process id, command line or
// user, so a number can be sent without reading it.
type ProcessMemory struct {
	Process      string `json:"process"`
	WorkingSetKB int64  `json:"workingSetKB"`
	PrivateKB    int64  `json:"privateKB"`
}

const (
	maxMemoryRows = 12
	maxMemoryKB   = 1 << 31
)

// shellProcess lists the types a shell may name. "kernel" is not among them:
// that row is read here, from this process, and nothing a caller sends can
// stand in for it.
var shellProcess = map[string]bool{"browser": true, "renderer": true, "gpu": true, "utility": true}

// memoryRows is what the window's shell said about its processes, cut to the
// types it may name and the sizes to 0..2^31, followed by the kernel's own row:
// the bytes it has taken from the system and the Go heap in use.
func memoryRows(shell []ProcessMemory) []ProcessMemory {
	out := make([]ProcessMemory, 0, min(len(shell)+1, maxMemoryRows))
	for _, r := range shell {
		if len(out) == maxMemoryRows-1 {
			break
		}
		if !shellProcess[r.Process] {
			continue
		}
		out = append(out, ProcessMemory{Process: r.Process, WorkingSetKB: clampKB(r.WorkingSetKB), PrivateKB: clampKB(r.PrivateKB)})
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return append(out, ProcessMemory{Process: "kernel", WorkingSetKB: clampKB(int64(m.Sys >> 10)), PrivateKB: clampKB(int64(m.HeapInuse >> 10))})
}

func clampKB(v int64) int64 {
	return max(0, min(v, maxMemoryKB))
}
