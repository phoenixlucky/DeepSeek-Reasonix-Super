//go:build !windows

package sandbox

// namesAnAccount has no account database to ask off Windows, where no SID is
// ever granted; the shape test alone applies.
func namesAnAccount(string) bool { return false }
