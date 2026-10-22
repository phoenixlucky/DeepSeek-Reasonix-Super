//go:build windows

package peermail

import "os"

// Windows has no owner or mode bits to read here; the state root's own ACL is
// what scopes it, and the caller reports that this platform is unauthenticated.
func ownedByUser(os.FileInfo) bool { return true }

func privateMode(os.FileInfo) bool { return true }
