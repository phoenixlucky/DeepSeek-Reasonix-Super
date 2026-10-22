package websearch

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"slices"
)

// errAddressRefused is an address the probe will not connect to. The cause is the
// address's own class, not its spelling, so a hostname that resolves to it is
// refused the same way a literal is.
var errAddressRefused = errors.New("address is not one a search instance is reached at")

// metadataAddrs are the cloud instance-metadata endpoints that sit outside the
// link-local block: Alibaba's and AWS's IPv6 one.
var metadataAddrs = []netip.Addr{
	netip.MustParseAddr("100.100.100.200"),
	netip.MustParseAddr("fd00:ec2::254"),
}

// forbiddenAddr reports an address that is never a search instance: link-local
// (which holds the 169.254.169.254 metadata endpoint), the unspecified and
// "this network" ranges, multicast, broadcast, and the other metadata endpoints.
// Loopback and private ranges are allowed: a self-hosted instance lives there.
func forbiddenAddr(a netip.Addr) bool {
	a = a.Unmap()
	switch {
	case !a.IsValid(), a.IsUnspecified(), a.IsLinkLocalUnicast(), a.IsLinkLocalMulticast(), a.IsMulticast(), a.IsInterfaceLocalMulticast():
		return true
	case a.Is4() && (a.As4()[0] == 0 || a == netip.AddrFrom4([4]byte{255, 255, 255, 255})):
		return true
	}
	return slices.Contains(metadataAddrs, a)
}

type lookupFunc func(ctx context.Context, host string) ([]netip.Addr, error)

func systemLookup(ctx context.Context, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

// guardedDial resolves the host once, refuses it if any answer is forbidden, and
// connects to the first answer by address. Connecting by the checked address is
// what keeps a second, different DNS answer from being used.
func guardedDial(lookup lookupFunc) func(ctx context.Context, network, address string) (net.Conn, error) {
	var d net.Dialer
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addrs, err := lookup(ctx, host)
		if err != nil {
			return nil, err
		}
		if len(addrs) == 0 {
			return nil, errAddressRefused
		}
		if slices.ContainsFunc(addrs, forbiddenAddr) {
			return nil, errAddressRefused
		}
		return d.DialContext(ctx, network, net.JoinHostPort(addrs[0].Unmap().String(), port))
	}
}
