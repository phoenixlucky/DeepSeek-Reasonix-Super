//go:build !windows

package sandbox

// ConfinesByDefault reports whether an enforcing spec is confined on this host
// without the user opting in, which off Windows is whether a backend exists.
func ConfinesByDefault() bool { return Available() }
