package config

import (
	"fmt"
	"strings"
)

// renderAllowDevices writes the device allowlist, or a commented example when
// none is set.
func renderAllowDevices(b *strings.Builder, devices []string) {
	if len(devices) > 0 {
		fmt.Fprintf(b, "allow_devices = %s   # host device nodes bound into the Linux sandbox; user-global\n", renderStringArray(devices))
		return
	}
	b.WriteString("# allow_devices = [\"/dev/nvidia*\"]   # host device nodes (GPU) bound into the Linux sandbox; user-global\n")
}
