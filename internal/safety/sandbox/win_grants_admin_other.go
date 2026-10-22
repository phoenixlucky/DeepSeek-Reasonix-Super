//go:build !windows

package sandbox

import "errors"

// ErrNotWindows: the grant record exists only on Windows.
var ErrNotWindows = errors.New("sandbox: the grant record is a Windows feature")

// GrantedRoots lists the folders the Windows sandbox has granted; elsewhere there are none.
func GrantedRoots(string) (string, []string, error) { return "", nil, ErrNotWindows }

// RevokeGrants removes every grant the Windows sandbox made; elsewhere there are none.
func RevokeGrants(string) ([]string, map[string]error, error) { return nil, nil, ErrNotWindows }
