package noncebalancer

import (
	"errors"

	"github.com/letsencrypt/boulder/nonce"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// Name is the name used to register the nonce balancer with the gRPC
	// runtime.
	Name = "nonce"

	// SRVResolverScheme is the scheme used to invoke an instance of the SRV
	// resolver which will use the noncebalancer to pick backends. It would be
	// ideal to export this from the SRV resolver package but that package is
	// internal.
	SRVResolverScheme = "nonce-srv"
)

// ErrNoBackendsMatchPrefix indicates that no backends were found which match
// the nonce prefix provided in the RPC context. This can happen when the
// provided nonce is stale, valid but the backend has since been removed from
// the balancer, or valid but the backend has not yet been added to the
// balancer.
//
// In any case, when the WFE receives this error it will return a badNonce error
// to the ACME client.
var ErrNoBackendsMatchPrefix = status.New(codes.Unavailable, "no backends match the nonce prefix")
var errMissingPrefixCtxKey = errors.New("nonce.PrefixCtxKey value required in RPC context")
var errMissingHMACKeyCtxKey = errors.New("nonce.HMACKeyCtxKey value required in RPC context")
var errInvalidPrefixCtxKeyType = errors.New("nonce.PrefixCtxKey value in RPC context must be a string")
var errInvalidHMACKeyCtxKeyType = errors.New("nonce.HMACKeyCtxKey value in RPC context must be a string")

// Balancer implements the base.PickerBuilder interface. It's used to create new
// balancer.Picker instances. It should only be used by nonce-service clients.
type Balancer struct{}

// Compile-time assertion that *Balancer implements the base.PickerBuilder
// interface.
var _ base.PickerBuilder = (*Balancer)(nil)

// Build implements the base.PickerBuilder interface. It is called by the gRPC
// runtime when the balancer is first initialized and when the set of backend
// (SubConn) addresses changes.
func (b *Balancer) Build(buildInfo base.PickerBuildInfo) balancer.Picker {
	if len(buildInfo.ReadySCs) == 0 {
		// The Picker must be rebuilt if there are no backends available.
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}
	return &Picker{
		backends: buildInfo.ReadySCs,
	}
}

// Picker implements the balancer.Picker interface. It picks a backend (SubConn)
// based on the nonce prefix contained in each request's Context.
type Picker struct {
	backends        map[balancer.SubConn]base.SubConnInfo
	prefixToBackend map[string]balancer.SubConn
}

// Compile-time assertion that *Picker implements the balancer.Picker interface.
var _ balancer.Picker = (*Picker)(nil)

// Pick implements the balancer.Picker interface. It is called by the gRPC
// runtime for each RPC message. It is responsible for picking a backend
// (SubConn) based on the context of each RPC message.
func (p *Picker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	if len(p.backends) == 0 {
		// This should never happen, the Picker should only be built when there
		// are backends available.
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}

	// Get the HMAC key from the RPC context.
	hmacKeyVal := info.Ctx.Value(nonce.HMACKeyCtxKey{})
	if hmacKeyVal == nil {
		// This should never happen.
		return balancer.PickResult{}, errMissingHMACKeyCtxKey
	}
	hmacKey, ok := hmacKeyVal.(string)
	if !ok {
		// This should never happen.
		return balancer.PickResult{}, errInvalidHMACKeyCtxKeyType
	}

	if p.prefixToBackend == nil {
		// First call to Pick with a new Picker.
		prefixToBackend := make(map[string]balancer.SubConn)
		for sc, scInfo := range p.backends {
			scPrefix := nonce.DerivePrefix(scInfo.Address.Addr, hmacKey)
			prefixToBackend[scPrefix] = sc
		}
		p.prefixToBackend = prefixToBackend
	}

	// Get the destination prefix from the RPC context.
	destPrefixVal := info.Ctx.Value(nonce.PrefixCtxKey{})
	if destPrefixVal == nil {
		// This should never happen.
		return balancer.PickResult{}, errMissingPrefixCtxKey
	}
	destPrefix, ok := destPrefixVal.(string)
	if !ok {
		// This should never happen.
		return balancer.PickResult{}, errInvalidPrefixCtxKeyType
	}

	sc, ok := p.prefixToBackend[destPrefix]
	if !ok {
		// No backend SubConn was found for the destination prefix.
		return balancer.PickResult{}, ErrNoBackendsMatchPrefix.Err()
	}
	return balancer.PickResult{SubConn: sc}, nil
}

func init() {
	balancer.Register(
		base.NewBalancerBuilder(Name, &Balancer{}, base.Config{}),
	)
}
