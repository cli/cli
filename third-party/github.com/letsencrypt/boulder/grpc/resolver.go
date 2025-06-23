package grpc

import (
	"fmt"
	"net"
	"strings"

	"google.golang.org/grpc/resolver"
)

// staticBuilder implements the `resolver.Builder` interface.
type staticBuilder struct{}

// newStaticBuilder creates a `staticBuilder` used to construct static DNS
// resolvers.
func newStaticBuilder() resolver.Builder {
	return &staticBuilder{}
}

// Build implements the `resolver.Builder` interface and is usually called by
// the gRPC dialer. It takes a target containing a comma separated list of
// IPv4/6 addresses and a `resolver.ClientConn` and returns a `staticResolver`
// which implements the `resolver.Resolver` interface.
func (sb *staticBuilder) Build(target resolver.Target, cc resolver.ClientConn, _ resolver.BuildOptions) (resolver.Resolver, error) {
	var resolverAddrs []resolver.Address
	for _, address := range strings.Split(target.Endpoint(), ",") {
		parsedAddress, err := parseResolverIPAddress(address)
		if err != nil {
			return nil, err
		}
		resolverAddrs = append(resolverAddrs, *parsedAddress)
	}
	r, err := newStaticResolver(cc, resolverAddrs)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// Scheme returns the scheme that `staticBuilder` will be registered for, for
// example: `static:///`.
func (sb *staticBuilder) Scheme() string {
	return "static"
}

// staticResolver is used to wrap an inner `resolver.ClientConn` and implements
// the `resolver.Resolver` interface.
type staticResolver struct {
	cc resolver.ClientConn
}

// newStaticResolver takes a `resolver.ClientConn` and a list of
// `resolver.Addresses`. It updates the state of the `resolver.ClientConn` with
// the provided addresses and returns a `staticResolver` which wraps the
// `resolver.ClientConn` and implements the `resolver.Resolver` interface.
func newStaticResolver(cc resolver.ClientConn, resolverAddrs []resolver.Address) (resolver.Resolver, error) {
	err := cc.UpdateState(resolver.State{Addresses: resolverAddrs})
	if err != nil {
		return nil, err
	}
	return &staticResolver{cc: cc}, nil
}

// ResolveNow is a no-op necessary for `staticResolver` to implement the
// `resolver.Resolver` interface. This resolver is constructed once by
// staticBuilder.Build and the state of the inner `resolver.ClientConn` is never
// updated.
func (sr *staticResolver) ResolveNow(_ resolver.ResolveNowOptions) {}

// Close is a no-op necessary for `staticResolver` to implement the
// `resolver.Resolver` interface.
func (sr *staticResolver) Close() {}

// parseResolverIPAddress takes an IPv4/6 address (ip:port, [ip]:port, or :port)
// and returns a properly formatted `resolver.Address` object. The `Addr` and
// `ServerName` fields of the returned `resolver.Address` will both be set to
// host:port or [host]:port if the host is an IPv6 address.
func parseResolverIPAddress(addr string) (*resolver.Address, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("splitting host and port for address %q: %w", addr, err)
	}
	if port == "" {
		// If the port field is empty the address ends with colon (e.g.
		// "[::1]:").
		return nil, fmt.Errorf("address %q missing port after port-separator colon", addr)
	}
	if host == "" {
		// Address only has a port (i.e ipv4-host:port, [ipv6-host]:port,
		// host-name:port). Keep consistent with net.Dial(); if the host is
		// empty (e.g. :80), the local system is assumed.
		host = "127.0.0.1"
	}
	if net.ParseIP(host) == nil {
		// Host is a DNS name or an IPv6 address without brackets.
		return nil, fmt.Errorf("address %q is not an IP address", addr)
	}
	parsedAddr := net.JoinHostPort(host, port)
	return &resolver.Address{
		Addr:       parsedAddr,
		ServerName: parsedAddr,
	}, nil
}

// init registers the `staticBuilder` with the gRPC resolver registry.
func init() {
	resolver.Register(newStaticBuilder())
}
