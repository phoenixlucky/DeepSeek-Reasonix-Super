//go:build windows

package sandbox

// ConfinesByDefault reports whether an enforcing spec is confined on this host
// without the user opting in. Windows has a backend, but it runs only after the
// user turns on windows_integrity, so a default configuration is not confined.
func ConfinesByDefault() bool { return false }
