package noncebalancer

import (
	"context"
	"testing"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/resolver"

	"github.com/letsencrypt/boulder/nonce"
	"github.com/letsencrypt/boulder/test"
)

func TestPickerPicksCorrectBackend(t *testing.T) {
	_, p, subConns := setupTest(false)
	prefix := nonce.DerivePrefix(subConns[0].addrs[0].Addr, []byte("Kala namak"))

	testCtx := context.WithValue(context.Background(), nonce.PrefixCtxKey{}, "HNmOnt8w")
	testCtx = context.WithValue(testCtx, nonce.HMACKeyCtxKey{}, []byte(prefix))
	info := balancer.PickInfo{Ctx: testCtx}

	gotPick, err := p.Pick(info)
	test.AssertNotError(t, err, "Pick failed")
	test.AssertDeepEquals(t, subConns[0], gotPick.SubConn)
}

func TestPickerMissingPrefixInCtx(t *testing.T) {
	_, p, subConns := setupTest(false)
	prefix := nonce.DerivePrefix(subConns[0].addrs[0].Addr, []byte("Kala namak"))

	testCtx := context.WithValue(context.Background(), nonce.HMACKeyCtxKey{}, []byte(prefix))
	info := balancer.PickInfo{Ctx: testCtx}

	gotPick, err := p.Pick(info)
	test.AssertErrorIs(t, err, errMissingPrefixCtxKey)
	test.AssertNil(t, gotPick.SubConn, "subConn should be nil")
}

func TestPickerInvalidPrefixInCtx(t *testing.T) {
	_, p, _ := setupTest(false)

	testCtx := context.WithValue(context.Background(), nonce.PrefixCtxKey{}, 9)
	testCtx = context.WithValue(testCtx, nonce.HMACKeyCtxKey{}, []byte("foobar"))
	info := balancer.PickInfo{Ctx: testCtx}

	gotPick, err := p.Pick(info)
	test.AssertErrorIs(t, err, errInvalidPrefixCtxKeyType)
	test.AssertNil(t, gotPick.SubConn, "subConn should be nil")
}

func TestPickerMissingHMACKeyInCtx(t *testing.T) {
	_, p, _ := setupTest(false)

	testCtx := context.WithValue(context.Background(), nonce.PrefixCtxKey{}, "HNmOnt8w")
	info := balancer.PickInfo{Ctx: testCtx}

	gotPick, err := p.Pick(info)
	test.AssertErrorIs(t, err, errMissingHMACKeyCtxKey)
	test.AssertNil(t, gotPick.SubConn, "subConn should be nil")
}

func TestPickerInvalidHMACKeyInCtx(t *testing.T) {
	_, p, _ := setupTest(false)

	testCtx := context.WithValue(context.Background(), nonce.PrefixCtxKey{}, "HNmOnt8w")
	testCtx = context.WithValue(testCtx, nonce.HMACKeyCtxKey{}, 9)
	info := balancer.PickInfo{Ctx: testCtx}

	gotPick, err := p.Pick(info)
	test.AssertErrorIs(t, err, errInvalidHMACKeyCtxKeyType)
	test.AssertNil(t, gotPick.SubConn, "subConn should be nil")
}

func TestPickerNoMatchingSubConnAvailable(t *testing.T) {
	_, p, subConns := setupTest(false)
	prefix := nonce.DerivePrefix(subConns[0].addrs[0].Addr, []byte("Kala namak"))

	testCtx := context.WithValue(context.Background(), nonce.PrefixCtxKey{}, "rUsTrUin")
	testCtx = context.WithValue(testCtx, nonce.HMACKeyCtxKey{}, []byte(prefix))
	info := balancer.PickInfo{Ctx: testCtx}

	gotPick, err := p.Pick(info)
	test.AssertErrorIs(t, err, ErrNoBackendsMatchPrefix.Err())
	test.AssertNil(t, gotPick.SubConn, "subConn should be nil")
}

func TestPickerNoSubConnsAvailable(t *testing.T) {
	b, p, _ := setupTest(true)
	b.Build(base.PickerBuildInfo{})
	info := balancer.PickInfo{Ctx: context.Background()}

	gotPick, err := p.Pick(info)
	test.AssertErrorIs(t, err, balancer.ErrNoSubConnAvailable)
	test.AssertNil(t, gotPick.SubConn, "subConn should be nil")
}

func setupTest(noSubConns bool) (*Balancer, balancer.Picker, []*subConn) {
	var subConns []*subConn
	bi := base.PickerBuildInfo{
		ReadySCs: make(map[balancer.SubConn]base.SubConnInfo),
	}

	sc := &subConn{}
	addr := resolver.Address{Addr: "10.77.77.77:8080"}
	sc.UpdateAddresses([]resolver.Address{addr})

	if !noSubConns {
		bi.ReadySCs[sc] = base.SubConnInfo{Address: addr}
		subConns = append(subConns, sc)
	}

	b := &Balancer{}
	p := b.Build(bi)
	return b, p, subConns
}

// subConn is a test mock which implements the balancer.SubConn interface.
type subConn struct {
	balancer.SubConn
	addrs []resolver.Address
}

func (s *subConn) UpdateAddresses(addrs []resolver.Address) {
	s.addrs = addrs
}
