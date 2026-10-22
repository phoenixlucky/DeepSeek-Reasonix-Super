//go:build !windows

package sandbox

// GrantSpec is the Windows launch preparation; elsewhere a spec needs none.
func GrantSpec(spec Spec, _ string) Spec { return spec }
