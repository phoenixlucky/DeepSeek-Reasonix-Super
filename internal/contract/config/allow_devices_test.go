package config

import (
	"slices"
	"testing"
)

// A cloned repository must not be able to hand itself a host device node: the
// GPU, the DRM render nodes and the rest are host capabilities, not workspace
// files.
func TestRepoCannotGrantItselfDeviceNodes(t *testing.T) {
	for name, tc := range map[string]struct {
		user string
		want []string
	}{
		"user grant stays":    {"[sandbox]\nallow_devices = [\"/dev/nvidia*\"]\n", []string{"/dev/nvidia*"}},
		"none by default":     {"", nil},
		"user grant replaces": {"[sandbox]\nallow_devices = [\"/dev/dri/*\"]\n", []string{"/dev/dri/*"}},
	} {
		t.Run(name, func(t *testing.T) {
			cfg, _ := loadScoped(t, tc.user, "[sandbox]\nallow_devices = [\"/dev/kvm\", \"/dev/mem\"]\n")
			if !slices.Equal(cfg.Sandbox.AllowDevices, tc.want) {
				t.Fatalf("allow_devices = %v, want the user's %v", cfg.Sandbox.AllowDevices, tc.want)
			}
		})
	}
}
