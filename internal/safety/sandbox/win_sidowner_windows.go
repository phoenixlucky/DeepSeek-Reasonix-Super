//go:build windows

package sandbox

import (
	"errors"

	"golang.org/x/sys/windows"
)

// namesAnAccount reports whether the SID resolves to an account on this machine
// or its domain. Only "no mapping" counts as unowned: any other failure leaves
// the question open, and an open question is answered as owned.
func namesAnAccount(sidText string) bool {
	sid, err := windows.StringToSid(sidText)
	if err != nil {
		return true
	}
	_, _, _, err = sid.LookupAccount("")
	return !errors.Is(err, windows.ERROR_NONE_MAPPED)
}
