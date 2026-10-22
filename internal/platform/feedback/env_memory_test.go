package feedback

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMemoryRowsKeepOnlyFixedTypesAndNumbers(t *testing.T) {
	env := CollectEnv(EnvContext{Surface: SurfaceStudio, Shell: []ProcessMemory{
		{Process: "browser", WorkingSetKB: 120000, PrivateKB: 90000},
		{Process: "renderer", WorkingSetKB: 3992908, PrivateKB: 3900000},
		{Process: "gpu", WorkingSetKB: 76000, PrivateKB: 60000},
		{Process: `C:\Users\someone\AppData\Reasonix Studio.exe`, WorkingSetKB: 1, PrivateKB: 1},
		{Process: "renderer --user-data-dir=/home/someone", WorkingSetKB: 1, PrivateKB: 1},
		{Process: "kernel", WorkingSetKB: 999, PrivateKB: 999},
		{Process: "utility", WorkingSetKB: -5, PrivateKB: 1 << 40},
	}})
	var seen []string
	for _, r := range env.Memory {
		seen = append(seen, r.Process)
	}
	if got := strings.Join(seen, ","); got != "browser,renderer,gpu,utility,kernel" {
		t.Fatalf("rows = %q, want the four shell types the shell is allowed to name, then the kernel's own", got)
	}
	last := env.Memory[len(env.Memory)-1]
	if last.Process != "kernel" || last.WorkingSetKB <= 0 {
		t.Fatalf("the kernel row is filled from the runtime, not from the shell: %+v", last)
	}
	utility := env.Memory[3]
	if utility.WorkingSetKB != 0 || utility.PrivateKB != maxMemoryKB {
		t.Fatalf("numbers are clamped to 0..2^31: %+v", utility)
	}
}

func TestMemoryRowsAreBounded(t *testing.T) {
	var shell []ProcessMemory
	for range 40 {
		shell = append(shell, ProcessMemory{Process: "utility", WorkingSetKB: 1, PrivateKB: 1})
	}
	if n := len(CollectEnv(EnvContext{Shell: shell}).Memory); n != maxMemoryRows {
		t.Fatalf("%d rows, want at most %d including the kernel's", n, maxMemoryRows)
	}
}

func TestEnvJSONNamesNoPathOrProcessDetail(t *testing.T) {
	env := CollectEnv(EnvContext{Surface: SurfaceStudio, Shell: []ProcessMemory{{Process: "renderer", WorkingSetKB: 5, PrivateKB: 4}}})
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	var shape struct {
		Memory []map[string]any `json:"memory"`
	}
	if err := json.Unmarshal(raw, &shape); err != nil {
		t.Fatal(err)
	}
	for _, row := range shape.Memory {
		if len(row) != 3 || row["process"] == nil || row["workingSetKB"] == nil || row["privateKB"] == nil {
			t.Fatalf("a row must be exactly process, workingSetKB and privateKB: %v", row)
		}
	}
	for _, bad := range []string{`\\`, "/Users", "pid", "cmdline", "name"} {
		if strings.Contains(strings.ToLower(string(raw[strings.Index(string(raw), `"memory"`):])), bad) {
			t.Fatalf("the memory block names %q: %s", bad, raw)
		}
	}
}

func TestNoShellRowsStillReportsTheKernelOnly(t *testing.T) {
	env := CollectEnv(EnvContext{Surface: SurfaceTUI})
	if len(env.Memory) != 1 || env.Memory[0].Process != "kernel" {
		t.Fatalf("a frontend with no shell reports the kernel alone: %+v", env.Memory)
	}
}
