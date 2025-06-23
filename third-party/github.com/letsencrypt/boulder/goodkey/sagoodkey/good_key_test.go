package sagoodkey

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"

	"google.golang.org/grpc"

	"github.com/letsencrypt/boulder/goodkey"
	sapb "github.com/letsencrypt/boulder/sa/proto"
	"github.com/letsencrypt/boulder/test"
)

func TestDBBlocklistAccept(t *testing.T) {
	for _, testCheck := range []BlockedKeyCheckFunc{
		nil,
		func(context.Context, *sapb.SPKIHash, ...grpc.CallOption) (*sapb.Exists, error) {
			return &sapb.Exists{Exists: false}, nil
		},
	} {
		policy, err := NewPolicy(&goodkey.Config{}, testCheck)
		test.AssertNotError(t, err, "NewKeyPolicy failed")

		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		test.AssertNotError(t, err, "ecdsa.GenerateKey failed")
		err = policy.GoodKey(context.Background(), k.Public())
		test.AssertNotError(t, err, "GoodKey failed with a non-blocked key")
	}
}

func TestDBBlocklistReject(t *testing.T) {
	testCheck := func(context.Context, *sapb.SPKIHash, ...grpc.CallOption) (*sapb.Exists, error) {
		return &sapb.Exists{Exists: true}, nil
	}

	policy, err := NewPolicy(&goodkey.Config{}, testCheck)
	test.AssertNotError(t, err, "NewKeyPolicy failed")

	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "ecdsa.GenerateKey failed")
	err = policy.GoodKey(context.Background(), k.Public())
	test.AssertError(t, err, "GoodKey didn't fail with a blocked key")
	test.AssertErrorIs(t, err, goodkey.ErrBadKey)
	test.AssertEquals(t, err.Error(), "public key is forbidden")
}
