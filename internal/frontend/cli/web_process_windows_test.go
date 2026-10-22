//go:build windows

package cli

import (
	"net"
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsForbiddenBindCountsAsATakenPort(t *testing.T) {
	for name, errno := range map[string]error{"in use": windows.WSAEADDRINUSE, "forbidden": windows.WSAEACCES} {
		err := &net.OpError{Op: "listen", Err: os.NewSyscallError("bind", errno)}
		if !webPortTaken(err) {
			t.Errorf("%s: a bind refused with %v was not retried on the next port", name, errno)
		}
	}
	other := &net.OpError{Op: "listen", Err: os.NewSyscallError("bind", windows.WSAEADDRNOTAVAIL)}
	if webPortTaken(other) {
		t.Error("an address that does not exist here was retried as if taken")
	}
}
