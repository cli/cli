package wfe2

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/ocsp"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/config"
	"github.com/letsencrypt/boulder/core"
	corepb "github.com/letsencrypt/boulder/core/proto"
	berrors "github.com/letsencrypt/boulder/errors"
	"github.com/letsencrypt/boulder/features"
	"github.com/letsencrypt/boulder/goodkey"
	"github.com/letsencrypt/boulder/identifier"
	"github.com/letsencrypt/boulder/issuance"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/mocks"
	"github.com/letsencrypt/boulder/must"
	"github.com/letsencrypt/boulder/nonce"
	noncepb "github.com/letsencrypt/boulder/nonce/proto"
	"github.com/letsencrypt/boulder/probs"
	rapb "github.com/letsencrypt/boulder/ra/proto"
	"github.com/letsencrypt/boulder/ratelimits"
	"github.com/letsencrypt/boulder/revocation"
	sapb "github.com/letsencrypt/boulder/sa/proto"
	"github.com/letsencrypt/boulder/test"
	inmemnonce "github.com/letsencrypt/boulder/test/inmem/nonce"
	"github.com/letsencrypt/boulder/unpause"
	"github.com/letsencrypt/boulder/web"
)

const (
	agreementURL = "http://example.invalid/terms"

	test1KeyPublicJSON = `
	{
		"kty":"RSA",
		"n":"yNWVhtYEKJR21y9xsHV-PD_bYwbXSeNuFal46xYxVfRL5mqha7vttvjB_vc7Xg2RvgCxHPCqoxgMPTzHrZT75LjCwIW2K_klBYN8oYvTwwmeSkAz6ut7ZxPv-nZaT5TJhGk0NT2kh_zSpdriEJ_3vW-mqxYbbBmpvHqsa1_zx9fSuHYctAZJWzxzUZXykbWMWQZpEiE0J4ajj51fInEzVn7VxV-mzfMyboQjujPh7aNJxAWSq4oQEJJDgWwSh9leyoJoPpONHxh5nEE5AjE01FkGICSxjpZsF-w8hOTI3XXohUdu29Se26k2B0PolDSuj0GIQU6-W9TdLXSjBb2SpQ",
		"e":"AQAB"
	}`

	test1KeyPrivatePEM = `
-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAyNWVhtYEKJR21y9xsHV+PD/bYwbXSeNuFal46xYxVfRL5mqh
a7vttvjB/vc7Xg2RvgCxHPCqoxgMPTzHrZT75LjCwIW2K/klBYN8oYvTwwmeSkAz
6ut7ZxPv+nZaT5TJhGk0NT2kh/zSpdriEJ/3vW+mqxYbbBmpvHqsa1/zx9fSuHYc
tAZJWzxzUZXykbWMWQZpEiE0J4ajj51fInEzVn7VxV+mzfMyboQjujPh7aNJxAWS
q4oQEJJDgWwSh9leyoJoPpONHxh5nEE5AjE01FkGICSxjpZsF+w8hOTI3XXohUdu
29Se26k2B0PolDSuj0GIQU6+W9TdLXSjBb2SpQIDAQABAoIBAHw58SXYV/Yp72Cn
jjFSW+U0sqWMY7rmnP91NsBjl9zNIe3C41pagm39bTIjB2vkBNR8ZRG7pDEB/QAc
Cn9Keo094+lmTArjL407ien7Ld+koW7YS8TyKADYikZo0vAK3qOy14JfQNiFAF9r
Bw61hG5/E58cK5YwQZe+YcyBK6/erM8fLrJEyw4CV49wWdq/QqmNYU1dx4OExAkl
KMfvYXpjzpvyyTnZuS4RONfHsO8+JTyJVm+lUv2x+bTce6R4W++UhQY38HakJ0x3
XRfXooRv1Bletu5OFlpXfTSGz/5gqsfemLSr5UHncsCcFMgoFBsk2t/5BVukBgC7
PnHrAjkCgYEA887PRr7zu3OnaXKxylW5U5t4LzdMQLpslVW7cLPD4Y08Rye6fF5s
O/jK1DNFXIoUB7iS30qR7HtaOnveW6H8/kTmMv/YAhLO7PAbRPCKxxcKtniEmP1x
ADH0tF2g5uHB/zeZhCo9qJiF0QaJynvSyvSyJFmY6lLvYZsAW+C+PesCgYEA0uCi
Q8rXLzLpfH2NKlLwlJTi5JjE+xjbabgja0YySwsKzSlmvYJqdnE2Xk+FHj7TCnSK
KUzQKR7+rEk5flwEAf+aCCNh3W4+Hp9MmrdAcCn8ZsKmEW/o7oDzwiAkRCmLw/ck
RSFJZpvFoxEg15riT37EjOJ4LBZ6SwedsoGA/a8CgYEA2Ve4sdGSR73/NOKZGc23
q4/B4R2DrYRDPhEySnMGoPCeFrSU6z/lbsUIU4jtQWSaHJPu4n2AfncsZUx9WeSb
OzTCnh4zOw33R4N4W8mvfXHODAJ9+kCc1tax1YRN5uTEYzb2dLqPQtfNGxygA1DF
BkaC9CKnTeTnH3TlKgK8tUcCgYB7J1lcgh+9ntwhKinBKAL8ox8HJfkUM+YgDbwR
sEM69E3wl1c7IekPFvsLhSFXEpWpq3nsuMFw4nsVHwaGtzJYAHByhEdpTDLXK21P
heoKF1sioFbgJB1C/Ohe3OqRLDpFzhXOkawOUrbPjvdBM2Erz/r11GUeSlpNazs7
vsoYXQKBgFwFM1IHmqOf8a2wEFa/a++2y/WT7ZG9nNw1W36S3P04K4lGRNRS2Y/S
snYiqxD9nL7pVqQP2Qbqbn0yD6d3G5/7r86F7Wu2pihM8g6oyMZ3qZvvRIBvKfWo
eROL1ve1vmQF3kjrMPhhK2kr6qdWnTE5XlPllVSZFQenSTzj98AO
-----END RSA PRIVATE KEY-----
`

	test2KeyPublicJSON = `{
		"kty":"RSA",
		"n":"qnARLrT7Xz4gRcKyLdydmCr-ey9OuPImX4X40thk3on26FkMznR3fRjs66eLK7mmPcBZ6uOJseURU6wAaZNmemoYx1dMvqvWWIyiQleHSD7Q8vBrhR6uIoO4jAzJZR-ChzZuSDt7iHN-3xUVspu5XGwXU_MVJZshTwp4TaFx5elHIT_ObnTvTOU3Xhish07AbgZKmWsVbXh5s-CrIicU4OexJPgunWZ_YJJueOKmTvnLlTV4MzKR2oZlBKZ27S0-SfdV_QDx_ydle5oMAyKVtlAV35cyPMIsYNwgUGBCdY_2Uzi5eX0lTc7MPRwz6qR1kip-i59VcGcUQgqHV6Fyqw",
		"e":"AQAB"
	}`

	test2KeyPrivatePEM = `
-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEAqnARLrT7Xz4gRcKyLdydmCr+ey9OuPImX4X40thk3on26FkM
znR3fRjs66eLK7mmPcBZ6uOJseURU6wAaZNmemoYx1dMvqvWWIyiQleHSD7Q8vBr
hR6uIoO4jAzJZR+ChzZuSDt7iHN+3xUVspu5XGwXU/MVJZshTwp4TaFx5elHIT/O
bnTvTOU3Xhish07AbgZKmWsVbXh5s+CrIicU4OexJPgunWZ/YJJueOKmTvnLlTV4
MzKR2oZlBKZ27S0+SfdV/QDx/ydle5oMAyKVtlAV35cyPMIsYNwgUGBCdY/2Uzi5
eX0lTc7MPRwz6qR1kip+i59VcGcUQgqHV6FyqwIDAQABAoIBAG5m8Xpj2YC0aYtG
tsxmX9812mpJFqFOmfS+f5N0gMJ2c+3F4TnKz6vE/ZMYkFnehAT0GErC4WrOiw68
F/hLdtJM74gQ0LGh9dKeJmz67bKqngcAHWW5nerVkDGIBtzuMEsNwxofDcIxrjkr
G0b7AHMRwXqrt0MI3eapTYxby7+08Yxm40mxpSsW87FSaI61LDxUDpeVkn7kolSN
WifVat7CpZb/D2BfGAQDxiU79YzgztpKhbynPdGc/OyyU+CNgk9S5MgUX2m9Elh3
aXrWh2bT2xzF+3KgZdNkJQcdIYVoGq/YRBxlGXPYcG4Do3xKhBmH79Io2BizevZv
nHkbUGECgYEAydjb4rl7wYrElDqAYpoVwKDCZAgC6o3AKSGXfPX1Jd2CXgGR5Hkl
ywP0jdSLbn2v/jgKQSAdRbYuEiP7VdroMb5M6BkBhSY619cH8etoRoLzFo1GxcE8
Y7B598VXMq8TT+TQqw/XRvM18aL3YDZ3LSsR7Gl2jF/sl6VwQAaZToUCgYEA2Cn4
fG58ME+M4IzlZLgAIJ83PlLb9ip6MeHEhUq2Dd0In89nss7Acu0IVg8ES88glJZy
4SjDLGSiuQuoQVo9UBq/E5YghdMJFp5ovwVfEaJ+ruWqOeujvWzzzPVyIWSLXRQa
N4kedtfrlqldMIXywxVru66Q1NOGvhDHm/Q8+28CgYEAkhLCbn3VNed7A9qidrkT
7OdqRoIVujEDU8DfpKtK0jBP3EA+mJ2j4Bvoq4uZrEiBSPS9VwwqovyIstAfX66g
Qv95IK6YDwfvpawUL9sxB3ZU/YkYIp0JWwun+Mtzo1ZYH4V0DZfVL59q9of9hj9k
V+fHfNOF22jAC67KYUtlPxECgYEAwF6hj4L3rDqvQYrB/p8tJdrrW+B7dhgZRNkJ
fiGd4LqLGUWHoH4UkHJXT9bvWNPMx88YDz6qapBoq8svAnHfTLFwyGp7KP1FAkcZ
Kp4KG/SDTvx+QCtvPX1/fjAUUJlc2QmxxyiU3uiK9Tpl/2/FOk2O4aiZpX1VVUIz
kZuKxasCgYBiVRkEBk2W4Ia0B7dDkr2VBrz4m23Y7B9cQLpNAapiijz/0uHrrCl8
TkLlEeVOuQfxTadw05gzKX0jKkMC4igGxvEeilYc6NR6a4nvRulG84Q8VV9Sy9Ie
wk6Oiadty3eQqSBJv0HnpmiEdQVffIK5Pg4M8Dd+aOBnEkbopAJOuA==
-----END RSA PRIVATE KEY-----
`
	test3KeyPrivatePEM = `
-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEAuTQER6vUA1RDixS8xsfCRiKUNGRzzyIK0MhbS2biClShbb0h
Sx2mPP7gBvis2lizZ9r+y9hL57kNQoYCKndOBg0FYsHzrQ3O9AcoV1z2Mq+XhHZb
FrVYaXI0M3oY9BJCWog0dyi3XC0x8AxC1npd1U61cToHx+3uSvgZOuQA5ffEn5L3
8Dz1Ti7OV3E4XahnRJvejadUmTkki7phLBUXm5MnnyFm0CPpf6ApV7zhLjN5W+nV
0WL17o7v8aDgV/t9nIdi1Y26c3PlCEtiVHZcebDH5F1Deta3oLLg9+g6rWnTqPbY
3knffhp4m0scLD6e33k8MtzxDX/D7vHsg0/X1wIDAQABAoIBAQCnFJpX3lhiuH5G
1uqHmmdVxpRVv9oKn/eJ63cRSzvZfgg0bE/A6Hq0xGtvXqDySttvck4zsGqqHnQr
86G4lfE53D1jnv4qvS5bUKnARwmFKIxU4EHE9s1QM8uMNTaV2nMqIX7TkVP6QHuw
yB70R2inq15dS7EBWVGFKNX6HwAAdj8pFuF6o2vIwmAfee20aFzpWWf81jOH9Ai6
hyJyV3NqrU1JzIwlXaeX67R1VroFdhN/lapp+2b0ZEcJJtFlcYFl99NjkQeVZyik
izNv0GZZNWizc57wU0/8cv+jQ2f26ltvyrPz3QNK61bFfzy+/tfMvLq7sdCmztKJ
tMxCBJOBAoGBAPKnIVQIS2nTvC/qZ8ajw1FP1rkvYblIiixegjgfFhM32HehQ+nu
3TELi3I3LngLYi9o6YSqtNBmdBJB+DUAzIXp0TdOihOweGiv5dAEWwY9rjCzMT5S
GP7dCWiJwoMUHrOs1Po3dwcjj/YsoAW+FC0jSvach2Ln2CvPgr5FP0ARAoGBAMNj
64qUCzgeXiSyPKK69bCCGtHlTYUndwHQAZmABjbmxAXZNYgp/kBezFpKOwmICE8R
kK8YALRrL0VWXl/yj85b0HAZGkquNFHPUDd1e6iiP5TrY+Hy4oqtlYApjH6f85CE
lWjQ1iyUL7aT6fcSgzq65ZWD2hUzvNtWbTt6zQFnAoGAWS/EuDY0QblpOdNWQVR/
vasyqO4ZZRiccKJsCmSioH2uOoozhBAfjJ9JqblOgyDr/bD546E6xD5j+zH0IMci
ZTYDh+h+J659Ez1Topl3O1wAYjX6q4VRWpuzkZDQxYznm/KydSVdwmn3x+uvBW1P
zSdjrjDqMhg1BCVJUNXy4YECgYEAjX1z+dwO68qB3gz7/9NnSzRL+6cTJdNYSIW6
QtAEsAkX9iw+qaXPKgn77X5HljVd3vQXU9QL3pqnloxetxhNrt+p5yMmeOIBnSSF
MEPxEkK7zDlRETPzfP0Kf86WoLNviz2XfFmOXqXIj2w5RuOvB/6DdmwOpr/aiPLj
EulwPw0CgYAMSzsWOt6vU+y/G5NyhUCHvY50TdnGOj2btBk9rYVwWGWxCpg2QF0R
pcKXgGzXEVZKFAqB8V1c/mmCo8ojPgmqGM+GzX2Bj4seVBW7PsTeZUjrHpADshjV
F7o5b7y92NlxO5kwQzRKEAhwS5PbKJdx90iCuG+JlI1YgWlA1VcJMw==
-----END RSA PRIVATE KEY-----
`

	testE1KeyPrivatePEM = `
-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIH+p32RUnqT/iICBEGKrLIWFcyButv0S0lU/BLPOyHn2oAoGCCqGSM49
AwEHoUQDQgAEFwvSZpu06i3frSk/mz9HcD9nETn4wf3mQ+zDtG21GapLytH7R1Zr
ycBzDV9u6cX9qNLc9Bn5DAumz7Zp2AuA+Q==
-----END EC PRIVATE KEY-----
`

	testE2KeyPrivatePEM = `
-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIFRcPxQ989AY6se2RyIoF1ll9O6gHev4oY15SWJ+Jf5eoAoGCCqGSM49
AwEHoUQDQgAES8FOmrZ3ywj4yyFqt0etAD90U+EnkNaOBSLfQmf7pNi8y+kPKoUN
EeMZ9nWyIM6bktLrE11HnFOnKhAYsM5fZA==
-----END EC PRIVATE KEY-----`
)

type MockRegistrationAuthority struct {
	rapb.RegistrationAuthorityClient
	clk                  clock.Clock
	lastRevocationReason revocation.Reason
}

func (ra *MockRegistrationAuthority) NewRegistration(ctx context.Context, in *corepb.Registration, _ ...grpc.CallOption) (*corepb.Registration, error) {
	in.Id = 1
	in.Contact = nil
	created := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	in.CreatedAt = timestamppb.New(created)
	return in, nil
}

func (ra *MockRegistrationAuthority) UpdateRegistrationKey(ctx context.Context, in *rapb.UpdateRegistrationKeyRequest, _ ...grpc.CallOption) (*corepb.Registration, error) {
	return &corepb.Registration{
		Status: string(core.StatusValid),
		Key:    in.Jwk,
	}, nil
}

func (ra *MockRegistrationAuthority) DeactivateRegistration(context.Context, *rapb.DeactivateRegistrationRequest, ...grpc.CallOption) (*corepb.Registration, error) {
	return &corepb.Registration{
		Status: string(core.StatusDeactivated),
		Key:    []byte(test1KeyPublicJSON),
	}, nil
}

func (ra *MockRegistrationAuthority) PerformValidation(context.Context, *rapb.PerformValidationRequest, ...grpc.CallOption) (*corepb.Authorization, error) {
	return &corepb.Authorization{}, nil
}

func (ra *MockRegistrationAuthority) RevokeCertByApplicant(ctx context.Context, in *rapb.RevokeCertByApplicantRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	ra.lastRevocationReason = revocation.Reason(in.Code)
	return &emptypb.Empty{}, nil
}

func (ra *MockRegistrationAuthority) RevokeCertByKey(ctx context.Context, in *rapb.RevokeCertByKeyRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	ra.lastRevocationReason = revocation.Reason(ocsp.KeyCompromise)
	return &emptypb.Empty{}, nil
}

// GetAuthorization returns a different authorization depending on the requested
// ID. All authorizations are associated with RegID 1, except for the one that isn't.
func (ra *MockRegistrationAuthority) GetAuthorization(_ context.Context, in *rapb.GetAuthorizationRequest, _ ...grpc.CallOption) (*corepb.Authorization, error) {
	switch in.Id {
	case 1: // Return a valid authorization with a single valid challenge.
		return &corepb.Authorization{
			Id:             "1",
			RegistrationID: 1,
			Identifier:     identifier.NewDNS("not-an-example.com").ToProto(),
			Status:         string(core.StatusValid),
			Expires:        timestamppb.New(ra.clk.Now().AddDate(100, 0, 0)),
			Challenges: []*corepb.Challenge{
				{Id: 1, Type: "http-01", Status: string(core.StatusValid), Token: "token"},
			},
		}, nil
	case 2: // Return a pending authorization with three pending challenges.
		return &corepb.Authorization{
			Id:             "2",
			RegistrationID: 1,
			Identifier:     identifier.NewDNS("not-an-example.com").ToProto(),
			Status:         string(core.StatusPending),
			Expires:        timestamppb.New(ra.clk.Now().AddDate(100, 0, 0)),
			Challenges: []*corepb.Challenge{
				{Id: 1, Type: "http-01", Status: string(core.StatusPending), Token: "token"},
				{Id: 2, Type: "dns-01", Status: string(core.StatusPending), Token: "token"},
				{Id: 3, Type: "tls-alpn-01", Status: string(core.StatusPending), Token: "token"},
			},
		}, nil
	case 3: // Return an expired authorization with three pending (but expired) challenges.
		return &corepb.Authorization{
			Id:             "3",
			RegistrationID: 1,
			Identifier:     identifier.NewDNS("not-an-example.com").ToProto(),
			Status:         string(core.StatusPending),
			Expires:        timestamppb.New(ra.clk.Now().AddDate(-1, 0, 0)),
			Challenges: []*corepb.Challenge{
				{Id: 1, Type: "http-01", Status: string(core.StatusPending), Token: "token"},
				{Id: 2, Type: "dns-01", Status: string(core.StatusPending), Token: "token"},
				{Id: 3, Type: "tls-alpn-01", Status: string(core.StatusPending), Token: "token"},
			},
		}, nil
	case 4: // Return an internal server error.
		return nil, fmt.Errorf("unspecified error")
	case 5: // Return a pending authorization as above, but associated with RegID 2.
		return &corepb.Authorization{
			Id:             "5",
			RegistrationID: 2,
			Identifier:     identifier.NewDNS("not-an-example.com").ToProto(),
			Status:         string(core.StatusPending),
			Expires:        timestamppb.New(ra.clk.Now().AddDate(100, 0, 0)),
			Challenges: []*corepb.Challenge{
				{Id: 1, Type: "http-01", Status: string(core.StatusPending), Token: "token"},
				{Id: 2, Type: "dns-01", Status: string(core.StatusPending), Token: "token"},
				{Id: 3, Type: "tls-alpn-01", Status: string(core.StatusPending), Token: "token"},
			},
		}, nil
	}

	return nil, berrors.NotFoundError("no authorization found with id %q", in.Id)
}

func (ra *MockRegistrationAuthority) DeactivateAuthorization(context.Context, *corepb.Authorization, ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (ra *MockRegistrationAuthority) NewOrder(ctx context.Context, in *rapb.NewOrderRequest, _ ...grpc.CallOption) (*corepb.Order, error) {
	created := time.Date(2021, 1, 1, 1, 1, 1, 0, time.UTC)
	expires := time.Date(2021, 2, 1, 1, 1, 1, 0, time.UTC)

	return &corepb.Order{
		Id:               1,
		RegistrationID:   in.RegistrationID,
		Created:          timestamppb.New(created),
		Expires:          timestamppb.New(expires),
		Identifiers:      in.Identifiers,
		Status:           string(core.StatusPending),
		V2Authorizations: []int64{1},
	}, nil
}

func (ra *MockRegistrationAuthority) FinalizeOrder(ctx context.Context, in *rapb.FinalizeOrderRequest, _ ...grpc.CallOption) (*corepb.Order, error) {
	in.Order.Status = string(core.StatusProcessing)
	return in.Order, nil
}

func makeBody(s string) io.ReadCloser {
	return io.NopCloser(strings.NewReader(s))
}

// loadKey loads a private key from PEM/DER-encoded data and returns
// a `crypto.Signer`.
func loadKey(t *testing.T, keyBytes []byte) crypto.Signer {
	// pem.Decode does not return an error as its 2nd arg, but instead the "rest"
	// that was leftover from parsing the PEM block. We only care if the decoded
	// PEM block was empty for this test function.
	block, _ := pem.Decode(keyBytes)
	if block == nil {
		t.Fatal("Unable to decode private key PEM bytes")
	}

	// Try decoding as an RSA private key
	if rsaKey, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return rsaKey
	}

	// Try decoding as a PKCS8 private key
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		// Determine the key's true type and return it as a crypto.Signer
		switch k := key.(type) {
		case *rsa.PrivateKey:
			return k
		case *ecdsa.PrivateKey:
			return k
		}
	}

	// Try as an ECDSA private key
	if ecdsaKey, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return ecdsaKey
	}

	// Nothing worked! Fail hard.
	t.Fatalf("Unable to decode private key PEM bytes")
	// NOOP - the t.Fatal() call will abort before this return
	return nil
}

var ctx = context.Background()

func setupWFE(t *testing.T) (WebFrontEndImpl, clock.FakeClock, requestSigner) {
	features.Reset()

	fc := clock.NewFake()
	stats := metrics.NoopRegisterer

	testKeyPolicy, err := goodkey.NewPolicy(nil, nil)
	test.AssertNotError(t, err, "creating test keypolicy")

	certChains := map[issuance.NameID][][]byte{}
	issuerCertificates := map[issuance.NameID]*issuance.Certificate{}
	for _, files := range [][]string{
		{
			"../test/hierarchy/int-r3.cert.pem",
			"../test/hierarchy/root-x1.cert.pem",
		},
		{
			"../test/hierarchy/int-r3-cross.cert.pem",
			"../test/hierarchy/root-dst.cert.pem",
		},
		{
			"../test/hierarchy/int-e1.cert.pem",
			"../test/hierarchy/root-x2.cert.pem",
		},
		{
			"../test/hierarchy/int-e1.cert.pem",
			"../test/hierarchy/root-x2-cross.cert.pem",
			"../test/hierarchy/root-x1-cross.cert.pem",
			"../test/hierarchy/root-dst.cert.pem",
		},
	} {
		certs, err := issuance.LoadChain(files)
		test.AssertNotError(t, err, "Unable to load chain")
		var buf bytes.Buffer
		for _, cert := range certs {
			buf.Write([]byte("\n"))
			buf.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}))
		}
		id := certs[0].NameID()
		certChains[id] = append(certChains[id], buf.Bytes())
		issuerCertificates[id] = certs[0]
	}

	mockSA := mocks.NewStorageAuthorityReadOnly(fc)

	// Use derived nonces.
	rncKey := []byte("b8c758dd85e113ea340ce0b3a99f389d40a308548af94d1730a7692c1874f1f")
	noncePrefix := nonce.DerivePrefix("192.168.1.1:8080", rncKey)
	nonceService, err := nonce.NewNonceService(metrics.NoopRegisterer, 100, noncePrefix)
	test.AssertNotError(t, err, "making nonceService")

	inmemNonceService := &inmemnonce.Service{NonceService: nonceService}
	gnc := inmemNonceService
	rnc := inmemNonceService

	// Setup rate limiting.
	limiter, err := ratelimits.NewLimiter(fc, ratelimits.NewInmemSource(), stats)
	test.AssertNotError(t, err, "making limiter")
	txnBuilder, err := ratelimits.NewTransactionBuilderFromFiles("../test/config-next/wfe2-ratelimit-defaults.yml", "")
	test.AssertNotError(t, err, "making transaction composer")

	unpauseSigner, err := unpause.NewJWTSigner(cmd.HMACKeyConfig{KeyFile: "../test/secrets/sfe_unpause_key"})
	test.AssertNotError(t, err, "making unpause signer")
	unpauseLifetime := time.Hour * 24 * 14
	unpauseURL := "https://boulder.service.consul:4003"
	wfe, err := NewWebFrontEndImpl(
		stats,
		fc,
		testKeyPolicy,
		certChains,
		issuerCertificates,
		blog.NewMock(),
		10*time.Second,
		10*time.Second,
		2,
		&MockRegistrationAuthority{clk: fc},
		mockSA,
		nil,
		gnc,
		rnc,
		rncKey,
		mockSA,
		limiter,
		txnBuilder,
		map[string]string{"default": "a test profile"},
		unpauseSigner,
		unpauseLifetime,
		unpauseURL,
	)
	test.AssertNotError(t, err, "Unable to create WFE")

	wfe.SubscriberAgreementURL = agreementURL

	return wfe, fc, requestSigner{t, inmemNonceService.AsSource()}
}

// makePostRequestWithPath creates an http.Request for localhost with method
// POST, the provided body, and the correct Content-Length. The path provided
// will be parsed as a URL and used to populate the request URL and RequestURI
func makePostRequestWithPath(path string, body string) *http.Request {
	request := &http.Request{
		Method:     "POST",
		RemoteAddr: "1.1.1.1:7882",
		Header: map[string][]string{
			"Content-Length": {strconv.Itoa(len(body))},
			"Content-Type":   {expectedJWSContentType},
		},
		Body: makeBody(body),
		Host: "localhost",
	}
	url := mustParseURL(path)
	request.URL = url
	request.RequestURI = url.Path
	return request
}

// signAndPost constructs a JWS signed by the account with ID 1, over the given
// payload, with the protected URL set to the provided signedURL. An HTTP
// request constructed to the provided path with the encoded JWS body as the
// POST body is returned.
func signAndPost(signer requestSigner, path, signedURL, payload string) *http.Request {
	_, _, body := signer.byKeyID(1, nil, signedURL, payload)
	return makePostRequestWithPath(path, body)
}

func mustParseURL(s string) *url.URL {
	return must.Do(url.Parse(s))
}

func sortHeader(s string) string {
	a := strings.Split(s, ", ")
	sort.Strings(a)
	return strings.Join(a, ", ")
}

func addHeadIfGet(s []string) []string {
	for _, a := range s {
		if a == "GET" {
			return append(s, "HEAD")
		}
	}
	return s
}

func TestHandleFunc(t *testing.T) {
	wfe, _, _ := setupWFE(t)
	var mux *http.ServeMux
	var rw *httptest.ResponseRecorder
	var stubCalled bool
	runWrappedHandler := func(req *http.Request, pattern string, allowed ...string) {
		mux = http.NewServeMux()
		rw = httptest.NewRecorder()
		stubCalled = false
		wfe.HandleFunc(mux, pattern, func(context.Context, *web.RequestEvent, http.ResponseWriter, *http.Request) {
			stubCalled = true
		}, allowed...)
		req.URL = mustParseURL(pattern)
		mux.ServeHTTP(rw, req)
	}

	// Plain requests (no CORS)
	type testCase struct {
		allowed        []string
		reqMethod      string
		shouldCallStub bool
		shouldSucceed  bool
		pattern        string
	}
	var lastNonce string
	for _, c := range []testCase{
		{[]string{"GET", "POST"}, "GET", true, true, "/test"},
		{[]string{"GET", "POST"}, "GET", true, true, newNoncePath},
		{[]string{"GET", "POST"}, "POST", true, true, "/test"},
		{[]string{"GET"}, "", false, false, "/test"},
		{[]string{"GET"}, "POST", false, false, "/test"},
		{[]string{"GET"}, "OPTIONS", false, true, "/test"},
		{[]string{"GET"}, "MAKE-COFFEE", false, false, "/test"}, // 405, or 418?
		{[]string{"GET"}, "GET", true, true, directoryPath},
	} {
		runWrappedHandler(&http.Request{Method: c.reqMethod}, c.pattern, c.allowed...)
		test.AssertEquals(t, stubCalled, c.shouldCallStub)
		if c.shouldSucceed {
			test.AssertEquals(t, rw.Code, http.StatusOK)
		} else {
			test.AssertEquals(t, rw.Code, http.StatusMethodNotAllowed)
			test.AssertEquals(t, sortHeader(rw.Header().Get("Allow")), sortHeader(strings.Join(addHeadIfGet(c.allowed), ", ")))
			test.AssertUnmarshaledEquals(t,
				rw.Body.String(),
				`{"type":"`+probs.ErrorNS+`malformed","detail":"Method not allowed","status":405}`)
		}
		if c.reqMethod == "GET" && c.pattern != newNoncePath {
			nonce := rw.Header().Get("Replay-Nonce")
			test.AssertEquals(t, nonce, "")
		} else {
			nonce := rw.Header().Get("Replay-Nonce")
			test.AssertNotEquals(t, nonce, lastNonce)
			test.AssertNotEquals(t, nonce, "")
			lastNonce = nonce
		}
		linkHeader := rw.Header().Get("Link")
		if c.pattern != directoryPath {
			// If the pattern wasn't the directory there should be a Link header for the index
			test.AssertEquals(t, linkHeader, `<http://localhost/directory>;rel="index"`)
		} else {
			// The directory resource shouldn't get a link header
			test.AssertEquals(t, linkHeader, "")
		}
	}

	// Disallowed method returns error JSON in body
	runWrappedHandler(&http.Request{Method: "PUT"}, "/test", "GET", "POST")
	test.AssertEquals(t, rw.Header().Get("Content-Type"), "application/problem+json")
	test.AssertUnmarshaledEquals(t, rw.Body.String(), `{"type":"`+probs.ErrorNS+`malformed","detail":"Method not allowed","status":405}`)
	test.AssertEquals(t, sortHeader(rw.Header().Get("Allow")), "GET, HEAD, POST")

	// Disallowed method special case: response to HEAD has got no body
	runWrappedHandler(&http.Request{Method: "HEAD"}, "/test", "GET", "POST")
	test.AssertEquals(t, stubCalled, true)
	test.AssertEquals(t, rw.Body.String(), "")

	// HEAD doesn't work with POST-only endpoints
	runWrappedHandler(&http.Request{Method: "HEAD"}, "/test", "POST")
	test.AssertEquals(t, stubCalled, false)
	test.AssertEquals(t, rw.Code, http.StatusMethodNotAllowed)
	test.AssertEquals(t, rw.Header().Get("Content-Type"), "application/problem+json")
	test.AssertEquals(t, rw.Header().Get("Allow"), "POST")
	test.AssertUnmarshaledEquals(t, rw.Body.String(), `{"type":"`+probs.ErrorNS+`malformed","detail":"Method not allowed","status":405}`)

	wfe.AllowOrigins = []string{"*"}
	testOrigin := "https://example.com"

	// CORS "actual" request for disallowed method
	runWrappedHandler(&http.Request{
		Method: "POST",
		Header: map[string][]string{
			"Origin": {testOrigin},
		},
	}, "/test", "GET")
	test.AssertEquals(t, stubCalled, false)
	test.AssertEquals(t, rw.Code, http.StatusMethodNotAllowed)

	// CORS "actual" request for allowed method
	runWrappedHandler(&http.Request{
		Method: "GET",
		Header: map[string][]string{
			"Origin": {testOrigin},
		},
	}, "/test", "GET", "POST")
	test.AssertEquals(t, stubCalled, true)
	test.AssertEquals(t, rw.Code, http.StatusOK)
	test.AssertEquals(t, rw.Header().Get("Access-Control-Allow-Methods"), "")
	test.AssertEquals(t, rw.Header().Get("Access-Control-Allow-Origin"), "*")
	test.AssertEquals(t, rw.Header().Get("Access-Control-Allow-Headers"), "Content-Type")
	test.AssertEquals(t, sortHeader(rw.Header().Get("Access-Control-Expose-Headers")), "Link, Location, Replay-Nonce")

	// CORS preflight request for disallowed method
	runWrappedHandler(&http.Request{
		Method: "OPTIONS",
		Header: map[string][]string{
			"Origin":                        {testOrigin},
			"Access-Control-Request-Method": {"POST"},
		},
	}, "/test", "GET")
	test.AssertEquals(t, stubCalled, false)
	test.AssertEquals(t, rw.Code, http.StatusOK)
	test.AssertEquals(t, rw.Header().Get("Allow"), "GET, HEAD")
	test.AssertEquals(t, rw.Header().Get("Access-Control-Allow-Origin"), "")
	test.AssertEquals(t, rw.Header().Get("Access-Control-Allow-Headers"), "")

	// CORS preflight request for allowed method
	runWrappedHandler(&http.Request{
		Method: "OPTIONS",
		Header: map[string][]string{
			"Origin":                         {testOrigin},
			"Access-Control-Request-Method":  {"POST"},
			"Access-Control-Request-Headers": {"X-Accept-Header1, X-Accept-Header2", "X-Accept-Header3"},
		},
	}, "/test", "GET", "POST")
	test.AssertEquals(t, rw.Code, http.StatusOK)
	test.AssertEquals(t, rw.Header().Get("Access-Control-Allow-Origin"), "*")
	test.AssertEquals(t, rw.Header().Get("Access-Control-Allow-Headers"), "Content-Type")
	test.AssertEquals(t, rw.Header().Get("Access-Control-Max-Age"), "86400")
	test.AssertEquals(t, sortHeader(rw.Header().Get("Access-Control-Allow-Methods")), "GET, HEAD, POST")
	test.AssertEquals(t, sortHeader(rw.Header().Get("Access-Control-Expose-Headers")), "Link, Location, Replay-Nonce")

	// OPTIONS request without an Origin header (i.e., not a CORS
	// preflight request)
	runWrappedHandler(&http.Request{
		Method: "OPTIONS",
		Header: map[string][]string{
			"Access-Control-Request-Method": {"POST"},
		},
	}, "/test", "GET", "POST")
	test.AssertEquals(t, rw.Code, http.StatusOK)
	test.AssertEquals(t, rw.Header().Get("Access-Control-Allow-Origin"), "")
	test.AssertEquals(t, rw.Header().Get("Access-Control-Allow-Headers"), "")
	test.AssertEquals(t, sortHeader(rw.Header().Get("Allow")), "GET, HEAD, POST")

	// CORS preflight request missing optional Request-Method
	// header. The "actual" request will be GET.
	for _, allowedMethod := range []string{"GET", "POST"} {
		runWrappedHandler(&http.Request{
			Method: "OPTIONS",
			Header: map[string][]string{
				"Origin": {testOrigin},
			},
		}, "/test", allowedMethod)
		test.AssertEquals(t, rw.Code, http.StatusOK)
		if allowedMethod == "GET" {
			test.AssertEquals(t, rw.Header().Get("Access-Control-Allow-Origin"), "*")
			test.AssertEquals(t, rw.Header().Get("Access-Control-Allow-Headers"), "Content-Type")
			test.AssertEquals(t, rw.Header().Get("Access-Control-Allow-Methods"), "GET, HEAD")
		} else {
			test.AssertEquals(t, rw.Header().Get("Access-Control-Allow-Origin"), "")
			test.AssertEquals(t, rw.Header().Get("Access-Control-Allow-Headers"), "")
		}
	}

	// No CORS headers are given when configuration does not list
	// "*" or the client-provided origin.
	for _, wfe.AllowOrigins = range [][]string{
		{},
		{"http://example.com", "https://other.example"},
		{""}, // Invalid origin is never matched
	} {
		runWrappedHandler(&http.Request{
			Method: "OPTIONS",
			Header: map[string][]string{
				"Origin":                        {testOrigin},
				"Access-Control-Request-Method": {"POST"},
			},
		}, "/test", "POST")
		test.AssertEquals(t, rw.Code, http.StatusOK)
		for _, h := range []string{
			"Access-Control-Allow-Methods",
			"Access-Control-Allow-Origin",
			"Access-Control-Allow-Headers",
			"Access-Control-Expose-Headers",
			"Access-Control-Request-Headers",
		} {
			test.AssertEquals(t, rw.Header().Get(h), "")
		}
	}

	// CORS headers are offered when configuration lists "*" or
	// the client-provided origin.
	for _, wfe.AllowOrigins = range [][]string{
		{testOrigin, "http://example.org", "*"},
		{"", "http://example.org", testOrigin}, // Invalid origin is harmless
	} {
		runWrappedHandler(&http.Request{
			Method: "OPTIONS",
			Header: map[string][]string{
				"Origin":                        {testOrigin},
				"Access-Control-Request-Method": {"POST"},
			},
		}, "/test", "POST")
		test.AssertEquals(t, rw.Code, http.StatusOK)
		test.AssertEquals(t, rw.Header().Get("Access-Control-Allow-Origin"), testOrigin)
		// http://www.w3.org/TR/cors/ section 6.4:
		test.AssertEquals(t, rw.Header().Get("Vary"), "Origin")
	}
}

func TestPOST404(t *testing.T) {
	wfe, _, _ := setupWFE(t)
	responseWriter := httptest.NewRecorder()
	url, _ := url.Parse("/foobar")
	wfe.Index(ctx, newRequestEvent(), responseWriter, &http.Request{
		Method: "POST",
		URL:    url,
	})
	test.AssertEquals(t, responseWriter.Code, http.StatusNotFound)
}

func TestIndex(t *testing.T) {
	wfe, _, _ := setupWFE(t)

	responseWriter := httptest.NewRecorder()

	url, _ := url.Parse("/")
	wfe.Index(ctx, newRequestEvent(), responseWriter, &http.Request{
		Method: "GET",
		URL:    url,
	})
	test.AssertEquals(t, responseWriter.Code, http.StatusOK)
	test.AssertNotEquals(t, responseWriter.Body.String(), "404 page not found\n")
	test.Assert(t, strings.Contains(responseWriter.Body.String(), directoryPath),
		"directory path not found")
	test.AssertEquals(t, responseWriter.Header().Get("Cache-Control"), "public, max-age=0, no-cache")

	responseWriter.Body.Reset()
	responseWriter.Header().Del("Cache-Control")
	url, _ = url.Parse("/foo")
	wfe.Index(ctx, newRequestEvent(), responseWriter, &http.Request{
		URL: url,
	})
	//test.AssertEquals(t, responseWriter.Code, http.StatusNotFound)
	test.AssertEquals(t, responseWriter.Body.String(), "404 page not found\n")
	test.AssertEquals(t, responseWriter.Header().Get("Cache-Control"), "")
}

// randomDirectoryKeyPresent unmarshals the given buf of JSON and returns true
// if `randomDirKeyExplanationLink` appears as the value of a key in the directory
// object.
func randomDirectoryKeyPresent(t *testing.T, buf []byte) bool {
	var dir map[string]interface{}
	err := json.Unmarshal(buf, &dir)
	if err != nil {
		t.Errorf("Failed to unmarshal directory: %s", err)
	}
	for _, v := range dir {
		if v == randomDirKeyExplanationLink {
			return true
		}
	}
	return false
}

type fakeRand struct{}

func (fr fakeRand) Read(p []byte) (int, error) {
	return len(p), nil
}

func TestDirectory(t *testing.T) {
	wfe, _, signer := setupWFE(t)
	mux := wfe.Handler(metrics.NoopRegisterer)
	core.RandReader = fakeRand{}
	defer func() { core.RandReader = rand.Reader }()

	dirURL, _ := url.Parse("/directory")

	getReq := &http.Request{
		Method: http.MethodGet,
		URL:    dirURL,
		Host:   "localhost:4300",
	}

	_, _, jwsBody := signer.byKeyID(1, nil, "http://localhost/directory", "")
	postAsGetReq := makePostRequestWithPath("/directory", jwsBody)

	testCases := []struct {
		name         string
		caaIdent     string
		website      string
		expectedJSON string
		request      *http.Request
	}{
		{
			name:    "standard GET, no CAA ident/website meta",
			request: getReq,
			expectedJSON: `{
  "keyChange": "http://localhost:4300/acme/key-change",
  "meta": {
    "termsOfService": "http://example.invalid/terms",
		"profiles": {
			"default": "a test profile"
		}
  },
  "newNonce": "http://localhost:4300/acme/new-nonce",
  "newAccount": "http://localhost:4300/acme/new-acct",
  "newOrder": "http://localhost:4300/acme/new-order",
  "revokeCert": "http://localhost:4300/acme/revoke-cert",
  "AAAAAAAAAAA": "https://community.letsencrypt.org/t/adding-random-entries-to-the-directory/33417"
}`,
		},
		{
			name:     "standard GET, CAA ident/website meta",
			caaIdent: "Radiant Lock",
			website:  "zombo.com",
			request:  getReq,
			expectedJSON: `{
  "AAAAAAAAAAA": "https://community.letsencrypt.org/t/adding-random-entries-to-the-directory/33417",
  "keyChange": "http://localhost:4300/acme/key-change",
  "meta": {
    "caaIdentities": [
      "Radiant Lock"
    ],
    "termsOfService": "http://example.invalid/terms",
    "website": "zombo.com",
		"profiles": {
			"default": "a test profile"
		}
  },
  "newAccount": "http://localhost:4300/acme/new-acct",
  "newNonce": "http://localhost:4300/acme/new-nonce",
  "newOrder": "http://localhost:4300/acme/new-order",
  "revokeCert": "http://localhost:4300/acme/revoke-cert"
}`,
		},
		{
			name:     "POST-as-GET, CAA ident/website meta",
			caaIdent: "Radiant Lock",
			website:  "zombo.com",
			request:  postAsGetReq,
			expectedJSON: `{
  "AAAAAAAAAAA": "https://community.letsencrypt.org/t/adding-random-entries-to-the-directory/33417",
  "keyChange": "http://localhost/acme/key-change",
  "meta": {
    "caaIdentities": [
      "Radiant Lock"
    ],
    "termsOfService": "http://example.invalid/terms",
    "website": "zombo.com",
		"profiles": {
			"default": "a test profile"
		}
  },
  "newAccount": "http://localhost/acme/new-acct",
  "newNonce": "http://localhost/acme/new-nonce",
  "newOrder": "http://localhost/acme/new-order",
  "revokeCert": "http://localhost/acme/revoke-cert"
}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Configure a caaIdentity and website for the /directory meta based on the tc
			wfe.DirectoryCAAIdentity = tc.caaIdent // "Radiant Lock"
			wfe.DirectoryWebsite = tc.website      //"zombo.com"
			responseWriter := httptest.NewRecorder()
			// Serve the /directory response for this request into a recorder
			mux.ServeHTTP(responseWriter, tc.request)
			// We expect all directory requests to return a json object with a good HTTP status
			test.AssertEquals(t, responseWriter.Header().Get("Content-Type"), "application/json")
			// We expect all requests to return status OK
			test.AssertEquals(t, responseWriter.Code, http.StatusOK)
			// The response should match expected
			test.AssertUnmarshaledEquals(t, responseWriter.Body.String(), tc.expectedJSON)
			// Check that the random directory key is present
			test.AssertEquals(t,
				randomDirectoryKeyPresent(t, responseWriter.Body.Bytes()),
				true)
		})
	}
}

func TestRelativeDirectory(t *testing.T) {
	wfe, _, _ := setupWFE(t)
	mux := wfe.Handler(metrics.NoopRegisterer)
	core.RandReader = fakeRand{}
	defer func() { core.RandReader = rand.Reader }()

	expectedDirectory := func(hostname string) string {
		expected := new(bytes.Buffer)

		fmt.Fprintf(expected, "{")
		fmt.Fprintf(expected, `"keyChange":"%s/acme/key-change",`, hostname)
		fmt.Fprintf(expected, `"newNonce":"%s/acme/new-nonce",`, hostname)
		fmt.Fprintf(expected, `"newAccount":"%s/acme/new-acct",`, hostname)
		fmt.Fprintf(expected, `"newOrder":"%s/acme/new-order",`, hostname)
		fmt.Fprintf(expected, `"revokeCert":"%s/acme/revoke-cert",`, hostname)
		fmt.Fprintf(expected, `"AAAAAAAAAAA":"https://community.letsencrypt.org/t/adding-random-entries-to-the-directory/33417",`)
		fmt.Fprintf(expected, `"meta":{`)
		fmt.Fprintf(expected, `"termsOfService":"http://example.invalid/terms",`)
		fmt.Fprintf(expected, `"profiles":{"default":"a test profile"}`)
		fmt.Fprintf(expected, "}")
		fmt.Fprintf(expected, "}")
		return expected.String()
	}

	dirTests := []struct {
		host        string
		protoHeader string
		result      string
	}{
		// Test '' (No host header) with no proto header
		{"", "", expectedDirectory("http://localhost")},
		// Test localhost:4300 with no proto header
		{"localhost:4300", "", expectedDirectory("http://localhost:4300")},
		// Test 127.0.0.1:4300 with no proto header
		{"127.0.0.1:4300", "", expectedDirectory("http://127.0.0.1:4300")},
		// Test localhost:4300 with HTTP proto header
		{"localhost:4300", "http", expectedDirectory("http://localhost:4300")},
		// Test localhost:4300 with HTTPS proto header
		{"localhost:4300", "https", expectedDirectory("https://localhost:4300")},
	}

	for _, tt := range dirTests {
		var headers map[string][]string
		responseWriter := httptest.NewRecorder()

		if tt.protoHeader != "" {
			headers = map[string][]string{
				"X-Forwarded-Proto": {tt.protoHeader},
			}
		}

		mux.ServeHTTP(responseWriter, &http.Request{
			Method: "GET",
			Host:   tt.host,
			URL:    mustParseURL(directoryPath),
			Header: headers,
		})
		test.AssertEquals(t, responseWriter.Header().Get("Content-Type"), "application/json")
		test.AssertEquals(t, responseWriter.Code, http.StatusOK)
		test.AssertUnmarshaledEquals(t, responseWriter.Body.String(), tt.result)
	}
}

// TestNonceEndpoint tests requests to the WFE2's new-nonce endpoint
func TestNonceEndpoint(t *testing.T) {
	wfe, _, signer := setupWFE(t)
	mux := wfe.Handler(metrics.NoopRegisterer)

	getReq := &http.Request{
		Method: http.MethodGet,
		URL:    mustParseURL(newNoncePath),
	}
	headReq := &http.Request{
		Method: http.MethodHead,
		URL:    mustParseURL(newNoncePath),
	}

	_, _, jwsBody := signer.byKeyID(1, nil, fmt.Sprintf("http://localhost%s", newNoncePath), "")
	postAsGetReq := makePostRequestWithPath(newNoncePath, jwsBody)

	testCases := []struct {
		name           string
		request        *http.Request
		expectedStatus int
	}{
		{
			name:           "GET new-nonce request",
			request:        getReq,
			expectedStatus: http.StatusNoContent,
		},
		{
			name:           "HEAD new-nonce request",
			request:        headReq,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "POST-as-GET new-nonce request",
			request:        postAsGetReq,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			responseWriter := httptest.NewRecorder()
			mux.ServeHTTP(responseWriter, tc.request)
			// The response should have the expected HTTP status code
			test.AssertEquals(t, responseWriter.Code, tc.expectedStatus)
			// And the response should contain a valid nonce in the Replay-Nonce header
			nonce := responseWriter.Header().Get("Replay-Nonce")
			redeemResp, err := wfe.rnc.Redeem(context.Background(), &noncepb.NonceMessage{Nonce: nonce})
			test.AssertNotError(t, err, "redeeming nonce")
			test.AssertEquals(t, redeemResp.Valid, true)
			// The server MUST include a Cache-Control header field with the "no-store"
			// directive in responses for the newNonce resource, in order to prevent
			// caching of this resource.
			cacheControl := responseWriter.Header().Get("Cache-Control")
			test.AssertEquals(t, cacheControl, "no-store")
		})
	}
}

func TestHTTPMethods(t *testing.T) {
	wfe, _, _ := setupWFE(t)
	mux := wfe.Handler(metrics.NoopRegisterer)

	// NOTE: Boulder's muxer treats HEAD as implicitly allowed if GET is specified
	// so we include both here in `getOnly`
	getOnly := map[string]bool{http.MethodGet: true, http.MethodHead: true}
	postOnly := map[string]bool{http.MethodPost: true}
	getOrPost := map[string]bool{http.MethodGet: true, http.MethodHead: true, http.MethodPost: true}

	testCases := []struct {
		Name    string
		Path    string
		Allowed map[string]bool
	}{
		{
			Name:    "Index path should be GET only",
			Path:    "/",
			Allowed: getOnly,
		},
		{
			Name:    "Directory path should be GET or POST only",
			Path:    directoryPath,
			Allowed: getOrPost,
		},
		{
			Name:    "NewAcct path should be POST only",
			Path:    newAcctPath,
			Allowed: postOnly,
		},
		{
			Name:    "Acct path should be POST only",
			Path:    acctPath,
			Allowed: postOnly,
		},
		// TODO(@cpu): Remove GET authz support, support only POST-as-GET
		{
			Name:    "Authz path should be GET or POST only",
			Path:    authzPath,
			Allowed: getOrPost,
		},
		// TODO(@cpu): Remove GET challenge support, support only POST-as-GET
		{
			Name:    "Challenge path should be GET or POST only",
			Path:    challengePath,
			Allowed: getOrPost,
		},
		// TODO(@cpu): Remove GET certificate support, support only POST-as-GET
		{
			Name:    "Certificate path should be GET or POST only",
			Path:    certPath,
			Allowed: getOrPost,
		},
		{
			Name:    "RevokeCert path should be POST only",
			Path:    revokeCertPath,
			Allowed: postOnly,
		},
		{
			Name:    "Build ID path should be GET only",
			Path:    buildIDPath,
			Allowed: getOnly,
		},
		{
			Name:    "Rollover path should be POST only",
			Path:    rolloverPath,
			Allowed: postOnly,
		},
		{
			Name:    "New order path should be POST only",
			Path:    newOrderPath,
			Allowed: postOnly,
		},
		// TODO(@cpu): Remove GET order support, support only POST-as-GET
		{
			Name:    "Order path should be GET or POST only",
			Path:    orderPath,
			Allowed: getOrPost,
		},
		{
			Name:    "Nonce path should be GET or POST only",
			Path:    newNoncePath,
			Allowed: getOrPost,
		},
	}

	// NOTE: We omit http.MethodOptions because all requests with this method are
	// redirected to a special endpoint for CORS headers
	allMethods := []string{
		http.MethodGet,
		http.MethodHead,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodConnect,
		http.MethodTrace,
	}

	responseWriter := httptest.NewRecorder()

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			// For every possible HTTP method check what the mux serves for the test
			// case path
			for _, method := range allMethods {
				responseWriter.Body.Reset()
				mux.ServeHTTP(responseWriter, &http.Request{
					Method: method,
					URL:    mustParseURL(tc.Path),
				})
				// If the method isn't one that is intended to be allowed by the path,
				// check that the response was the not allowed response
				if _, ok := tc.Allowed[method]; !ok {
					var prob probs.ProblemDetails
					// Unmarshal the body into a problem
					body := responseWriter.Body.String()
					err := json.Unmarshal([]byte(body), &prob)
					test.AssertNotError(t, err, fmt.Sprintf("Error unmarshalling resp body: %q", body))
					// TODO(@cpu): It seems like the mux should be returning
					// http.StatusMethodNotAllowed here, but instead it returns StatusOK
					// with a problem that has a StatusMethodNotAllowed HTTPStatus. Is
					// this a bug?
					test.AssertEquals(t, responseWriter.Code, http.StatusOK)
					test.AssertEquals(t, prob.HTTPStatus, http.StatusMethodNotAllowed)
					test.AssertEquals(t, prob.Detail, "Method not allowed")
				} else {
					// Otherwise if it was an allowed method, ensure that the response was
					// *not* StatusMethodNotAllowed
					test.AssertNotEquals(t, responseWriter.Code, http.StatusMethodNotAllowed)
				}
			}
		})
	}
}

func TestGetChallengeHandler(t *testing.T) {
	wfe, _, _ := setupWFE(t)

	// The slug "7TyhFQ" is the StringID of a challenge with type "http-01" and
	// token "token".
	challSlug := "7TyhFQ"

	for _, method := range []string{"GET", "HEAD"} {
		resp := httptest.NewRecorder()

		// We set req.URL.Path separately to emulate the path-stripping that
		// Boulder's request handler does.
		challengeURL := fmt.Sprintf("http://localhost/acme/chall/1/1/%s", challSlug)
		req, err := http.NewRequest(method, challengeURL, nil)
		test.AssertNotError(t, err, "Could not make NewRequest")
		req.URL.Path = fmt.Sprintf("1/1/%s", challSlug)

		wfe.ChallengeHandler(ctx, newRequestEvent(), resp, req)
		test.AssertEquals(t, resp.Code, http.StatusOK)
		test.AssertEquals(t, resp.Header().Get("Location"), challengeURL)
		test.AssertEquals(t, resp.Header().Get("Content-Type"), "application/json")
		test.AssertEquals(t, resp.Header().Get("Link"), `<http://localhost/acme/authz/1/1>;rel="up"`)

		// Body is only relevant for GET. For HEAD, body will
		// be discarded by HandleFunc() anyway, so it doesn't
		// matter what Challenge() writes to it.
		if method == "GET" {
			test.AssertUnmarshaledEquals(
				t, resp.Body.String(),
				`{"status": "valid", "type":"http-01","token":"token","url":"http://localhost/acme/chall/1/1/7TyhFQ"}`)
		}
	}
}

func TestChallengeHandler(t *testing.T) {
	wfe, _, signer := setupWFE(t)

	post := func(path string) *http.Request {
		signedURL := fmt.Sprintf("http://localhost/%s", path)
		_, _, jwsBody := signer.byKeyID(1, nil, signedURL, `{}`)
		return makePostRequestWithPath(path, jwsBody)
	}
	postAsGet := func(keyID int64, path, body string) *http.Request {
		_, _, jwsBody := signer.byKeyID(keyID, nil, fmt.Sprintf("http://localhost/%s", path), body)
		return makePostRequestWithPath(path, jwsBody)
	}

	testCases := []struct {
		Name            string
		Request         *http.Request
		ExpectedStatus  int
		ExpectedHeaders map[string]string
		ExpectedBody    string
	}{
		{
			Name:           "Valid challenge",
			Request:        post("1/1/7TyhFQ"),
			ExpectedStatus: http.StatusOK,
			ExpectedHeaders: map[string]string{
				"Content-Type": "application/json",
				"Location":     "http://localhost/acme/chall/1/1/7TyhFQ",
				"Link":         `<http://localhost/acme/authz/1/1>;rel="up"`,
			},
			ExpectedBody: `{"status": "valid", "type":"http-01","token":"token","url":"http://localhost/acme/chall/1/1/7TyhFQ"}`,
		},
		{
			Name:           "Expired challenge",
			Request:        post("1/3/7TyhFQ"),
			ExpectedStatus: http.StatusNotFound,
			ExpectedBody:   `{"type":"` + probs.ErrorNS + `malformed","detail":"Expired authorization","status":404}`,
		},
		{
			Name:           "Missing challenge",
			Request:        post("1/1/"),
			ExpectedStatus: http.StatusNotFound,
			ExpectedBody:   `{"type":"` + probs.ErrorNS + `malformed","detail":"No such challenge","status":404}`,
		},
		{
			Name:           "Unspecified database error",
			Request:        post("1/4/7TyhFQ"),
			ExpectedStatus: http.StatusInternalServerError,
			ExpectedBody:   `{"type":"` + probs.ErrorNS + `serverInternal","detail":"Problem getting authorization","status":500}`,
		},
		{
			Name:           "POST-as-GET, wrong owner",
			Request:        postAsGet(1, "1/5/7TyhFQ", ""),
			ExpectedStatus: http.StatusForbidden,
			ExpectedBody:   `{"type":"` + probs.ErrorNS + `unauthorized","detail":"User account ID doesn't match account ID in authorization","status":403}`,
		},
		{
			Name:           "Valid POST-as-GET",
			Request:        postAsGet(1, "1/1/7TyhFQ", ""),
			ExpectedStatus: http.StatusOK,
			ExpectedBody:   `{"status": "valid", "type":"http-01", "token":"token", "url": "http://localhost/acme/chall/1/1/7TyhFQ"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			responseWriter := httptest.NewRecorder()
			wfe.ChallengeHandler(ctx, newRequestEvent(), responseWriter, tc.Request)
			// Check the response code, headers and body match expected
			headers := responseWriter.Header()
			body := responseWriter.Body.String()
			test.AssertEquals(t, responseWriter.Code, tc.ExpectedStatus)
			for h, v := range tc.ExpectedHeaders {
				test.AssertEquals(t, headers.Get(h), v)
			}
			test.AssertUnmarshaledEquals(t, body, tc.ExpectedBody)
		})
	}
}

// MockRAPerformValidationError is a mock RA that just returns an error on
// PerformValidation.
type MockRAPerformValidationError struct {
	MockRegistrationAuthority
}

func (ra *MockRAPerformValidationError) PerformValidation(context.Context, *rapb.PerformValidationRequest, ...grpc.CallOption) (*corepb.Authorization, error) {
	return nil, errors.New("broken on purpose")
}

// TestUpdateChallengeHandlerFinalizedAuthz tests that POSTing a challenge associated
// with an already valid authorization just returns the challenge without calling
// the RA.
func TestUpdateChallengeHandlerFinalizedAuthz(t *testing.T) {
	wfe, fc, signer := setupWFE(t)
	wfe.ra = &MockRAPerformValidationError{MockRegistrationAuthority{clk: fc}}
	responseWriter := httptest.NewRecorder()

	signedURL := "http://localhost/1/1/7TyhFQ"
	_, _, jwsBody := signer.byKeyID(1, nil, signedURL, `{}`)
	request := makePostRequestWithPath("1/1/7TyhFQ", jwsBody)
	wfe.ChallengeHandler(ctx, newRequestEvent(), responseWriter, request)

	body := responseWriter.Body.String()
	test.AssertUnmarshaledEquals(t, body, `{
	  "status": "valid",
		"type": "http-01",
		"token": "token",
		"url": "http://localhost/acme/chall/1/1/7TyhFQ"
	  }`)
}

// TestUpdateChallengeHandlerRAError tests that when the RA returns an error from
// PerformValidation that the WFE returns an internal server error as expected
// and does not panic or otherwise bug out.
func TestUpdateChallengeHandlerRAError(t *testing.T) {
	wfe, fc, signer := setupWFE(t)
	// Mock the RA to always fail PerformValidation
	wfe.ra = &MockRAPerformValidationError{MockRegistrationAuthority{clk: fc}}

	// Update a pending challenge
	signedURL := "http://localhost/1/2/7TyhFQ"
	_, _, jwsBody := signer.byKeyID(1, nil, signedURL, `{}`)
	responseWriter := httptest.NewRecorder()
	request := makePostRequestWithPath("1/2/7TyhFQ", jwsBody)

	wfe.ChallengeHandler(ctx, newRequestEvent(), responseWriter, request)

	// The result should be an internal server error problem.
	body := responseWriter.Body.String()
	test.AssertUnmarshaledEquals(t, body, `{
		"type": "urn:ietf:params:acme:error:serverInternal",
	  "detail": "Unable to update challenge",
		"status": 500
	}`)
}

func TestBadNonce(t *testing.T) {
	wfe, _, _ := setupWFE(t)

	key := loadKey(t, []byte(test2KeyPrivatePEM))
	rsaKey, ok := key.(*rsa.PrivateKey)
	test.Assert(t, ok, "Couldn't load RSA key")
	// NOTE: We deliberately do not set the NonceSource in the jose.SignerOptions
	// for this test in order to provoke a bad nonce error
	noNonceSigner, err := jose.NewSigner(jose.SigningKey{
		Key:       rsaKey,
		Algorithm: jose.RS256,
	}, &jose.SignerOptions{
		EmbedJWK: true,
	})
	test.AssertNotError(t, err, "Failed to make signer")

	responseWriter := httptest.NewRecorder()
	result, err := noNonceSigner.Sign([]byte(`{"contact":["mailto:person@mail.com"]}`))
	test.AssertNotError(t, err, "Failed to sign body")
	wfe.NewAccount(ctx, newRequestEvent(), responseWriter,
		makePostRequestWithPath("nonce", result.FullSerialize()))
	test.AssertUnmarshaledEquals(t, responseWriter.Body.String(), `{"type":"`+probs.ErrorNS+`badNonce","detail":"Unable to validate JWS :: JWS has no anti-replay nonce","status":400}`)
}

func TestNewECDSAAccount(t *testing.T) {
	wfe, _, signer := setupWFE(t)

	// E1 always exists; E2 never exists
	key := loadKey(t, []byte(testE2KeyPrivatePEM))
	_, ok := key.(*ecdsa.PrivateKey)
	test.Assert(t, ok, "Couldn't load ECDSA key")

	payload := `{"contact":["mailto:person@mail.com"],"termsOfServiceAgreed":true}`
	path := newAcctPath
	signedURL := fmt.Sprintf("http://localhost%s", path)
	_, _, body := signer.embeddedJWK(key, signedURL, payload)
	request := makePostRequestWithPath(path, body)

	responseWriter := httptest.NewRecorder()
	wfe.NewAccount(ctx, newRequestEvent(), responseWriter, request)

	var acct core.Registration
	responseBody := responseWriter.Body.String()
	err := json.Unmarshal([]byte(responseBody), &acct)
	test.AssertNotError(t, err, "Couldn't unmarshal returned account object")
	test.AssertEquals(t, acct.Agreement, "")

	test.AssertEquals(t, responseWriter.Header().Get("Location"), "http://localhost/acme/acct/1")

	key = loadKey(t, []byte(testE1KeyPrivatePEM))
	_, ok = key.(*ecdsa.PrivateKey)
	test.Assert(t, ok, "Couldn't load ECDSA key")

	_, _, body = signer.embeddedJWK(key, signedURL, payload)
	request = makePostRequestWithPath(path, body)

	// Reset the body and status code
	responseWriter = httptest.NewRecorder()
	// POST, Valid JSON, Key already in use
	wfe.NewAccount(ctx, newRequestEvent(), responseWriter, request)
	test.AssertUnmarshaledEquals(t, responseWriter.Body.String(),
		`{
		"key": {
			"kty": "EC",
			"crv": "P-256",
			"x": "FwvSZpu06i3frSk_mz9HcD9nETn4wf3mQ-zDtG21Gao",
			"y": "S8rR-0dWa8nAcw1fbunF_ajS3PQZ-QwLps-2adgLgPk"
		},
		"status": ""
		}`)
	test.AssertEquals(t, responseWriter.Header().Get("Location"), "http://localhost/acme/acct/3")
	test.AssertEquals(t, responseWriter.Code, 200)

	// test3KeyPrivatePEM is a private key corresponding to a deactivated account in the mock SA's GetRegistration test data.
	key = loadKey(t, []byte(test3KeyPrivatePEM))
	_, ok = key.(*rsa.PrivateKey)
	test.Assert(t, ok, "Couldn't load test3 key")

	// Reset the body and status code
	responseWriter = httptest.NewRecorder()

	// Test POST valid JSON with deactivated account
	payload = `{}`
	path = "1"
	signedURL = "http://localhost/1"
	_, _, body = signer.embeddedJWK(key, signedURL, payload)
	request = makePostRequestWithPath(path, body)
	wfe.NewAccount(ctx, newRequestEvent(), responseWriter, request)
	test.AssertEquals(t, responseWriter.Code, http.StatusForbidden)
}

// Test that the WFE handling of the "empty update" POST is correct. The ACME
// spec describes how when clients wish to query the server for information
// about an account an empty account update should be sent, and
// a populated acct object will be returned.
func TestEmptyAccount(t *testing.T) {
	wfe, _, signer := setupWFE(t)

	// Test Key 1 is mocked in the mock StorageAuthority used in setupWFE to
	// return a populated account for GetRegistrationByKey when test key 1 is
	// used.
	key := loadKey(t, []byte(test1KeyPrivatePEM))
	_, ok := key.(*rsa.PrivateKey)
	test.Assert(t, ok, "Couldn't load RSA key")

	path := "1"
	signedURL := "http://localhost/1"

	testCases := []struct {
		Name           string
		Payload        string
		ExpectedStatus int
	}{
		{
			Name:           "POST empty string to acct",
			Payload:        "",
			ExpectedStatus: http.StatusOK,
		},
		{
			Name:           "POST empty JSON object to acct",
			Payload:        "{}",
			ExpectedStatus: http.StatusOK,
		},
		{
			Name:           "POST invalid empty JSON string to acct",
			Payload:        "\"\"",
			ExpectedStatus: http.StatusBadRequest,
		},
		{
			Name:           "POST invalid empty JSON array to acct",
			Payload:        "[]",
			ExpectedStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			responseWriter := httptest.NewRecorder()

			_, _, body := signer.byKeyID(1, key, signedURL, tc.Payload)
			request := makePostRequestWithPath(path, body)

			// Send an account update with the trivial body
			wfe.Account(
				ctx,
				newRequestEvent(),
				responseWriter,
				request)

			responseBody := responseWriter.Body.String()
			test.AssertEquals(t, responseWriter.Code, tc.ExpectedStatus)

			// If success is expected, we should get back a populated Account
			if tc.ExpectedStatus == http.StatusOK {
				var acct core.Registration
				err := json.Unmarshal([]byte(responseBody), &acct)
				test.AssertNotError(t, err, "Couldn't unmarshal returned account object")
				test.AssertEquals(t, acct.Agreement, "")
			}

			responseWriter.Body.Reset()
		})
	}
}

func TestNewAccount(t *testing.T) {
	wfe, _, signer := setupWFE(t)
	mux := wfe.Handler(metrics.NoopRegisterer)
	key := loadKey(t, []byte(test2KeyPrivatePEM))
	_, ok := key.(*rsa.PrivateKey)
	test.Assert(t, ok, "Couldn't load test2 key")

	path := newAcctPath
	signedURL := fmt.Sprintf("http://localhost%s", path)

	wrongAgreementAcct := `{"contact":["mailto:person@mail.com"],"termsOfServiceAgreed":false}`
	// An acct with the terms not agreed to
	_, _, wrongAgreementBody := signer.embeddedJWK(key, signedURL, wrongAgreementAcct)

	// A non-JSON payload
	_, _, fooBody := signer.embeddedJWK(key, signedURL, `foo`)

	type newAcctErrorTest struct {
		r        *http.Request
		respBody string
	}

	acctErrTests := []newAcctErrorTest{
		// POST, but no body.
		{
			&http.Request{
				Method: "POST",
				URL:    mustParseURL(newAcctPath),
				Header: map[string][]string{
					"Content-Length": {"0"},
					"Content-Type":   {expectedJWSContentType},
				},
			},
			`{"type":"` + probs.ErrorNS + `malformed","detail":"Unable to validate JWS :: No body on POST","status":400}`,
		},

		// POST, but body that isn't valid JWS
		{
			makePostRequestWithPath(newAcctPath, "hi"),
			`{"type":"` + probs.ErrorNS + `malformed","detail":"Unable to validate JWS :: Parse error reading JWS","status":400}`,
		},

		// POST, Properly JWS-signed, but payload is "foo", not base64-encoded JSON.
		{
			makePostRequestWithPath(newAcctPath, fooBody),
			`{"type":"` + probs.ErrorNS + `malformed","detail":"Unable to validate JWS :: Request payload did not parse as JSON","status":400}`,
		},

		// Same signed body, but payload modified by one byte, breaking signature.
		// should fail JWS verification.
		{
			makePostRequestWithPath(newAcctPath,
				`{"payload":"Zm9x","protected":"eyJhbGciOiJSUzI1NiIsImp3ayI6eyJrdHkiOiJSU0EiLCJuIjoicW5BUkxyVDdYejRnUmNLeUxkeWRtQ3ItZXk5T3VQSW1YNFg0MHRoazNvbjI2RmtNem5SM2ZSanM2NmVMSzdtbVBjQlo2dU9Kc2VVUlU2d0FhWk5tZW1vWXgxZE12cXZXV0l5aVFsZUhTRDdROHZCcmhSNnVJb080akF6SlpSLUNoelp1U0R0N2lITi0zeFVWc3B1NVhHd1hVX01WSlpzaFR3cDRUYUZ4NWVsSElUX09iblR2VE9VM1hoaXNoMDdBYmdaS21Xc1ZiWGg1cy1DcklpY1U0T2V4SlBndW5XWl9ZSkp1ZU9LbVR2bkxsVFY0TXpLUjJvWmxCS1oyN1MwLVNmZFZfUUR4X3lkbGU1b01BeUtWdGxBVjM1Y3lQTUlzWU53Z1VHQkNkWV8yVXppNWVYMGxUYzdNUFJ3ejZxUjFraXAtaTU5VmNHY1VRZ3FIVjZGeXF3IiwiZSI6IkFRQUIifSwia2lkIjoiIiwibm9uY2UiOiJyNHpuenZQQUVwMDlDN1JwZUtYVHhvNkx3SGwxZVBVdmpGeXhOSE1hQnVvIiwidXJsIjoiaHR0cDovL2xvY2FsaG9zdC9hY21lL25ldy1yZWcifQ","signature":"jcTdxSygm_cvD7KbXqsxgnoPApCTSkV4jolToSOd2ciRkg5W7Yl0ZKEEKwOc-dYIbQiwGiDzisyPCicwWsOUA1WSqHylKvZ3nxSMc6KtwJCW2DaOqcf0EEjy5VjiZJUrOt2c-r6b07tbn8sfOJKwlF2lsOeGi4s-rtvvkeQpAU-AWauzl9G4bv2nDUeCviAZjHx_PoUC-f9GmZhYrbDzAvXZ859ktM6RmMeD0OqPN7bhAeju2j9Gl0lnryZMtq2m0J2m1ucenQBL1g4ZkP1JiJvzd2cAz5G7Ftl2YeJJyWhqNd3qq0GVOt1P11s8PTGNaSoM0iR9QfUxT9A6jxARtg"}`),
			`{"type":"` + probs.ErrorNS + `malformed","detail":"Unable to validate JWS :: JWS verification error","status":400}`,
		},
		{
			makePostRequestWithPath(newAcctPath, wrongAgreementBody),
			`{"type":"` + probs.ErrorNS + `malformed","detail":"must agree to terms of service","status":400}`,
		},
	}
	for _, rt := range acctErrTests {
		responseWriter := httptest.NewRecorder()
		mux.ServeHTTP(responseWriter, rt.r)
		test.AssertUnmarshaledEquals(t, responseWriter.Body.String(), rt.respBody)
	}

	responseWriter := httptest.NewRecorder()

	payload := `{"contact":["mailto:person@mail.com"],"termsOfServiceAgreed":true}`
	_, _, body := signer.embeddedJWK(key, signedURL, payload)
	request := makePostRequestWithPath(path, body)

	wfe.NewAccount(ctx, newRequestEvent(), responseWriter, request)

	var acct core.Registration
	responseBody := responseWriter.Body.String()
	err := json.Unmarshal([]byte(responseBody), &acct)
	test.AssertNotError(t, err, "Couldn't unmarshal returned account object")
	// Agreement is an ACMEv1 field and should not be present
	test.AssertEquals(t, acct.Agreement, "")

	test.AssertEquals(
		t, responseWriter.Header().Get("Location"),
		"http://localhost/acme/acct/1")

	// Load an existing key
	key = loadKey(t, []byte(test1KeyPrivatePEM))
	_, ok = key.(*rsa.PrivateKey)
	test.Assert(t, ok, "Couldn't load test1 key")

	// Reset the body and status code
	responseWriter = httptest.NewRecorder()
	// POST, Valid JSON, Key already in use
	_, _, body = signer.embeddedJWK(key, signedURL, payload)
	request = makePostRequestWithPath(path, body)
	// POST the NewAccount request
	wfe.NewAccount(ctx, newRequestEvent(), responseWriter, request)
	// We expect a Location header and a 200 response with an empty body
	test.AssertEquals(
		t, responseWriter.Header().Get("Location"),
		"http://localhost/acme/acct/1")
	test.AssertEquals(t, responseWriter.Code, 200)
	test.AssertUnmarshaledEquals(t, responseWriter.Body.String(),
		`{
		"key": {
			"kty": "RSA",
			"n": "yNWVhtYEKJR21y9xsHV-PD_bYwbXSeNuFal46xYxVfRL5mqha7vttvjB_vc7Xg2RvgCxHPCqoxgMPTzHrZT75LjCwIW2K_klBYN8oYvTwwmeSkAz6ut7ZxPv-nZaT5TJhGk0NT2kh_zSpdriEJ_3vW-mqxYbbBmpvHqsa1_zx9fSuHYctAZJWzxzUZXykbWMWQZpEiE0J4ajj51fInEzVn7VxV-mzfMyboQjujPh7aNJxAWSq4oQEJJDgWwSh9leyoJoPpONHxh5nEE5AjE01FkGICSxjpZsF-w8hOTI3XXohUdu29Se26k2B0PolDSuj0GIQU6-W9TdLXSjBb2SpQ",
			"e": "AQAB"
		},
		"contact": [
			"mailto:person@mail.com"
		],
		"status": "valid"
	}`)
}

func TestNewAccountWhenAccountHasBeenDeactivated(t *testing.T) {
	wfe, _, signer := setupWFE(t)
	signedURL := fmt.Sprintf("http://localhost%s", newAcctPath)
	// test3KeyPrivatePEM is a private key corresponding to a deactivated account in the mock SA's GetRegistration test data.
	k := loadKey(t, []byte(test3KeyPrivatePEM))
	_, ok := k.(*rsa.PrivateKey)
	test.Assert(t, ok, "Couldn't load test3 key")

	payload := `{"contact":["mailto:person@mail.com"],"termsOfServiceAgreed":true}`
	_, _, body := signer.embeddedJWK(k, signedURL, payload)
	request := makePostRequestWithPath(newAcctPath, body)

	responseWriter := httptest.NewRecorder()
	wfe.NewAccount(ctx, newRequestEvent(), responseWriter, request)

	test.AssertEquals(t, responseWriter.Code, http.StatusForbidden)
}

func TestNewAccountNoID(t *testing.T) {
	wfe, _, signer := setupWFE(t)
	key := loadKey(t, []byte(test2KeyPrivatePEM))
	_, ok := key.(*rsa.PrivateKey)
	test.Assert(t, ok, "Couldn't load test2 key")
	path := newAcctPath
	signedURL := fmt.Sprintf("http://localhost%s", path)

	payload := `{"contact":["mailto:person@mail.com"],"termsOfServiceAgreed":true}`
	_, _, body := signer.embeddedJWK(key, signedURL, payload)
	request := makePostRequestWithPath(path, body)

	responseWriter := httptest.NewRecorder()
	wfe.NewAccount(ctx, newRequestEvent(), responseWriter, request)

	responseBody := responseWriter.Body.String()
	test.AssertUnmarshaledEquals(t, responseBody, `{
		"key": {
			"kty": "RSA",
			"n": "qnARLrT7Xz4gRcKyLdydmCr-ey9OuPImX4X40thk3on26FkMznR3fRjs66eLK7mmPcBZ6uOJseURU6wAaZNmemoYx1dMvqvWWIyiQleHSD7Q8vBrhR6uIoO4jAzJZR-ChzZuSDt7iHN-3xUVspu5XGwXU_MVJZshTwp4TaFx5elHIT_ObnTvTOU3Xhish07AbgZKmWsVbXh5s-CrIicU4OexJPgunWZ_YJJueOKmTvnLlTV4MzKR2oZlBKZ27S0-SfdV_QDx_ydle5oMAyKVtlAV35cyPMIsYNwgUGBCdY_2Uzi5eX0lTc7MPRwz6qR1kip-i59VcGcUQgqHV6Fyqw",
			"e": "AQAB"
		},
		"createdAt": "2021-01-01T00:00:00Z",
		"status": ""
	}`)
}

func TestContactsToEmails(t *testing.T) {
	t.Parallel()
	wfe, _, _ := setupWFE(t)

	for _, tc := range []struct {
		name     string
		contacts []string
		want     []string
		wantErr  string
	}{
		{
			name:     "no contacts",
			contacts: []string{},
			want:     []string{},
		},
		{
			name:     "happy path",
			contacts: []string{"mailto:one@mail.com", "mailto:two@mail.com"},
			want:     []string{"one@mail.com", "two@mail.com"},
		},
		{
			name:     "empty url",
			contacts: []string{""},
			wantErr:  "empty contact",
		},
		{
			name:     "too many contacts",
			contacts: []string{"mailto:one@mail.com", "mailto:two@mail.com", "mailto:three@mail.com"},
			wantErr:  "too many contacts",
		},
		{
			name:     "unknown scheme",
			contacts: []string{"ansible:earth.sol.milkyway.laniakea/letsencrypt"},
			wantErr:  "contact scheme",
		},
		{
			name:     "malformed email",
			contacts: []string{"mailto:admin.com"},
			wantErr:  "unable to parse email address",
		},
		{
			name:     "non-ascii email",
			contacts: []string{"mailto:señor@email.com"},
			wantErr:  "contains non-ASCII characters",
		},
		{
			name:     "unarseable email",
			contacts: []string{"mailto:a@mail.com, b@mail.com"},
			wantErr:  "unable to parse email address",
		},
		{
			name:     "forbidden example domain",
			contacts: []string{"mailto:a@example.org"},
			wantErr:  "forbidden",
		},
		{
			name:     "forbidden non-public domain",
			contacts: []string{"mailto:admin@localhost"},
			wantErr:  "needs at least one dot",
		},
		{
			name:     "forbidden non-iana domain",
			contacts: []string{"mailto:admin@non.iana.suffix"},
			wantErr:  "does not end with a valid public suffix",
		},
		{
			name:     "forbidden ip domain",
			contacts: []string{"mailto:admin@1.2.3.4"},
			wantErr:  "value is an IP address",
		},
		{
			name:     "forbidden bracketed ip domain",
			contacts: []string{"mailto:admin@[1.2.3.4]"},
			wantErr:  "contains an invalid character",
		},
		{
			name:     "query parameter",
			contacts: []string{"mailto:admin@a.com?no-reminder-emails"},
			wantErr:  "contains a question mark",
		},
		{
			name:     "empty query parameter",
			contacts: []string{"mailto:admin@a.com?"},
			wantErr:  "contains a question mark",
		},
		{
			name:     "fragment url",
			contacts: []string{"mailto:admin@a.com#optional"},
			wantErr:  "contains a '#'",
		},
		{
			name:     "empty fragment url",
			contacts: []string{"mailto:admin@a.com#"},
			wantErr:  "contains a '#'",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := wfe.contactsToEmails(tc.contacts)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("contactsToEmails(%#v) = nil, but want %q", tc.contacts, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("contactsToEmails(%#v) = %q, but want %q", tc.contacts, err.Error(), tc.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("contactsToEmails(%#v) = %q, but want %#v", tc.contacts, err.Error(), tc.want)
				}
				if !slices.Equal(got, tc.want) {
					t.Errorf("contactsToEmails(%#v) = %#v, but want %#v", tc.contacts, got, tc.want)
				}
			}
		})
	}
}

func TestGetAuthorizationHandler(t *testing.T) {
	wfe, _, signer := setupWFE(t)

	// Expired authorizations should be inaccessible
	authzURL := "1/3"
	responseWriter := httptest.NewRecorder()
	wfe.AuthorizationHandler(ctx, newRequestEvent(), responseWriter, &http.Request{
		Method: "GET",
		URL:    mustParseURL(authzURL),
	})
	test.AssertEquals(t, responseWriter.Code, http.StatusNotFound)
	test.AssertUnmarshaledEquals(t, responseWriter.Body.String(),
		`{"type":"`+probs.ErrorNS+`malformed","detail":"Expired authorization","status":404}`)
	responseWriter.Body.Reset()

	// Ensure that a valid authorization can't be reached with an invalid URL
	wfe.AuthorizationHandler(ctx, newRequestEvent(), responseWriter, &http.Request{
		URL:    mustParseURL("1/1d"),
		Method: "GET",
	})
	test.AssertUnmarshaledEquals(t, responseWriter.Body.String(),
		`{"type":"`+probs.ErrorNS+`malformed","detail":"Invalid authorization ID","status":400}`)

	_, _, jwsBody := signer.byKeyID(1, nil, "http://localhost/1/1", "")
	postAsGet := makePostRequestWithPath("1/1", jwsBody)

	responseWriter = httptest.NewRecorder()
	// Ensure that a POST-as-GET to an authorization works
	wfe.AuthorizationHandler(ctx, newRequestEvent(), responseWriter, postAsGet)
	test.AssertEquals(t, responseWriter.Code, http.StatusOK)
	body := responseWriter.Body.String()
	test.AssertUnmarshaledEquals(t, body, `
	{
		"identifier": {
			"type": "dns",
			"value": "not-an-example.com"
		},
		"status": "valid",
		"expires": "2070-01-01T00:00:00Z",
		"challenges": [
			{
			  "status": "valid",
				"type": "http-01",
				"token":"token",
				"url": "http://localhost/acme/chall/1/1/7TyhFQ"
			}
		]
	}`)
}

// TestAuthorizationHandler500 tests that internal errors on GetAuthorization result in
// a 500.
func TestAuthorizationHandler500(t *testing.T) {
	wfe, _, _ := setupWFE(t)

	responseWriter := httptest.NewRecorder()
	wfe.AuthorizationHandler(ctx, newRequestEvent(), responseWriter, &http.Request{
		Method: "GET",
		URL:    mustParseURL("1/4"),
	})
	expected := `{
         "type": "urn:ietf:params:acme:error:serverInternal",
				 "detail": "Problem getting authorization",
				 "status": 500
  }`
	test.AssertUnmarshaledEquals(t, responseWriter.Body.String(), expected)
}

// RAWithFailedChallenges is a fake RA whose GetAuthorization method returns
// an authz with a failed challenge.
type RAWithFailedChallenge struct {
	rapb.RegistrationAuthorityClient
	clk clock.Clock
}

func (ra *RAWithFailedChallenge) GetAuthorization(ctx context.Context, id *rapb.GetAuthorizationRequest, _ ...grpc.CallOption) (*corepb.Authorization, error) {
	return &corepb.Authorization{
		Id:             "6",
		RegistrationID: 1,
		Identifier:     identifier.NewDNS("not-an-example.com").ToProto(),
		Status:         string(core.StatusInvalid),
		Expires:        timestamppb.New(ra.clk.Now().AddDate(100, 0, 0)),
		Challenges: []*corepb.Challenge{
			{
				Id:     1,
				Type:   "http-01",
				Status: string(core.StatusInvalid),
				Token:  "token",
				Error: &corepb.ProblemDetails{
					ProblemType: "things:are:whack",
					Detail:      "whack attack",
					HttpStatus:  555,
				},
			},
		},
	}, nil
}

// TestAuthorizationChallengeHandlerNamespace tests that the runtime prefixing of
// Challenge Problem Types works as expected
func TestAuthorizationChallengeHandlerNamespace(t *testing.T) {
	wfe, clk, _ := setupWFE(t)
	wfe.ra = &RAWithFailedChallenge{clk: clk}

	responseWriter := httptest.NewRecorder()
	wfe.AuthorizationHandler(ctx, newRequestEvent(), responseWriter, &http.Request{
		Method: "GET",
		URL:    mustParseURL("1/6"),
	})

	var authz core.Authorization
	err := json.Unmarshal(responseWriter.Body.Bytes(), &authz)
	test.AssertNotError(t, err, "Couldn't unmarshal returned authorization object")
	test.AssertEquals(t, len(authz.Challenges), 1)
	// The Challenge Error Type should have had the probs.ErrorNS prefix added
	test.AssertEquals(t, string(authz.Challenges[0].Error.Type), probs.ErrorNS+"things:are:whack")
	responseWriter.Body.Reset()
}

func TestAccount(t *testing.T) {
	wfe, _, signer := setupWFE(t)
	mux := wfe.Handler(metrics.NoopRegisterer)
	responseWriter := httptest.NewRecorder()

	// Test GET proper entry returns 405
	mux.ServeHTTP(responseWriter, &http.Request{
		Method: "GET",
		URL:    mustParseURL(acctPath),
	})
	test.AssertUnmarshaledEquals(t,
		responseWriter.Body.String(),
		`{"type":"`+probs.ErrorNS+`malformed","detail":"Method not allowed","status":405}`)
	responseWriter.Body.Reset()

	// Test POST invalid JSON
	wfe.Account(ctx, newRequestEvent(), responseWriter, makePostRequestWithPath("2", "invalid"))
	test.AssertUnmarshaledEquals(t,
		responseWriter.Body.String(),
		`{"type":"`+probs.ErrorNS+`malformed","detail":"Unable to validate JWS :: Parse error reading JWS","status":400}`)
	responseWriter.Body.Reset()

	key := loadKey(t, []byte(test2KeyPrivatePEM))
	_, ok := key.(*rsa.PrivateKey)
	test.Assert(t, ok, "Couldn't load RSA key")

	signedURL := fmt.Sprintf("http://localhost%s%d", acctPath, 102)
	path := fmt.Sprintf("%s%d", acctPath, 102)
	payload := `{}`
	// ID 102 is used by the mock for missing acct
	_, _, body := signer.byKeyID(102, nil, signedURL, payload)
	request := makePostRequestWithPath(path, body)

	// Test POST valid JSON but key is not registered
	wfe.Account(ctx, newRequestEvent(), responseWriter, request)
	test.AssertUnmarshaledEquals(t,
		responseWriter.Body.String(),
		`{"type":"`+probs.ErrorNS+`accountDoesNotExist","detail":"Unable to validate JWS :: Account \"http://localhost/acme/acct/102\" not found","status":400}`)
	responseWriter.Body.Reset()

	key = loadKey(t, []byte(test1KeyPrivatePEM))
	_, ok = key.(*rsa.PrivateKey)
	test.Assert(t, ok, "Couldn't load RSA key")

	// Test POST valid JSON with account up in the mock
	payload = `{}`
	path = "1"
	signedURL = "http://localhost/1"
	_, _, body = signer.byKeyID(1, nil, signedURL, payload)
	request = makePostRequestWithPath(path, body)

	wfe.Account(ctx, newRequestEvent(), responseWriter, request)
	test.AssertNotContains(t, responseWriter.Body.String(), probs.ErrorNS)
	links := responseWriter.Header()["Link"]
	test.AssertEquals(t, slices.Contains(links, "<"+agreementURL+">;rel=\"terms-of-service\""), true)
	responseWriter.Body.Reset()

	// Test POST valid JSON with garbage in URL but valid account ID
	payload = `{}`
	signedURL = "http://localhost/a/bunch/of/garbage/1"
	_, _, body = signer.byKeyID(1, nil, signedURL, payload)
	request = makePostRequestWithPath("/a/bunch/of/garbage/1", body)

	wfe.Account(ctx, newRequestEvent(), responseWriter, request)
	test.AssertContains(t, responseWriter.Body.String(), "400")
	test.AssertContains(t, responseWriter.Body.String(), probs.ErrorNS+"malformed")
	responseWriter.Body.Reset()

	// Test valid POST-as-GET request
	responseWriter = httptest.NewRecorder()
	_, _, body = signer.byKeyID(1, nil, "http://localhost/1", "")
	request = makePostRequestWithPath("1", body)
	wfe.Account(ctx, newRequestEvent(), responseWriter, request)
	// It should not error
	test.AssertNotContains(t, responseWriter.Body.String(), probs.ErrorNS)
	test.AssertEquals(t, responseWriter.Code, http.StatusOK)

	altKey := loadKey(t, []byte(test2KeyPrivatePEM))
	_, ok = altKey.(*rsa.PrivateKey)
	test.Assert(t, ok, "Couldn't load altKey RSA key")

	// Test POST-as-GET request signed with wrong account key
	responseWriter = httptest.NewRecorder()
	_, _, body = signer.byKeyID(2, altKey, "http://localhost/1", "")
	request = makePostRequestWithPath("1", body)
	wfe.Account(ctx, newRequestEvent(), responseWriter, request)
	// It should error
	test.AssertEquals(t, responseWriter.Code, http.StatusForbidden)
	test.AssertUnmarshaledEquals(t, responseWriter.Body.String(), `{
		"type": "urn:ietf:params:acme:error:unauthorized",
		"detail": "Request signing key did not match account key",
		"status": 403
	}`)
}

func TestUpdateAccount(t *testing.T) {
	t.Parallel()
	wfe, _, _ := setupWFE(t)

	for _, tc := range []struct {
		name     string
		req      string
		wantAcct *core.Registration
		wantErr  string
	}{
		{
			name:     "empty status",
			req:      `{}`,
			wantAcct: &core.Registration{Status: core.StatusValid},
		},
		{
			name:     "empty status with contact",
			req:      `{"contact": ["mailto:admin@example.com"]}`,
			wantAcct: &core.Registration{Status: core.StatusValid},
		},
		{
			name:     "valid",
			req:      `{"status": "valid"}`,
			wantAcct: &core.Registration{Status: core.StatusValid},
		},
		{
			name:     "valid with contact",
			req:      `{"status": "valid", "contact": ["mailto:admin@example.com"]}`,
			wantAcct: &core.Registration{Status: core.StatusValid},
		},
		{
			name:     "deactivate",
			req:      `{"status": "deactivated"}`,
			wantAcct: &core.Registration{Status: core.StatusDeactivated},
		},
		{
			name:     "deactivate with contact",
			req:      `{"status": "deactivated", "contact": ["mailto:admin@example.com"]}`,
			wantAcct: &core.Registration{Status: core.StatusDeactivated},
		},
		{
			name:    "unrecognized status",
			req:     `{"status": "foo"}`,
			wantErr: "invalid status",
		},
		{
			// We're happy to ignore fields we don't recognize; they might be useful
			// for other CAs.
			name:     "unrecognized request field",
			req:      `{"foo": "bar"}`,
			wantAcct: &core.Registration{Status: core.StatusValid},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			currAcct := core.Registration{Status: core.StatusValid}

			gotAcct, gotProb := wfe.updateAccount(context.Background(), []byte(tc.req), &currAcct)
			if tc.wantAcct != nil {
				if gotAcct.Status != tc.wantAcct.Status {
					t.Errorf("want status %s, got %s", tc.wantAcct.Status, gotAcct.Status)
				}
				if !reflect.DeepEqual(gotAcct.Contact, tc.wantAcct.Contact) {
					t.Errorf("want contact %v, got %v", tc.wantAcct.Contact, gotAcct.Contact)
				}
			}
			if tc.wantErr != "" {
				if gotProb == nil {
					t.Fatalf("want error %q, got nil", tc.wantErr)
				}
				if !strings.Contains(gotProb.Error(), tc.wantErr) {
					t.Errorf("want error %q, got %q", tc.wantErr, gotProb.Error())
				}
			}
		})
	}
}

type mockSAWithCert struct {
	sapb.StorageAuthorityReadOnlyClient
	cert   *x509.Certificate
	status core.OCSPStatus
}

func newMockSAWithCert(t *testing.T, sa sapb.StorageAuthorityReadOnlyClient) *mockSAWithCert {
	cert, err := core.LoadCert("../test/hierarchy/ee-r3.cert.pem")
	test.AssertNotError(t, err, "Failed to load test cert")
	return &mockSAWithCert{sa, cert, core.OCSPStatusGood}
}

// GetCertificate returns the mock SA's hard-coded certificate, issued by the
// account with regID 1, if the given serial matches. Otherwise, returns not found.
func (sa *mockSAWithCert) GetCertificate(_ context.Context, req *sapb.Serial, _ ...grpc.CallOption) (*corepb.Certificate, error) {
	if req.Serial != core.SerialToString(sa.cert.SerialNumber) {
		return nil, berrors.NotFoundError("Certificate with serial %q not found", req.Serial)
	}

	return &corepb.Certificate{
		RegistrationID: 1,
		Serial:         core.SerialToString(sa.cert.SerialNumber),
		Issued:         timestamppb.New(sa.cert.NotBefore),
		Expires:        timestamppb.New(sa.cert.NotAfter),
		Der:            sa.cert.Raw,
	}, nil
}

// GetCertificateStatus returns the mock SA's status, if the given serial matches.
// Otherwise, returns not found.
func (sa *mockSAWithCert) GetCertificateStatus(_ context.Context, req *sapb.Serial, _ ...grpc.CallOption) (*corepb.CertificateStatus, error) {
	if req.Serial != core.SerialToString(sa.cert.SerialNumber) {
		return nil, berrors.NotFoundError("Status for certificate with serial %q not found", req.Serial)
	}

	return &corepb.CertificateStatus{
		Serial: core.SerialToString(sa.cert.SerialNumber),
		Status: string(sa.status),
	}, nil
}

type mockSAWithIncident struct {
	sapb.StorageAuthorityReadOnlyClient
	incidents map[string]*sapb.Incidents
}

// newMockSAWithIncident returns a mock SA with an enabled (ongoing) incident
// for each of the provided serials.
func newMockSAWithIncident(sa sapb.StorageAuthorityReadOnlyClient, serial []string) *mockSAWithIncident {
	incidents := make(map[string]*sapb.Incidents)
	for _, s := range serial {
		incidents[s] = &sapb.Incidents{
			Incidents: []*sapb.Incident{
				{
					Id:          0,
					SerialTable: "incident_foo",
					Url:         "http://big.bad/incident",
					RenewBy:     nil,
					Enabled:     true,
				},
			},
		}
	}
	return &mockSAWithIncident{sa, incidents}
}

func (sa *mockSAWithIncident) IncidentsForSerial(_ context.Context, req *sapb.Serial, _ ...grpc.CallOption) (*sapb.Incidents, error) {
	incidents, ok := sa.incidents[req.Serial]
	if ok {
		return incidents, nil
	}
	return &sapb.Incidents{}, nil
}

func TestGetCertificate(t *testing.T) {
	wfe, _, signer := setupWFE(t)
	wfe.sa = newMockSAWithCert(t, wfe.sa)
	mux := wfe.Handler(metrics.NoopRegisterer)

	makeGet := func(path string) *http.Request {
		return &http.Request{URL: &url.URL{Path: path}, Method: "GET"}
	}

	makePost := func(keyID int64, key interface{}, path, body string) *http.Request {
		_, _, jwsBody := signer.byKeyID(keyID, key, fmt.Sprintf("http://localhost%s", path), body)
		return makePostRequestWithPath(path, jwsBody)
	}

	altKey := loadKey(t, []byte(test2KeyPrivatePEM))
	_, ok := altKey.(*rsa.PrivateKey)
	test.Assert(t, ok, "Couldn't load RSA key")

	certPemBytes, _ := os.ReadFile("../test/hierarchy/ee-r3.cert.pem")
	cert, err := core.LoadCert("../test/hierarchy/ee-r3.cert.pem")
	test.AssertNotError(t, err, "failed to load test certificate")

	chainPemBytes, err := os.ReadFile("../test/hierarchy/int-r3.cert.pem")
	test.AssertNotError(t, err, "Error reading ../test/hierarchy/int-r3.cert.pem")

	chainCrossPemBytes, err := os.ReadFile("../test/hierarchy/int-r3-cross.cert.pem")
	test.AssertNotError(t, err, "Error reading ../test/hierarchy/int-r3-cross.cert.pem")

	reqPath := fmt.Sprintf("/acme/cert/%s", core.SerialToString(cert.SerialNumber))
	pkixContent := "application/pem-certificate-chain"
	noCache := "public, max-age=0, no-cache"
	notFound := `{"type":"` + probs.ErrorNS + `malformed","detail":"Certificate not found","status":404}`

	testCases := []struct {
		Name            string
		Request         *http.Request
		ExpectedStatus  int
		ExpectedHeaders map[string]string
		ExpectedLink    string
		ExpectedBody    string
		ExpectedCert    []byte
		AnyCert         bool
	}{
		{
			Name:           "Valid serial",
			Request:        makeGet(reqPath),
			ExpectedStatus: http.StatusOK,
			ExpectedHeaders: map[string]string{
				"Content-Type": pkixContent,
			},
			ExpectedCert: append(certPemBytes, append([]byte("\n"), chainPemBytes...)...),
			ExpectedLink: fmt.Sprintf(`<http://localhost%s/1>;rel="alternate"`, reqPath),
		},
		{
			Name:           "Valid serial, POST-as-GET",
			Request:        makePost(1, nil, reqPath, ""),
			ExpectedStatus: http.StatusOK,
			ExpectedHeaders: map[string]string{
				"Content-Type": pkixContent,
			},
			ExpectedCert: append(certPemBytes, append([]byte("\n"), chainPemBytes...)...),
		},
		{
			Name:           "Valid serial, bad POST-as-GET",
			Request:        makePost(1, nil, reqPath, "{}"),
			ExpectedStatus: http.StatusBadRequest,
			ExpectedBody: `{
				"type": "urn:ietf:params:acme:error:malformed",
				"status": 400,
				"detail": "Unable to validate JWS :: POST-as-GET requests must have an empty payload"
			}`,
		},
		{
			Name:           "Valid serial, POST-as-GET from wrong account",
			Request:        makePost(2, altKey, reqPath, ""),
			ExpectedStatus: http.StatusForbidden,
			ExpectedBody: `{
				"type": "urn:ietf:params:acme:error:unauthorized",
				"status": 403,
				"detail": "Account in use did not issue specified certificate"
			}`,
		},
		{
			Name:           "Unused serial, no cache",
			Request:        makeGet("/acme/cert/000000000000000000000000000000000001"),
			ExpectedStatus: http.StatusNotFound,
			ExpectedBody:   notFound,
		},
		{
			Name:           "Invalid serial, no cache",
			Request:        makeGet("/acme/cert/nothex"),
			ExpectedStatus: http.StatusNotFound,
			ExpectedBody:   notFound,
		},
		{
			Name:           "Another invalid serial, no cache",
			Request:        makeGet("/acme/cert/00000000000000"),
			ExpectedStatus: http.StatusNotFound,
			ExpectedBody:   notFound,
		},
		{
			Name:           "Valid serial (explicit default chain)",
			Request:        makeGet(reqPath + "/0"),
			ExpectedStatus: http.StatusOK,
			ExpectedHeaders: map[string]string{
				"Content-Type": pkixContent,
			},
			ExpectedLink: fmt.Sprintf(`<http://localhost%s/1>;rel="alternate"`, reqPath),
			ExpectedCert: append(certPemBytes, append([]byte("\n"), chainPemBytes...)...),
		},
		{
			Name:           "Valid serial (explicit alternate chain)",
			Request:        makeGet(reqPath + "/1"),
			ExpectedStatus: http.StatusOK,
			ExpectedHeaders: map[string]string{
				"Content-Type": pkixContent,
			},
			ExpectedLink: fmt.Sprintf(`<http://localhost%s/0>;rel="alternate"`, reqPath),
			ExpectedCert: append(certPemBytes, append([]byte("\n"), chainCrossPemBytes...)...),
		},
		{
			Name:           "Valid serial (explicit non-existent alternate chain)",
			Request:        makeGet(reqPath + "/2"),
			ExpectedStatus: http.StatusNotFound,
			ExpectedBody:   `{"type":"` + probs.ErrorNS + `malformed","detail":"Unknown issuance chain","status":404}`,
		},
		{
			Name:           "Valid serial (explicit negative alternate chain)",
			Request:        makeGet(reqPath + "/-1"),
			ExpectedStatus: http.StatusBadRequest,
			ExpectedBody:   `{"type":"` + probs.ErrorNS + `malformed","detail":"Chain ID must be a non-negative integer","status":400}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			responseWriter := httptest.NewRecorder()
			mockLog := wfe.log.(*blog.Mock)
			mockLog.Clear()

			// Mux a request for a certificate
			mux.ServeHTTP(responseWriter, tc.Request)
			headers := responseWriter.Header()

			// Assert that the status code written is as expected
			test.AssertEquals(t, responseWriter.Code, tc.ExpectedStatus)

			// All of the responses should have the correct cache control header
			test.AssertEquals(t, headers.Get("Cache-Control"), noCache)

			// If the test cases expects additional headers, check those too
			for h, v := range tc.ExpectedHeaders {
				test.AssertEquals(t, headers.Get(h), v)
			}

			if tc.ExpectedLink != "" {
				found := false
				links := headers["Link"]
				for _, link := range links {
					if link == tc.ExpectedLink {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected link '%s', but did not find it in (%v)",
						tc.ExpectedLink, links)
				}
			}

			if tc.AnyCert { // Certificate is randomly generated, don't match it
				return
			}

			if len(tc.ExpectedCert) > 0 {
				// If the expectation was to return a certificate, check that it was the one expected
				bodyBytes := responseWriter.Body.Bytes()
				test.Assert(t, bytes.Equal(bodyBytes, tc.ExpectedCert), "Certificates don't match")

				// Successful requests should be logged as such
				reqlogs := mockLog.GetAllMatching(`INFO: [^ ]+ [^ ]+ [^ ]+ 200 .*`)
				if len(reqlogs) != 1 {
					t.Errorf("Didn't find info logs with code 200. Instead got:\n%s\n",
						strings.Join(mockLog.GetAllMatching(`.*`), "\n"))
				}
			} else {
				// Otherwise if the expectation wasn't a certificate, check that the body matches the expected
				body := responseWriter.Body.String()
				test.AssertUnmarshaledEquals(t, body, tc.ExpectedBody)

				// Unsuccessful requests should be logged as such
				reqlogs := mockLog.GetAllMatching(fmt.Sprintf(`INFO: [^ ]+ [^ ]+ [^ ]+ %d .*`, tc.ExpectedStatus))
				if len(reqlogs) != 1 {
					t.Errorf("Didn't find info logs with code %d. Instead got:\n%s\n",
						tc.ExpectedStatus, strings.Join(mockLog.GetAllMatching(`.*`), "\n"))
				}
			}
		})
	}
}

type mockSAWithNewCert struct {
	sapb.StorageAuthorityReadOnlyClient
	clk clock.Clock
}

func (sa *mockSAWithNewCert) GetCertificate(_ context.Context, req *sapb.Serial, _ ...grpc.CallOption) (*corepb.Certificate, error) {
	issuer, err := core.LoadCert("../test/hierarchy/int-e1.cert.pem")
	if err != nil {
		return nil, fmt.Errorf("failed to load test issuer cert: %w", err)
	}

	issuerKeyPem, err := os.ReadFile("../test/hierarchy/int-e1.key.pem")
	if err != nil {
		return nil, fmt.Errorf("failed to load test issuer key: %w", err)
	}
	issuerKey := loadKey(&testing.T{}, issuerKeyPem)

	newKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to create test key: %w", err)
	}

	sn, err := core.StringToSerial(req.Serial)
	if err != nil {
		return nil, fmt.Errorf("failed to parse test serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: sn,
		DNSNames:     []string{"new.ee.boulder.test"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, issuer, &newKey.PublicKey, issuerKey)
	if err != nil {
		return nil, fmt.Errorf("failed to issue test cert: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("failed to parse test cert: %w", err)
	}

	return &corepb.Certificate{
		RegistrationID: 1,
		Serial:         core.SerialToString(cert.SerialNumber),
		Issued:         timestamppb.New(sa.clk.Now().Add(-1 * time.Second)),
		Der:            cert.Raw,
	}, nil
}

// TestGetCertificateNew tests for the case when the certificate is new (by
// dynamically generating it at test time), and therefore isn't served by the
// GET api.
func TestGetCertificateNew(t *testing.T) {
	wfe, fc, signer := setupWFE(t)
	wfe.sa = &mockSAWithNewCert{wfe.sa, fc}
	mux := wfe.Handler(metrics.NoopRegisterer)

	makeGet := func(path string) *http.Request {
		return &http.Request{URL: &url.URL{Path: path}, Method: "GET"}
	}

	makePost := func(keyID int64, key interface{}, path, body string) *http.Request {
		_, _, jwsBody := signer.byKeyID(keyID, key, fmt.Sprintf("http://localhost%s", path), body)
		return makePostRequestWithPath(path, jwsBody)
	}

	altKey := loadKey(t, []byte(test2KeyPrivatePEM))
	_, ok := altKey.(*rsa.PrivateKey)
	test.Assert(t, ok, "Couldn't load RSA key")

	pkixContent := "application/pem-certificate-chain"
	noCache := "public, max-age=0, no-cache"

	testCases := []struct {
		Name            string
		Request         *http.Request
		ExpectedStatus  int
		ExpectedHeaders map[string]string
		ExpectedBody    string
	}{
		{
			Name:           "Get",
			Request:        makeGet("/get/cert/000000000000000000000000000000000001"),
			ExpectedStatus: http.StatusForbidden,
			ExpectedBody: `{
				"type": "` + probs.ErrorNS + `unauthorized",
				"detail": "Certificate is too new for GET API. You should only use this non-standard API to access resources created more than 10s ago",
				"status": 403
			}`,
		},
		{
			Name:           "ACME Get",
			Request:        makeGet("/acme/cert/000000000000000000000000000000000002"),
			ExpectedStatus: http.StatusOK,
			ExpectedHeaders: map[string]string{
				"Content-Type": pkixContent,
			},
		},
		{
			Name:           "ACME POST-as-GET",
			Request:        makePost(1, nil, "/acme/cert/000000000000000000000000000000000003", ""),
			ExpectedStatus: http.StatusOK,
			ExpectedHeaders: map[string]string{
				"Content-Type": pkixContent,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			responseWriter := httptest.NewRecorder()
			mockLog := wfe.log.(*blog.Mock)
			mockLog.Clear()

			// Mux a request for a certificate
			mux.ServeHTTP(responseWriter, tc.Request)
			headers := responseWriter.Header()

			// Assert that the status code written is as expected
			test.AssertEquals(t, responseWriter.Code, tc.ExpectedStatus)

			// All of the responses should have the correct cache control header
			test.AssertEquals(t, headers.Get("Cache-Control"), noCache)

			// If the test cases expects additional headers, check those too
			for h, v := range tc.ExpectedHeaders {
				test.AssertEquals(t, headers.Get(h), v)
			}

			// If we're expecting a particular body (because of an error), check that.
			if tc.ExpectedBody != "" {
				body := responseWriter.Body.String()
				test.AssertUnmarshaledEquals(t, body, tc.ExpectedBody)

				// Unsuccessful requests should be logged as such
				reqlogs := mockLog.GetAllMatching(fmt.Sprintf(`INFO: [^ ]+ [^ ]+ [^ ]+ %d .*`, tc.ExpectedStatus))
				if len(reqlogs) != 1 {
					t.Errorf("Didn't find info logs with code %d. Instead got:\n%s\n",
						tc.ExpectedStatus, strings.Join(mockLog.GetAllMatching(`.*`), "\n"))
				}
			}
		})
	}
}

// This uses httptest.NewServer because ServeMux.ServeHTTP won't prevent the
// body from being sent like the net/http Server's actually do.
func TestGetCertificateHEADHasCorrectBodyLength(t *testing.T) {
	wfe, _, _ := setupWFE(t)
	wfe.sa = newMockSAWithCert(t, wfe.sa)

	certPemBytes, _ := os.ReadFile("../test/hierarchy/ee-r3.cert.pem")
	cert, err := core.LoadCert("../test/hierarchy/ee-r3.cert.pem")
	test.AssertNotError(t, err, "failed to load test certificate")

	chainPemBytes, err := os.ReadFile("../test/hierarchy/int-r3.cert.pem")
	test.AssertNotError(t, err, "Error reading ../test/hierarchy/int-r3.cert.pem")
	chain := fmt.Sprintf("%s\n%s", string(certPemBytes), string(chainPemBytes))
	chainLen := strconv.Itoa(len(chain))

	mockLog := wfe.log.(*blog.Mock)
	mockLog.Clear()

	mux := wfe.Handler(metrics.NoopRegisterer)
	s := httptest.NewServer(mux)
	defer s.Close()
	req, _ := http.NewRequest(
		"HEAD", fmt.Sprintf("%s/acme/cert/%s", s.URL, core.SerialToString(cert.SerialNumber)), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		test.AssertNotError(t, err, "do error")
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		test.AssertNotEquals(t, err, "readall error")
	}
	err = resp.Body.Close()
	if err != nil {
		test.AssertNotEquals(t, err, "readall error")
	}
	test.AssertEquals(t, resp.StatusCode, 200)
	test.AssertEquals(t, chainLen, resp.Header.Get("Content-Length"))
	test.AssertEquals(t, 0, len(body))
}

type mockSAWithError struct {
	sapb.StorageAuthorityReadOnlyClient
}

func (sa *mockSAWithError) GetCertificate(_ context.Context, req *sapb.Serial, _ ...grpc.CallOption) (*corepb.Certificate, error) {
	return nil, errors.New("Oops")
}

func TestGetCertificateServerError(t *testing.T) {
	// TODO: add tests for failure to parse the retrieved cert, a cert whose
	// IssuerNameID is unknown, and a cert whose signature can't be verified.
	wfe, _, _ := setupWFE(t)
	wfe.sa = &mockSAWithError{wfe.sa}
	mux := wfe.Handler(metrics.NoopRegisterer)

	cert, err := core.LoadCert("../test/hierarchy/ee-r3.cert.pem")
	test.AssertNotError(t, err, "failed to load test certificate")

	reqPath := fmt.Sprintf("/acme/cert/%s", core.SerialToString(cert.SerialNumber))
	req := &http.Request{URL: &url.URL{Path: reqPath}, Method: "GET"}

	// Mux a request for a certificate
	responseWriter := httptest.NewRecorder()
	mux.ServeHTTP(responseWriter, req)

	test.AssertEquals(t, responseWriter.Code, http.StatusInternalServerError)

	noCache := "public, max-age=0, no-cache"
	test.AssertEquals(t, responseWriter.Header().Get("Cache-Control"), noCache)

	body := `{
		"type": "urn:ietf:params:acme:error:serverInternal",
		"status": 500,
		"detail": "Failed to retrieve certificate"
	}`
	test.AssertUnmarshaledEquals(t, responseWriter.Body.String(), body)
}

func newRequestEvent() *web.RequestEvent {
	return &web.RequestEvent{Extra: make(map[string]interface{})}
}

func TestHeaderBoulderRequester(t *testing.T) {
	wfe, _, signer := setupWFE(t)
	mux := wfe.Handler(metrics.NoopRegisterer)
	responseWriter := httptest.NewRecorder()

	key := loadKey(t, []byte(test1KeyPrivatePEM))
	_, ok := key.(*rsa.PrivateKey)
	test.Assert(t, ok, "Failed to load test 1 RSA key")

	payload := `{}`
	path := fmt.Sprintf("%s%d", acctPath, 1)
	signedURL := fmt.Sprintf("http://localhost%s", path)
	_, _, body := signer.byKeyID(1, nil, signedURL, payload)
	request := makePostRequestWithPath(path, body)

	mux.ServeHTTP(responseWriter, request)
	test.AssertEquals(t, responseWriter.Header().Get("Boulder-Requester"), "1")

	// requests that do call sendError() also should have the requester header
	payload = `{"agreement":"https://letsencrypt.org/im-bad"}`
	_, _, body = signer.byKeyID(1, nil, signedURL, payload)
	request = makePostRequestWithPath(path, body)
	mux.ServeHTTP(responseWriter, request)
	test.AssertEquals(t, responseWriter.Header().Get("Boulder-Requester"), "1")
}

func TestDeactivateAuthorizationHandler(t *testing.T) {
	wfe, _, signer := setupWFE(t)
	responseWriter := httptest.NewRecorder()

	responseWriter.Body.Reset()

	payload := `{"status":""}`
	_, _, body := signer.byKeyID(1, nil, "http://localhost/1/1", payload)
	request := makePostRequestWithPath("1/1", body)

	wfe.AuthorizationHandler(ctx, newRequestEvent(), responseWriter, request)
	test.AssertUnmarshaledEquals(t,
		responseWriter.Body.String(),
		`{"type": "`+probs.ErrorNS+`malformed","detail": "Invalid status value","status": 400}`)

	responseWriter.Body.Reset()
	payload = `{"status":"deactivated"}`
	_, _, body = signer.byKeyID(1, nil, "http://localhost/1/1", payload)
	request = makePostRequestWithPath("1/1", body)

	wfe.AuthorizationHandler(ctx, newRequestEvent(), responseWriter, request)
	test.AssertUnmarshaledEquals(t,
		responseWriter.Body.String(),
		`{
		  "identifier": {
		    "type": "dns",
		    "value": "not-an-example.com"
		  },
		  "status": "deactivated",
		  "expires": "2070-01-01T00:00:00Z",
		  "challenges": [
		    {
					"status": "valid",
					"type": "http-01",
					"token": "token",
					"url": "http://localhost/acme/chall/1/1/7TyhFQ"
		    }
		  ]
		}`)
}

func TestDeactivateAccount(t *testing.T) {
	responseWriter := httptest.NewRecorder()
	wfe, _, signer := setupWFE(t)

	responseWriter.Body.Reset()
	payload := `{"status":"asd"}`
	signedURL := "http://localhost/1"
	path := "1"
	_, _, body := signer.byKeyID(1, nil, signedURL, payload)
	request := makePostRequestWithPath(path, body)

	wfe.Account(ctx, newRequestEvent(), responseWriter, request)
	test.AssertUnmarshaledEquals(t,
		responseWriter.Body.String(),
		`{"type": "`+probs.ErrorNS+`malformed","detail": "Unable to update account :: invalid status \"asd\" for account update request, must be \"valid\" or \"deactivated\"","status": 400}`)

	responseWriter.Body.Reset()
	payload = `{"status":"deactivated"}`
	_, _, body = signer.byKeyID(1, nil, signedURL, payload)
	request = makePostRequestWithPath(path, body)

	wfe.Account(ctx, newRequestEvent(), responseWriter, request)
	test.AssertUnmarshaledEquals(t,
		responseWriter.Body.String(),
		`{
		  "key": {
		    "kty": "RSA",
		    "n": "yNWVhtYEKJR21y9xsHV-PD_bYwbXSeNuFal46xYxVfRL5mqha7vttvjB_vc7Xg2RvgCxHPCqoxgMPTzHrZT75LjCwIW2K_klBYN8oYvTwwmeSkAz6ut7ZxPv-nZaT5TJhGk0NT2kh_zSpdriEJ_3vW-mqxYbbBmpvHqsa1_zx9fSuHYctAZJWzxzUZXykbWMWQZpEiE0J4ajj51fInEzVn7VxV-mzfMyboQjujPh7aNJxAWSq4oQEJJDgWwSh9leyoJoPpONHxh5nEE5AjE01FkGICSxjpZsF-w8hOTI3XXohUdu29Se26k2B0PolDSuj0GIQU6-W9TdLXSjBb2SpQ",
		    "e": "AQAB"
		  },
		  "status": "deactivated"
		}`)

	responseWriter.Body.Reset()
	payload = `{"status":"deactivated", "contact":[]}`
	_, _, body = signer.byKeyID(1, nil, signedURL, payload)
	request = makePostRequestWithPath(path, body)
	wfe.Account(ctx, newRequestEvent(), responseWriter, request)
	test.AssertUnmarshaledEquals(t,
		responseWriter.Body.String(),
		`{
		  "key": {
		    "kty": "RSA",
		    "n": "yNWVhtYEKJR21y9xsHV-PD_bYwbXSeNuFal46xYxVfRL5mqha7vttvjB_vc7Xg2RvgCxHPCqoxgMPTzHrZT75LjCwIW2K_klBYN8oYvTwwmeSkAz6ut7ZxPv-nZaT5TJhGk0NT2kh_zSpdriEJ_3vW-mqxYbbBmpvHqsa1_zx9fSuHYctAZJWzxzUZXykbWMWQZpEiE0J4ajj51fInEzVn7VxV-mzfMyboQjujPh7aNJxAWSq4oQEJJDgWwSh9leyoJoPpONHxh5nEE5AjE01FkGICSxjpZsF-w8hOTI3XXohUdu29Se26k2B0PolDSuj0GIQU6-W9TdLXSjBb2SpQ",
		    "e": "AQAB"
		  },
		  "status": "deactivated"
		}`)

	responseWriter.Body.Reset()
	key := loadKey(t, []byte(test3KeyPrivatePEM))
	_, ok := key.(*rsa.PrivateKey)
	test.Assert(t, ok, "Couldn't load test3 RSA key")

	payload = `{"status":"deactivated"}`
	path = "3"
	signedURL = "http://localhost/3"
	_, _, body = signer.byKeyID(3, key, signedURL, payload)
	request = makePostRequestWithPath(path, body)

	wfe.Account(ctx, newRequestEvent(), responseWriter, request)

	test.AssertUnmarshaledEquals(t,
		responseWriter.Body.String(),
		`{
		  "type": "`+probs.ErrorNS+`unauthorized",
		  "detail": "Unable to validate JWS :: Account is not valid, has status \"deactivated\"",
		  "status": 403
		}`)
}

func TestNewOrder(t *testing.T) {
	wfe, _, signer := setupWFE(t)
	responseWriter := httptest.NewRecorder()

	targetHost := "localhost"
	targetPath := "new-order"
	signedURL := fmt.Sprintf("http://%s/%s", targetHost, targetPath)

	invalidIdentifierBody := `
	{
		"Identifiers": [
			{"type": "dns",    "value": "not-example.com"},
			{"type": "dns",    "value": "www.not-example.com"},
			{"type": "fakeID", "value": "www.i-am-21.com"}
		]
	}
	`

	validOrderBody := `
	{
		"Identifiers": [
			{"type": "dns", "value": "not-example.com"},
			{"type": "dns", "value": "www.not-example.com"},
			{"type": "ip", "value": "9.9.9.9"}
		]
	}`

	validOrderBodyWithMixedCaseIdentifiers := `
	{
		"Identifiers": [
			{"type": "dns", "value": "Not-Example.com"},
			{"type": "dns", "value": "WWW.Not-example.com"},
			{"type": "ip", "value": "9.9.9.9"}
		]
	}`

	// Body with a SAN that is longer than 64 bytes. This one is 65 bytes.
	tooLongCNBody := `
	{
		"Identifiers": [
			{
				"type": "dns",
				"value": "thisreallylongexampledomainisabytelongerthanthemaxcnbytelimit.com"
			}
		]
	}`

	oneLongOneShortCNBody := `
	{
		"Identifiers": [
			{
				"type": "dns",
				"value": "thisreallylongexampledomainisabytelongerthanthemaxcnbytelimit.com"
			},
			{
				"type": "dns",
				"value": "not-example.com"
			}
		]
	}`

	testCases := []struct {
		Name            string
		Request         *http.Request
		ExpectedBody    string
		ExpectedHeaders map[string]string
	}{
		{
			Name: "POST, but no body",
			Request: &http.Request{
				Method: "POST",
				Header: map[string][]string{
					"Content-Length": {"0"},
					"Content-Type":   {expectedJWSContentType},
				},
			},
			ExpectedBody: `{"type":"` + probs.ErrorNS + `malformed","detail":"Unable to validate JWS :: No body on POST","status":400}`,
		},
		{
			Name:         "POST, with an invalid JWS body",
			Request:      makePostRequestWithPath("hi", "hi"),
			ExpectedBody: `{"type":"` + probs.ErrorNS + `malformed","detail":"Unable to validate JWS :: Parse error reading JWS","status":400}`,
		},
		{
			Name:         "POST, properly signed JWS, payload isn't valid",
			Request:      signAndPost(signer, targetPath, signedURL, "foo"),
			ExpectedBody: `{"type":"` + probs.ErrorNS + `malformed","detail":"Unable to validate JWS :: Request payload did not parse as JSON","status":400}`,
		},
		{
			Name:         "POST, empty DNS identifier",
			Request:      signAndPost(signer, targetPath, signedURL, `{"identifiers":[{"type":"dns","value":""}]}`),
			ExpectedBody: `{"type":"` + probs.ErrorNS + `malformed","detail":"NewOrder request included empty identifier","status":400}`,
		},
		{
			Name:         "POST, empty IP identifier",
			Request:      signAndPost(signer, targetPath, signedURL, `{"identifiers":[{"type":"ip","value":""}]}`),
			ExpectedBody: `{"type":"` + probs.ErrorNS + `malformed","detail":"NewOrder request included empty identifier","status":400}`,
		},
		{
			Name:         "POST, invalid DNS identifier",
			Request:      signAndPost(signer, targetPath, signedURL, `{"identifiers":[{"type":"dns","value":"example.invalid"}]}`),
			ExpectedBody: `{"type":"` + probs.ErrorNS + `rejectedIdentifier","detail":"Invalid identifiers requested :: Cannot issue for \"example.invalid\": Domain name does not end with a valid public suffix (TLD)","status":400}`,
		},
		{
			Name:         "POST, invalid IP identifier",
			Request:      signAndPost(signer, targetPath, signedURL, `{"identifiers":[{"type":"ip","value":"127.0.0.0.0.0.0.1"}]}`),
			ExpectedBody: `{"type":"` + probs.ErrorNS + `rejectedIdentifier","detail":"Invalid identifiers requested :: Cannot issue for \"127.0.0.0.0.0.0.1\": IP address is invalid","status":400}`,
		},
		{
			Name:         "POST, no identifiers in payload",
			Request:      signAndPost(signer, targetPath, signedURL, "{}"),
			ExpectedBody: `{"type":"` + probs.ErrorNS + `malformed","detail":"NewOrder request did not specify any identifiers","status":400}`,
		},
		{
			Name:         "POST, invalid identifier type in payload",
			Request:      signAndPost(signer, targetPath, signedURL, invalidIdentifierBody),
			ExpectedBody: `{"type":"` + probs.ErrorNS + `unsupportedIdentifier","detail":"NewOrder request included unsupported identifier: type \"fakeID\", value \"www.i-am-21.com\"","status":400}`,
		},
		{
			Name:         "POST, notAfter and notBefore in payload",
			Request:      signAndPost(signer, targetPath, signedURL, `{"identifiers":[{"type": "dns", "value": "not-example.com"}], "notBefore":"now", "notAfter": "later"}`),
			ExpectedBody: `{"type":"` + probs.ErrorNS + `malformed","detail":"NotBefore and NotAfter are not supported","status":400}`,
		},
		{
			Name:    "POST, good payload, all names too long to fit in CN",
			Request: signAndPost(signer, targetPath, signedURL, tooLongCNBody),
			ExpectedBody: `
			{
				"status": "pending",
				"expires": "2021-02-01T01:01:01Z",
				"identifiers": [
					{ "type": "dns", "value": "thisreallylongexampledomainisabytelongerthanthemaxcnbytelimit.com"}
				],
				"authorizations": [
					"http://localhost/acme/authz/1/1"
				],
				"finalize": "http://localhost/acme/finalize/1/1"
			}`,
		},
		{
			Name:    "POST, good payload, one potential CNs less than 64 bytes and one longer",
			Request: signAndPost(signer, targetPath, signedURL, oneLongOneShortCNBody),
			ExpectedBody: `
			{
				"status": "pending",
				"expires": "2021-02-01T01:01:01Z",
				"identifiers": [
					{ "type": "dns", "value": "not-example.com"},
					{ "type": "dns", "value": "thisreallylongexampledomainisabytelongerthanthemaxcnbytelimit.com"}
				],
				"authorizations": [
					"http://localhost/acme/authz/1/1"
				],
				"finalize": "http://localhost/acme/finalize/1/1"
			}`,
		},
		{
			Name:    "POST, good payload",
			Request: signAndPost(signer, targetPath, signedURL, validOrderBody),
			ExpectedBody: `
					{
						"status": "pending",
						"expires": "2021-02-01T01:01:01Z",
						"identifiers": [
							{ "type": "dns", "value": "not-example.com"},
							{ "type": "dns", "value": "www.not-example.com"},
							{ "type": "ip", "value": "9.9.9.9"}
						],
						"authorizations": [
							"http://localhost/acme/authz/1/1"
						],
						"finalize": "http://localhost/acme/finalize/1/1"
					}`,
		},
		{
			Name:    "POST, good payload, but when the input had mixed case",
			Request: signAndPost(signer, targetPath, signedURL, validOrderBodyWithMixedCaseIdentifiers),
			ExpectedBody: `
					{
						"status": "pending",
						"expires": "2021-02-01T01:01:01Z",
						"identifiers": [
							{ "type": "dns", "value": "not-example.com"},
							{ "type": "dns", "value": "www.not-example.com"},
							{ "type": "ip", "value": "9.9.9.9"}
						],
						"authorizations": [
							"http://localhost/acme/authz/1/1"
						],
						"finalize": "http://localhost/acme/finalize/1/1"
					}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			responseWriter.Body.Reset()

			wfe.NewOrder(ctx, newRequestEvent(), responseWriter, tc.Request)
			test.AssertUnmarshaledEquals(t, responseWriter.Body.String(), tc.ExpectedBody)

			headers := responseWriter.Header()
			for k, v := range tc.ExpectedHeaders {
				test.AssertEquals(t, headers.Get(k), v)
			}
		})
	}

	// Test that we log the "Created" field.
	responseWriter.Body.Reset()
	request := signAndPost(signer, targetPath, signedURL, validOrderBody)
	requestEvent := newRequestEvent()
	wfe.NewOrder(ctx, requestEvent, responseWriter, request)

	if requestEvent.Created != "1" {
		t.Errorf("Expected to log Created field when creating Order: %#v", requestEvent)
	}
}

func TestFinalizeOrder(t *testing.T) {
	wfe, _, signer := setupWFE(t)
	responseWriter := httptest.NewRecorder()

	targetHost := "localhost"
	targetPath := "1/1"
	signedURL := fmt.Sprintf("http://%s/%s", targetHost, targetPath)

	// This example is a well-formed CSR for the name "example.com".
	goodCertCSRPayload := `{
		"csr": "MIHRMHgCAQAwFjEUMBIGA1UEAxMLZXhhbXBsZS5jb20wWTATBgcqhkjOPQIBBggqhkjOPQMBBwNCAAQ2hlvArQl5k0L1eF1vF5dwr7ASm2iKqibmauund-z3QJpuudnNEjlyOXi-IY1rxyhehRrtbm_bbcNCtZLgbkPvoAAwCgYIKoZIzj0EAwIDSQAwRgIhAJ8z2EDll2BvoNRotAknEfrqeP6K5CN1NeVMB4QOu0G1AiEAqAVpiGwNyV7SEZ67vV5vyuGsKPAGnqrisZh5Vg5JKHE="
	}`

	egUrl := mustParseURL("1/1")

	testCases := []struct {
		Name            string
		Request         *http.Request
		ExpectedHeaders map[string]string
		ExpectedBody    string
	}{
		{
			Name: "POST, but no body",
			Request: &http.Request{
				URL:        egUrl,
				RequestURI: targetPath,
				Method:     "POST",
				Header: map[string][]string{
					"Content-Length": {"0"},
					"Content-Type":   {expectedJWSContentType},
				},
			},
			ExpectedBody: `{"type":"` + probs.ErrorNS + `malformed","detail":"Unable to validate JWS :: No body on POST","status":400}`,
		},
		{
			Name:         "POST, with an invalid JWS body",
			Request:      makePostRequestWithPath(targetPath, "hi"),
			ExpectedBody: `{"type":"` + probs.ErrorNS + `malformed","detail":"Unable to validate JWS :: Parse error reading JWS","status":400}`,
		},
		{
			Name:         "POST, properly signed JWS, payload isn't valid",
			Request:      signAndPost(signer, targetPath, signedURL, "foo"),
			ExpectedBody: `{"type":"` + probs.ErrorNS + `malformed","detail":"Unable to validate JWS :: Request payload did not parse as JSON","status":400}`,
		},
		{
			Name:         "Invalid path",
			Request:      signAndPost(signer, "1", "http://localhost/1", "{}"),
			ExpectedBody: `{"type":"` + probs.ErrorNS + `malformed","detail":"Invalid request path","status":404}`,
		},
		{
			Name:         "Bad acct ID in path",
			Request:      signAndPost(signer, "a/1", "http://localhost/a/1", "{}"),
			ExpectedBody: `{"type":"` + probs.ErrorNS + `malformed","detail":"Invalid account ID","status":400}`,
		},
		{
			Name: "Mismatched acct ID in path/JWS",
			// Note(@cpu): We use "http://localhost/2/1" here not
			// "http://localhost/order/2/1" because we are calling the Order
			// handler directly and it normally has the initial path component
			// stripped by the global WFE2 handler. We need the JWS URL to match the request
			// URL so we fudge both such that the finalize-order prefix has been removed.
			Request:      signAndPost(signer, "2/1", "http://localhost/2/1", "{}"),
			ExpectedBody: `{"type":"` + probs.ErrorNS + `malformed","detail":"Mismatched account ID","status":400}`,
		},
		{
			Name:         "Order ID is invalid",
			Request:      signAndPost(signer, "1/okwhatever/finalize-order", "http://localhost/1/okwhatever/finalize-order", "{}"),
			ExpectedBody: `{"type":"` + probs.ErrorNS + `malformed","detail":"Invalid order ID","status":400}`,
		},
		{
			Name: "Order doesn't exist",
			// mocks/mocks.go's StorageAuthority's GetOrder mock treats ID 2 as missing
			Request:      signAndPost(signer, "1/2", "http://localhost/1/2", "{}"),
			ExpectedBody: `{"type":"` + probs.ErrorNS + `malformed","detail":"No order for ID 2","status":404}`,
		},
		{
			Name: "Order is already finalized",
			// mocks/mocks.go's StorageAuthority's GetOrder mock treats ID 1 as an Order with a Serial
			Request:      signAndPost(signer, "1/1", "http://localhost/1/1", goodCertCSRPayload),
			ExpectedBody: `{"type":"` + probs.ErrorNS + `orderNotReady","detail":"Order's status (\"valid\") is not acceptable for finalization","status":403}`,
		},
		{
			Name: "Order is expired",
			// mocks/mocks.go's StorageAuthority's GetOrder mock treats ID 7 as an Order that has already expired
			Request:      signAndPost(signer, "1/7", "http://localhost/1/7", goodCertCSRPayload),
			ExpectedBody: `{"type":"` + probs.ErrorNS + `malformed","detail":"Order 7 is expired","status":404}`,
		},
		{
			Name:         "Good CSR, Pending Order",
			Request:      signAndPost(signer, "1/4", "http://localhost/1/4", goodCertCSRPayload),
			ExpectedBody: `{"type":"` + probs.ErrorNS + `orderNotReady","detail":"Order's status (\"pending\") is not acceptable for finalization","status":403}`,
		},
		{
			Name:    "Good CSR, Ready Order",
			Request: signAndPost(signer, "1/8", "http://localhost/1/8", goodCertCSRPayload),
			ExpectedHeaders: map[string]string{
				"Location":    "http://localhost/acme/order/1/8",
				"Retry-After": "3",
			},
			ExpectedBody: `
{
  "status": "processing",
  "expires": "2000-01-01T00:00:00Z",
  "identifiers": [
    {"type":"dns","value":"example.com"}
  ],
	"profile": "default",
  "authorizations": [
    "http://localhost/acme/authz/1/1"
  ],
  "finalize": "http://localhost/acme/finalize/1/8"
}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			responseWriter.Body.Reset()
			wfe.FinalizeOrder(ctx, newRequestEvent(), responseWriter, tc.Request)
			for k, v := range tc.ExpectedHeaders {
				got := responseWriter.Header().Get(k)
				if v != got {
					t.Errorf("Header %q: Expected %q, got %q", k, v, got)
				}
			}
			test.AssertUnmarshaledEquals(t, responseWriter.Body.String(), tc.ExpectedBody)
		})
	}

	// Check a bad CSR request separately from the above testcases. We don't want
	// to match the whole response body because the "detail" of a bad CSR problem
	// contains a verbose Go error message that can change between versions (e.g.
	// Go 1.10.4 to 1.11 changed the expected format)
	badCSRReq := signAndPost(signer, "1/8", "http://localhost/1/8", `{"CSR": "ABCD"}`)
	responseWriter.Body.Reset()
	wfe.FinalizeOrder(ctx, newRequestEvent(), responseWriter, badCSRReq)
	responseBody := responseWriter.Body.String()
	test.AssertContains(t, responseBody, "Error parsing certificate request")
}

func TestKeyRollover(t *testing.T) {
	responseWriter := httptest.NewRecorder()
	wfe, _, signer := setupWFE(t)

	existingKey, err := rsa.GenerateKey(rand.Reader, 2048)
	test.AssertNotError(t, err, "Error creating random 2048 RSA key")

	newKeyBytes, err := os.ReadFile("../test/test-key-5.der")
	test.AssertNotError(t, err, "Failed to read ../test/test-key-5.der")
	newKeyPriv, err := x509.ParsePKCS1PrivateKey(newKeyBytes)
	test.AssertNotError(t, err, "Failed parsing private key")
	newJWKJSON, err := jose.JSONWebKey{Key: newKeyPriv.Public()}.MarshalJSON()
	test.AssertNotError(t, err, "Failed to marshal JWK JSON")

	wfe.KeyRollover(ctx, newRequestEvent(), responseWriter, makePostRequestWithPath("", "{}"))
	test.AssertUnmarshaledEquals(t,
		responseWriter.Body.String(),
		`{
		  "type": "`+probs.ErrorNS+`malformed",
		  "detail": "Unable to validate JWS :: Parse error reading JWS",
		  "status": 400
		}`)

	testCases := []struct {
		Name             string
		Payload          string
		ExpectedResponse string
		NewKey           crypto.Signer
		ErrorStatType    string
	}{
		{
			Name:    "Missing account URL",
			Payload: `{"oldKey":` + test1KeyPublicJSON + `}`,
			ExpectedResponse: `{
		     "type": "` + probs.ErrorNS + `malformed",
		     "detail": "Inner key rollover request specified Account \"\", but outer JWS has Key ID \"http://localhost/acme/acct/1\"",
		     "status": 400
		   }`,
			NewKey:        newKeyPriv,
			ErrorStatType: "KeyRolloverMismatchedAccount",
		},
		{
			Name:    "incorrect old key",
			Payload: `{"oldKey":` + string(newJWKJSON) + `,"account":"http://localhost/acme/acct/1"}`,
			ExpectedResponse: `{
		     "type": "` + probs.ErrorNS + `malformed",
		     "detail": "Unable to validate JWS :: Inner JWS does not contain old key field matching current account key",
		     "status": 400
		   }`,
			NewKey:        newKeyPriv,
			ErrorStatType: "KeyRolloverWrongOldKey",
		},
		{
			Name:    "Valid key rollover request, key exists",
			Payload: `{"oldKey":` + test1KeyPublicJSON + `,"account":"http://localhost/acme/acct/1"}`,
			ExpectedResponse: `{
                          "type": "urn:ietf:params:acme:error:conflict",
                          "detail": "New key is already in use for a different account",
                          "status": 409
                        }`,
			NewKey: existingKey,
		},
		{
			Name:    "Valid key rollover request",
			Payload: `{"oldKey":` + test1KeyPublicJSON + `,"account":"http://localhost/acme/acct/1"}`,
			ExpectedResponse: `{
		     "key": ` + string(newJWKJSON) + `,
		     "status": "valid"
		   }`,
			NewKey: newKeyPriv,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			wfe.stats.joseErrorCount.Reset()
			responseWriter.Body.Reset()
			_, _, inner := signer.embeddedJWK(tc.NewKey, "http://localhost/key-change", tc.Payload)
			_, _, outer := signer.byKeyID(1, nil, "http://localhost/key-change", inner)
			wfe.KeyRollover(ctx, newRequestEvent(), responseWriter, makePostRequestWithPath("key-change", outer))
			t.Log(responseWriter.Body.String())
			t.Log(tc.ExpectedResponse)
			test.AssertUnmarshaledEquals(t, responseWriter.Body.String(), tc.ExpectedResponse)
			if tc.ErrorStatType != "" {
				test.AssertMetricWithLabelsEquals(
					t, wfe.stats.joseErrorCount, prometheus.Labels{"type": tc.ErrorStatType}, 1)
			}
		})
	}
}

func TestKeyRolloverMismatchedJWSURLs(t *testing.T) {
	responseWriter := httptest.NewRecorder()
	wfe, _, signer := setupWFE(t)

	newKeyBytes, err := os.ReadFile("../test/test-key-5.der")
	test.AssertNotError(t, err, "Failed to read ../test/test-key-5.der")
	newKeyPriv, err := x509.ParsePKCS1PrivateKey(newKeyBytes)
	test.AssertNotError(t, err, "Failed parsing private key")

	_, _, inner := signer.embeddedJWK(newKeyPriv, "http://localhost/wrong-url", "{}")
	_, _, outer := signer.byKeyID(1, nil, "http://localhost/key-change", inner)
	wfe.KeyRollover(ctx, newRequestEvent(), responseWriter, makePostRequestWithPath("key-change", outer))
	test.AssertUnmarshaledEquals(t, responseWriter.Body.String(), `
		{
			"type": "urn:ietf:params:acme:error:malformed",
			"detail": "Unable to validate JWS :: Outer JWS 'url' value \"http://localhost/key-change\" does not match inner JWS 'url' value \"http://localhost/wrong-url\"",
			"status": 400
		}`)
}

func TestGetOrder(t *testing.T) {
	wfe, _, signer := setupWFE(t)

	makeGet := func(path string) *http.Request {
		return &http.Request{URL: &url.URL{Path: path}, Method: "GET"}
	}

	makePost := func(keyID int64, path, body string) *http.Request {
		_, _, jwsBody := signer.byKeyID(keyID, nil, fmt.Sprintf("http://localhost/%s", path), body)
		return makePostRequestWithPath(path, jwsBody)
	}

	testCases := []struct {
		Name     string
		Request  *http.Request
		Response string
		Headers  map[string]string
	}{
		{
			Name:     "Good request",
			Request:  makeGet("1/1"),
			Response: `{"status": "valid","expires": "2000-01-01T00:00:00Z","identifiers":[{"type":"dns", "value":"example.com"}], "profile": "default", "authorizations":["http://localhost/acme/authz/1/1"],"finalize":"http://localhost/acme/finalize/1/1","certificate":"http://localhost/acme/cert/serial"}`,
		},
		{
			Name:     "404 request",
			Request:  makeGet("1/2"),
			Response: `{"type":"` + probs.ErrorNS + `malformed","detail":"No order for ID 2", "status":404}`,
		},
		{
			Name:     "Invalid request path",
			Request:  makeGet("asd"),
			Response: `{"type":"` + probs.ErrorNS + `malformed","detail":"Invalid request path","status":404}`,
		},
		{
			Name:     "Invalid account ID",
			Request:  makeGet("asd/asd"),
			Response: `{"type":"` + probs.ErrorNS + `malformed","detail":"Invalid account ID","status":400}`,
		},
		{
			Name:     "Invalid order ID",
			Request:  makeGet("1/asd"),
			Response: `{"type":"` + probs.ErrorNS + `malformed","detail":"Invalid order ID","status":400}`,
		},
		{
			Name:     "Real request, wrong account",
			Request:  makeGet("2/1"),
			Response: `{"type":"` + probs.ErrorNS + `malformed","detail":"No order found for account ID 2", "status":404}`,
		},
		{
			Name:     "Internal error request",
			Request:  makeGet("1/3"),
			Response: `{"type":"` + probs.ErrorNS + `serverInternal","detail":"Failed to retrieve order for ID 3","status":500}`,
		},
		{
			Name:     "Invalid POST-as-GET",
			Request:  makePost(1, "1/1", "{}"),
			Response: `{"type":"` + probs.ErrorNS + `malformed","detail":"Unable to validate JWS :: POST-as-GET requests must have an empty payload", "status":400}`,
		},
		{
			Name:     "Valid POST-as-GET, wrong account",
			Request:  makePost(1, "2/1", ""),
			Response: `{"type":"` + probs.ErrorNS + `malformed","detail":"No order found for account ID 2", "status":404}`,
		},
		{
			Name:     "Valid POST-as-GET",
			Request:  makePost(1, "1/1", ""),
			Response: `{"status": "valid","expires": "2000-01-01T00:00:00Z","identifiers":[{"type":"dns", "value":"example.com"}], "profile": "default", "authorizations":["http://localhost/acme/authz/1/1"],"finalize":"http://localhost/acme/finalize/1/1","certificate":"http://localhost/acme/cert/serial"}`,
		},
		{
			Name:     "GET new order from old endpoint",
			Request:  makeGet("1/9"),
			Response: `{"status": "valid","expires": "2000-01-01T00:00:00Z","identifiers":[{"type":"dns", "value":"example.com"}], "profile": "default", "authorizations":["http://localhost/acme/authz/1/1"],"finalize":"http://localhost/acme/finalize/1/9","certificate":"http://localhost/acme/cert/serial"}`,
		},
		{
			Name:     "POST-as-GET new order",
			Request:  makePost(1, "1/9", ""),
			Response: `{"status": "valid","expires": "2000-01-01T00:00:00Z","identifiers":[{"type":"dns", "value":"example.com"}], "profile": "default", "authorizations":["http://localhost/acme/authz/1/1"],"finalize":"http://localhost/acme/finalize/1/9","certificate":"http://localhost/acme/cert/serial"}`,
		},
		{
			Name:     "POST-as-GET processing order",
			Request:  makePost(1, "1/10", ""),
			Response: `{"status": "processing","expires": "2000-01-01T00:00:00Z","identifiers":[{"type":"dns", "value":"example.com"}], "profile": "default", "authorizations":["http://localhost/acme/authz/1/1"],"finalize":"http://localhost/acme/finalize/1/10"}`,
			Headers:  map[string]string{"Retry-After": "3"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			responseWriter := httptest.NewRecorder()
			wfe.GetOrder(ctx, newRequestEvent(), responseWriter, tc.Request)
			t.Log(tc.Name)
			t.Log("actual:", responseWriter.Body.String())
			t.Log("expect:", tc.Response)
			test.AssertUnmarshaledEquals(t, responseWriter.Body.String(), tc.Response)
			for k, v := range tc.Headers {
				test.AssertEquals(t, responseWriter.Header().Get(k), v)
			}
		})
	}
}

func makeRevokeRequestJSON(reason *revocation.Reason) ([]byte, error) {
	certPemBytes, err := os.ReadFile("../test/hierarchy/ee-r3.cert.pem")
	if err != nil {
		return nil, err
	}
	certBlock, _ := pem.Decode(certPemBytes)
	return makeRevokeRequestJSONForCert(certBlock.Bytes, reason)
}

func makeRevokeRequestJSONForCert(der []byte, reason *revocation.Reason) ([]byte, error) {
	revokeRequest := struct {
		CertificateDER core.JSONBuffer    `json:"certificate"`
		Reason         *revocation.Reason `json:"reason"`
	}{
		CertificateDER: der,
		Reason:         reason,
	}
	revokeRequestJSON, err := json.Marshal(revokeRequest)
	if err != nil {
		return nil, err
	}
	return revokeRequestJSON, nil
}

// Valid revocation request for existing, non-revoked cert, signed using the
// issuing account key.
func TestRevokeCertificateByApplicantValid(t *testing.T) {
	wfe, _, signer := setupWFE(t)
	wfe.sa = newMockSAWithCert(t, wfe.sa)

	mockLog := wfe.log.(*blog.Mock)
	mockLog.Clear()

	revokeRequestJSON, err := makeRevokeRequestJSON(nil)
	test.AssertNotError(t, err, "Failed to make revokeRequestJSON")
	_, _, jwsBody := signer.byKeyID(1, nil, "http://localhost/revoke-cert", string(revokeRequestJSON))

	responseWriter := httptest.NewRecorder()
	wfe.RevokeCertificate(ctx, newRequestEvent(), responseWriter,
		makePostRequestWithPath("revoke-cert", jwsBody))

	test.AssertEquals(t, responseWriter.Code, 200)
	test.AssertEquals(t, responseWriter.Body.String(), "")
	test.AssertDeepEquals(t, mockLog.GetAllMatching("Authenticated revocation"), []string{
		`INFO: [AUDIT] Authenticated revocation JSON={"Serial":"000000000000000000001d72443db5189821","Reason":0,"RegID":1,"Method":"applicant"}`,
	})
}

// Valid revocation request for existing, non-revoked cert, signed using the
// certificate private key.
func TestRevokeCertificateByKeyValid(t *testing.T) {
	wfe, _, signer := setupWFE(t)
	wfe.sa = newMockSAWithCert(t, wfe.sa)

	mockLog := wfe.log.(*blog.Mock)
	mockLog.Clear()

	keyPemBytes, err := os.ReadFile("../test/hierarchy/ee-r3.key.pem")
	test.AssertNotError(t, err, "Failed to load key")
	key := loadKey(t, keyPemBytes)

	revocationReason := revocation.Reason(ocsp.KeyCompromise)
	revokeRequestJSON, err := makeRevokeRequestJSON(&revocationReason)
	test.AssertNotError(t, err, "Failed to make revokeRequestJSON")
	_, _, jwsBody := signer.embeddedJWK(key, "http://localhost/revoke-cert", string(revokeRequestJSON))

	responseWriter := httptest.NewRecorder()
	wfe.RevokeCertificate(ctx, newRequestEvent(), responseWriter,
		makePostRequestWithPath("revoke-cert", jwsBody))

	test.AssertEquals(t, responseWriter.Code, 200)
	test.AssertEquals(t, responseWriter.Body.String(), "")
	test.AssertDeepEquals(t, mockLog.GetAllMatching("Authenticated revocation"), []string{
		`INFO: [AUDIT] Authenticated revocation JSON={"Serial":"000000000000000000001d72443db5189821","Reason":1,"RegID":0,"Method":"privkey"}`,
	})
}

// Invalid revocation request: although signed with the cert key, the cert
// wasn't issued by any issuer the Boulder is aware of.
func TestRevokeCertificateNotIssued(t *testing.T) {
	wfe, _, signer := setupWFE(t)
	wfe.sa = newMockSAWithCert(t, wfe.sa)

	// Make a self-signed junk certificate
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "unexpected error making random private key")
	// Use a known serial from the mockSAWithValidCert mock.
	// This ensures that any failures here are due to the certificate's issuer
	// not matching up with issuers known by the mock, rather than due to the
	// certificate's serial not matching up with serials known by the mock.
	knownCert, err := core.LoadCert("../test/hierarchy/ee-r3.cert.pem")
	test.AssertNotError(t, err, "Unexpected error loading test cert")
	template := &x509.Certificate{
		SerialNumber: knownCert.SerialNumber,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, k.Public(), k)
	test.AssertNotError(t, err, "Unexpected error creating self-signed junk cert")

	keyPemBytes, err := os.ReadFile("../test/hierarchy/ee-r3.key.pem")
	test.AssertNotError(t, err, "Failed to load key")
	key := loadKey(t, keyPemBytes)

	revokeRequestJSON, err := makeRevokeRequestJSONForCert(certDER, nil)
	test.AssertNotError(t, err, "Failed to make revokeRequestJSON for certDER")
	_, _, jwsBody := signer.embeddedJWK(key, "http://localhost/revoke-cert", string(revokeRequestJSON))

	responseWriter := httptest.NewRecorder()
	wfe.RevokeCertificate(ctx, newRequestEvent(), responseWriter,
		makePostRequestWithPath("revoke-cert", jwsBody))
	// It should result in a 404 response with a problem body
	test.AssertEquals(t, responseWriter.Code, 404)
	test.AssertEquals(t, responseWriter.Body.String(), "{\n  \"type\": \"urn:ietf:params:acme:error:malformed\",\n  \"detail\": \"Unable to revoke :: Certificate from unrecognized issuer\",\n  \"status\": 404\n}")
}

func TestRevokeCertificateExpired(t *testing.T) {
	wfe, fc, signer := setupWFE(t)
	wfe.sa = newMockSAWithCert(t, wfe.sa)

	keyPemBytes, err := os.ReadFile("../test/hierarchy/ee-r3.key.pem")
	test.AssertNotError(t, err, "Failed to load key")
	key := loadKey(t, keyPemBytes)

	revokeRequestJSON, err := makeRevokeRequestJSON(nil)
	test.AssertNotError(t, err, "Failed to make revokeRequestJSON")

	_, _, jwsBody := signer.embeddedJWK(key, "http://localhost/revoke-cert", string(revokeRequestJSON))

	cert, err := core.LoadCert("../test/hierarchy/ee-r3.cert.pem")
	test.AssertNotError(t, err, "Failed to load test certificate")

	fc.Set(cert.NotAfter.Add(time.Hour))

	responseWriter := httptest.NewRecorder()
	wfe.RevokeCertificate(ctx, newRequestEvent(), responseWriter,
		makePostRequestWithPath("revoke-cert", jwsBody))
	test.AssertEquals(t, responseWriter.Code, 403)
	test.AssertEquals(t, responseWriter.Body.String(), "{\n  \"type\": \"urn:ietf:params:acme:error:unauthorized\",\n  \"detail\": \"Unable to revoke :: Certificate is expired\",\n  \"status\": 403\n}")
}

func TestRevokeCertificateReasons(t *testing.T) {
	wfe, _, signer := setupWFE(t)
	wfe.sa = newMockSAWithCert(t, wfe.sa)
	ra := wfe.ra.(*MockRegistrationAuthority)

	reason0 := revocation.Reason(ocsp.Unspecified)
	reason1 := revocation.Reason(ocsp.KeyCompromise)
	reason2 := revocation.Reason(ocsp.CACompromise)
	reason100 := revocation.Reason(100)

	testCases := []struct {
		Name             string
		Reason           *revocation.Reason
		ExpectedHTTPCode int
		ExpectedBody     string
		ExpectedReason   *revocation.Reason
	}{
		{
			Name:             "Valid reason",
			Reason:           &reason1,
			ExpectedHTTPCode: http.StatusOK,
			ExpectedReason:   &reason1,
		},
		{
			Name:             "No reason",
			ExpectedHTTPCode: http.StatusOK,
			ExpectedReason:   &reason0,
		},
		{
			Name:             "Unsupported reason",
			Reason:           &reason2,
			ExpectedHTTPCode: http.StatusBadRequest,
			ExpectedBody:     `{"type":"` + probs.ErrorNS + `badRevocationReason","detail":"Unable to revoke :: disallowed revocation reason: 2","status":400}`,
		},
		{
			Name:             "Non-existent reason",
			Reason:           &reason100,
			ExpectedHTTPCode: http.StatusBadRequest,
			ExpectedBody:     `{"type":"` + probs.ErrorNS + `badRevocationReason","detail":"Unable to revoke :: disallowed revocation reason: 100","status":400}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			revokeRequestJSON, err := makeRevokeRequestJSON(tc.Reason)
			test.AssertNotError(t, err, "Failed to make revokeRequestJSON")
			_, _, jwsBody := signer.byKeyID(1, nil, "http://localhost/revoke-cert", string(revokeRequestJSON))

			responseWriter := httptest.NewRecorder()
			wfe.RevokeCertificate(ctx, newRequestEvent(), responseWriter,
				makePostRequestWithPath("revoke-cert", jwsBody))

			test.AssertEquals(t, responseWriter.Code, tc.ExpectedHTTPCode)
			if tc.ExpectedBody != "" {
				test.AssertUnmarshaledEquals(t, responseWriter.Body.String(), tc.ExpectedBody)
			} else {
				test.AssertEquals(t, responseWriter.Body.String(), tc.ExpectedBody)
			}
			if tc.ExpectedReason != nil {
				test.AssertEquals(t, ra.lastRevocationReason, *tc.ExpectedReason)
			}
		})
	}
}

// A revocation request signed by an incorrect certificate private key.
func TestRevokeCertificateWrongCertificateKey(t *testing.T) {
	wfe, _, signer := setupWFE(t)
	wfe.sa = newMockSAWithCert(t, wfe.sa)

	keyPemBytes, err := os.ReadFile("../test/hierarchy/ee-e1.key.pem")
	test.AssertNotError(t, err, "Failed to load key")
	key := loadKey(t, keyPemBytes)

	revocationReason := revocation.Reason(ocsp.KeyCompromise)
	revokeRequestJSON, err := makeRevokeRequestJSON(&revocationReason)
	test.AssertNotError(t, err, "Failed to make revokeRequestJSON")
	_, _, jwsBody := signer.embeddedJWK(key, "http://localhost/revoke-cert", string(revokeRequestJSON))

	responseWriter := httptest.NewRecorder()
	wfe.RevokeCertificate(ctx, newRequestEvent(), responseWriter,
		makePostRequestWithPath("revoke-cert", jwsBody))
	test.AssertEquals(t, responseWriter.Code, 403)
	test.AssertUnmarshaledEquals(t, responseWriter.Body.String(),
		`{"type":"`+probs.ErrorNS+`unauthorized","detail":"Unable to revoke :: JWK embedded in revocation request must be the same public key as the cert to be revoked","status":403}`)
}

type mockSAGetRegByKeyFails struct {
	sapb.StorageAuthorityReadOnlyClient
}

func (sa *mockSAGetRegByKeyFails) GetRegistrationByKey(_ context.Context, req *sapb.JSONWebKey, _ ...grpc.CallOption) (*corepb.Registration, error) {
	return nil, fmt.Errorf("whoops")
}

// When SA.GetRegistrationByKey errors (e.g. gRPC timeout), NewAccount should
// return internal server errors.
func TestNewAccountWhenGetRegByKeyFails(t *testing.T) {
	wfe, _, signer := setupWFE(t)
	wfe.sa = &mockSAGetRegByKeyFails{wfe.sa}
	key := loadKey(t, []byte(testE2KeyPrivatePEM))
	_, ok := key.(*ecdsa.PrivateKey)
	test.Assert(t, ok, "Couldn't load ECDSA key")
	payload := `{"contact":["mailto:person@mail.com"],"agreement":"` + agreementURL + `"}`
	responseWriter := httptest.NewRecorder()
	_, _, body := signer.embeddedJWK(key, "http://localhost/new-account", payload)
	wfe.NewAccount(ctx, newRequestEvent(), responseWriter, makePostRequestWithPath("/new-account", body))
	if responseWriter.Code != 500 {
		t.Fatalf("Wrong response code %d for NewAccount with failing GetRegByKey (wanted 500)", responseWriter.Code)
	}
	var prob probs.ProblemDetails
	err := json.Unmarshal(responseWriter.Body.Bytes(), &prob)
	test.AssertNotError(t, err, "unmarshalling response")
	if prob.Type != probs.ErrorNS+probs.ServerInternalProblem {
		t.Errorf("Wrong type for returned problem: %#v", prob.Type)
	}
}

type mockSAGetRegByKeyNotFound struct {
	sapb.StorageAuthorityReadOnlyClient
}

func (sa *mockSAGetRegByKeyNotFound) GetRegistrationByKey(_ context.Context, req *sapb.JSONWebKey, _ ...grpc.CallOption) (*corepb.Registration, error) {
	return nil, berrors.NotFoundError("not found")
}

func TestNewAccountWhenGetRegByKeyNotFound(t *testing.T) {
	wfe, _, signer := setupWFE(t)
	wfe.sa = &mockSAGetRegByKeyNotFound{wfe.sa}
	key := loadKey(t, []byte(testE2KeyPrivatePEM))
	_, ok := key.(*ecdsa.PrivateKey)
	test.Assert(t, ok, "Couldn't load ECDSA key")
	// When SA.GetRegistrationByKey returns NotFound, and no onlyReturnExisting
	// field is sent, NewAccount should succeed.
	payload := `{"contact":["mailto:person@mail.com"],"termsOfServiceAgreed":true}`
	signedURL := "http://localhost/new-account"
	responseWriter := httptest.NewRecorder()
	_, _, body := signer.embeddedJWK(key, signedURL, payload)
	wfe.NewAccount(ctx, newRequestEvent(), responseWriter, makePostRequestWithPath("/new-account", body))
	if responseWriter.Code != http.StatusCreated {
		t.Errorf("Bad response to NewRegistration: %d, %s", responseWriter.Code, responseWriter.Body)
	}

	// When SA.GetRegistrationByKey returns NotFound, and onlyReturnExisting
	// field **is** sent, NewAccount should fail with the expected error.
	payload = `{"contact":["mailto:person@mail.com"],"termsOfServiceAgreed":true,"onlyReturnExisting":true}`
	responseWriter = httptest.NewRecorder()
	_, _, body = signer.embeddedJWK(key, signedURL, payload)
	// Process the new account request
	wfe.NewAccount(ctx, newRequestEvent(), responseWriter, makePostRequestWithPath("/new-account", body))
	test.AssertEquals(t, responseWriter.Code, http.StatusBadRequest)
	test.AssertUnmarshaledEquals(t, responseWriter.Body.String(), `
	{
		"type": "urn:ietf:params:acme:error:accountDoesNotExist",
		"detail": "No account exists with the provided key",
		"status": 400
	}`)
}

func TestPrepAuthzForDisplay(t *testing.T) {
	t.Parallel()
	wfe, _, _ := setupWFE(t)

	authz := &core.Authorization{
		ID:             "12345",
		Status:         core.StatusPending,
		RegistrationID: 1,
		Identifier:     identifier.NewDNS("example.com"),
		Challenges: []core.Challenge{
			{Type: core.ChallengeTypeDNS01, Status: core.StatusPending, Token: "token"},
			{Type: core.ChallengeTypeHTTP01, Status: core.StatusPending, Token: "token"},
			{Type: core.ChallengeTypeTLSALPN01, Status: core.StatusPending, Token: "token"},
		},
	}

	// This modifies the authz in-place.
	wfe.prepAuthorizationForDisplay(&http.Request{Host: "localhost"}, authz)

	// Ensure ID and RegID are omitted.
	authzJSON, err := json.Marshal(authz)
	test.AssertNotError(t, err, "Failed to marshal authz")
	test.AssertNotContains(t, string(authzJSON), "\"id\":\"12345\"")
	test.AssertNotContains(t, string(authzJSON), "\"registrationID\":\"1\"")
}

func TestPrepRevokedAuthzForDisplay(t *testing.T) {
	t.Parallel()
	wfe, _, _ := setupWFE(t)

	authz := &core.Authorization{
		ID:             "12345",
		Status:         core.StatusInvalid,
		RegistrationID: 1,
		Identifier:     identifier.NewDNS("example.com"),
		Challenges: []core.Challenge{
			{Type: core.ChallengeTypeDNS01, Status: core.StatusPending, Token: "token"},
			{Type: core.ChallengeTypeHTTP01, Status: core.StatusPending, Token: "token"},
			{Type: core.ChallengeTypeTLSALPN01, Status: core.StatusPending, Token: "token"},
		},
	}

	// This modifies the authz in-place.
	wfe.prepAuthorizationForDisplay(&http.Request{Host: "localhost"}, authz)

	// All of the challenges should be revoked as well.
	for _, chall := range authz.Challenges {
		test.AssertEquals(t, chall.Status, core.StatusInvalid)
	}
}

func TestPrepWildcardAuthzForDisplay(t *testing.T) {
	t.Parallel()
	wfe, _, _ := setupWFE(t)

	authz := &core.Authorization{
		ID:             "12345",
		Status:         core.StatusPending,
		RegistrationID: 1,
		Identifier:     identifier.NewDNS("*.example.com"),
		Challenges: []core.Challenge{
			{Type: core.ChallengeTypeDNS01, Status: core.StatusPending, Token: "token"},
		},
	}

	// This modifies the authz in-place.
	wfe.prepAuthorizationForDisplay(&http.Request{Host: "localhost"}, authz)

	// The identifier should not start with a star, but the authz should be marked
	// as a wildcard.
	test.AssertEquals(t, strings.HasPrefix(authz.Identifier.Value, "*."), false)
	test.AssertEquals(t, authz.Wildcard, true)
}

func TestPrepAuthzForDisplayShuffle(t *testing.T) {
	t.Parallel()
	wfe, _, _ := setupWFE(t)

	authz := &core.Authorization{
		ID:             "12345",
		Status:         core.StatusPending,
		RegistrationID: 1,
		Identifier:     identifier.NewDNS("example.com"),
		Challenges: []core.Challenge{
			{Type: core.ChallengeTypeDNS01, Status: core.StatusPending, Token: "token"},
			{Type: core.ChallengeTypeHTTP01, Status: core.StatusPending, Token: "token"},
			{Type: core.ChallengeTypeTLSALPN01, Status: core.StatusPending, Token: "token"},
		},
	}

	// The challenges should be presented in an unpredictable order.

	// Create a structure to count how many times each challenge type ends up in
	// each position in the output authz.Challenges list.
	counts := make(map[core.AcmeChallenge]map[int]int)
	counts[core.ChallengeTypeDNS01] = map[int]int{0: 0, 1: 0, 2: 0}
	counts[core.ChallengeTypeHTTP01] = map[int]int{0: 0, 1: 0, 2: 0}
	counts[core.ChallengeTypeTLSALPN01] = map[int]int{0: 0, 1: 0, 2: 0}

	// Prep the authz 100 times, and count where each challenge ended up each time.
	for range 100 {
		// This modifies the authz in place
		wfe.prepAuthorizationForDisplay(&http.Request{Host: "localhost"}, authz)
		for i, chall := range authz.Challenges {
			counts[chall.Type][i] += 1
		}
	}

	// Ensure that at least some amount of randomization is happening.
	for challType, indices := range counts {
		for index, count := range indices {
			test.Assert(t, count > 10, fmt.Sprintf("challenge type %s did not appear in position %d as often as expected", challType, index))
		}
	}
}

// noSCTMockRA is a mock RA that always returns a `berrors.MissingSCTsError` from `FinalizeOrder`
type noSCTMockRA struct {
	MockRegistrationAuthority
}

func (ra *noSCTMockRA) FinalizeOrder(context.Context, *rapb.FinalizeOrderRequest, ...grpc.CallOption) (*corepb.Order, error) {
	return nil, berrors.MissingSCTsError("noSCTMockRA missing scts error")
}

func TestFinalizeSCTError(t *testing.T) {
	wfe, _, signer := setupWFE(t)

	// Set up an RA mock that always returns a berrors.MissingSCTsError from
	// `FinalizeOrder`
	wfe.ra = &noSCTMockRA{}

	// Create a response writer to capture the WFE response
	responseWriter := httptest.NewRecorder()

	// This example is a well-formed CSR for the name "example.com".
	goodCertCSRPayload := `{
		"csr": "MIHRMHgCAQAwFjEUMBIGA1UEAxMLZXhhbXBsZS5jb20wWTATBgcqhkjOPQIBBggqhkjOPQMBBwNCAAQ2hlvArQl5k0L1eF1vF5dwr7ASm2iKqibmauund-z3QJpuudnNEjlyOXi-IY1rxyhehRrtbm_bbcNCtZLgbkPvoAAwCgYIKoZIzj0EAwIDSQAwRgIhAJ8z2EDll2BvoNRotAknEfrqeP6K5CN1NeVMB4QOu0G1AiEAqAVpiGwNyV7SEZ67vV5vyuGsKPAGnqrisZh5Vg5JKHE="
	}`

	// Create a finalization request with the above payload
	request := signAndPost(signer, "1/8", "http://localhost/1/8", goodCertCSRPayload)

	// POST the finalize order request.
	wfe.FinalizeOrder(ctx, newRequestEvent(), responseWriter, request)

	// We expect the berrors.MissingSCTsError error to have been converted into
	// a serverInternal error with the right message.
	test.AssertUnmarshaledEquals(t,
		responseWriter.Body.String(),
		`{"type":"`+probs.ErrorNS+`serverInternal","detail":"Error finalizing order :: Unable to meet CA SCT embedding requirements","status":500}`)
}

func TestOrderToOrderJSONV2Authorizations(t *testing.T) {
	wfe, fc, _ := setupWFE(t)
	expires := fc.Now()
	orderJSON := wfe.orderToOrderJSON(&http.Request{}, &corepb.Order{
		Id:               1,
		RegistrationID:   1,
		Identifiers:      []*corepb.Identifier{identifier.NewDNS("a").ToProto()},
		Status:           string(core.StatusPending),
		Expires:          timestamppb.New(expires),
		V2Authorizations: []int64{1, 2},
	})
	test.AssertDeepEquals(t, orderJSON.Authorizations, []string{
		"http://localhost/acme/authz/1/1",
		"http://localhost/acme/authz/1/2",
	})
}

func TestPrepAccountForDisplay(t *testing.T) {
	acct := &core.Registration{
		ID:        1987,
		Agreement: "disagreement",
	}

	// Prep the account for display.
	prepAccountForDisplay(acct)

	// The Agreement should always be cleared.
	test.AssertEquals(t, acct.Agreement, "")
	// The ID field should be zeroed.
	test.AssertEquals(t, acct.ID, int64(0))
}

// TestGet404 tests that a 404 is served and that the expected endpoint of
// "/" is logged when an unknown path is requested. This will test the
// codepath to the wfe.Index() handler which handles "/" and all non-api
// endpoint requests to make sure the endpoint is set properly in the logs.
func TestIndexGet404(t *testing.T) {
	// Setup
	wfe, _, _ := setupWFE(t)
	path := "/nopathhere/nope/nofilehere"
	req := &http.Request{URL: &url.URL{Path: path}, Method: "GET"}
	logEvent := &web.RequestEvent{}
	responseWriter := httptest.NewRecorder()

	// Send a request to wfe.Index()
	wfe.Index(context.Background(), logEvent, responseWriter, req)

	// Test that a 404 is received as expected
	test.AssertEquals(t, responseWriter.Code, http.StatusNotFound)
	// Test that we logged the "/" endpoint
	test.AssertEquals(t, logEvent.Endpoint, "/")
	// Test that the rest of the path is logged as the slug
	test.AssertEquals(t, logEvent.Slug, path[1:])
}

// TestARI tests that requests for real certs result in renewal info, while
// requests for certs that don't exist result in errors.
func TestARI(t *testing.T) {
	wfe, _, _ := setupWFE(t)
	msa := newMockSAWithCert(t, wfe.sa)
	wfe.sa = msa

	features.Set(features.Config{ServeRenewalInfo: true})
	defer features.Reset()

	makeGet := func(path, endpoint string) (*http.Request, *web.RequestEvent) {
		return &http.Request{URL: &url.URL{Path: path}, Method: "GET"},
			&web.RequestEvent{Endpoint: endpoint, Extra: map[string]interface{}{}}
	}

	// Load the leaf certificate.
	cert, err := core.LoadCert("../test/hierarchy/ee-r3.cert.pem")
	test.AssertNotError(t, err, "failed to load test certificate")

	// Ensure that a correct draft-ietf-acme-ari03 query results in a 200.
	certID := fmt.Sprintf("%s.%s",
		base64.RawURLEncoding.EncodeToString(cert.AuthorityKeyId),
		base64.RawURLEncoding.EncodeToString(cert.SerialNumber.Bytes()),
	)
	req, event := makeGet(certID, renewalInfoPath)
	resp := httptest.NewRecorder()
	wfe.RenewalInfo(context.Background(), event, resp, req)
	test.AssertEquals(t, resp.Code, http.StatusOK)
	test.AssertEquals(t, resp.Header().Get("Retry-After"), "21600")
	var ri core.RenewalInfo
	err = json.Unmarshal(resp.Body.Bytes(), &ri)
	test.AssertNotError(t, err, "unmarshalling renewal info")
	test.Assert(t, ri.SuggestedWindow.Start.After(cert.NotBefore), "suggested window begins before cert issuance")
	test.Assert(t, ri.SuggestedWindow.End.Before(cert.NotAfter), "suggested window ends after cert expiry")

	// Ensure that a correct draft-ietf-acme-ari03 query for a revoked cert
	// results in a renewal window in the past.
	msa.status = core.OCSPStatusRevoked
	req, event = makeGet(certID, renewalInfoPath)
	resp = httptest.NewRecorder()
	wfe.RenewalInfo(context.Background(), event, resp, req)
	test.AssertEquals(t, resp.Code, http.StatusOK)
	test.AssertEquals(t, resp.Header().Get("Retry-After"), "21600")
	err = json.Unmarshal(resp.Body.Bytes(), &ri)
	test.AssertNotError(t, err, "unmarshalling renewal info")
	test.Assert(t, ri.SuggestedWindow.End.Before(wfe.clk.Now()), "suggested window should end in the past")
	test.Assert(t, ri.SuggestedWindow.Start.Before(ri.SuggestedWindow.End), "suggested window should start before it ends")

	// Ensure that a draft-ietf-acme-ari03 query for a non-existent serial
	// results in a 404.
	certID = fmt.Sprintf("%s.%s",
		base64.RawURLEncoding.EncodeToString(cert.AuthorityKeyId),
		base64.RawURLEncoding.EncodeToString(
			big.NewInt(0).Add(cert.SerialNumber, big.NewInt(1)).Bytes(),
		),
	)
	req, event = makeGet(certID, renewalInfoPath)
	resp = httptest.NewRecorder()
	wfe.RenewalInfo(context.Background(), event, resp, req)
	test.AssertEquals(t, resp.Code, http.StatusNotFound)
	test.AssertEquals(t, resp.Header().Get("Retry-After"), "")

	// Ensure that a query with a non-CertID path fails.
	req, event = makeGet("lolwutsup", renewalInfoPath)
	resp = httptest.NewRecorder()
	wfe.RenewalInfo(context.Background(), event, resp, req)
	test.AssertEquals(t, resp.Code, http.StatusBadRequest)
	test.AssertContains(t, resp.Body.String(), "Invalid path")

	// Ensure that a query with no path slug at all bails out early.
	req, event = makeGet("", renewalInfoPath)
	resp = httptest.NewRecorder()
	wfe.RenewalInfo(context.Background(), event, resp, req)
	test.AssertEquals(t, resp.Code, http.StatusNotFound)
	test.AssertContains(t, resp.Body.String(), "Must specify a request path")
}

// TestIncidentARI tests that requests certs impacted by an ongoing revocation
// incident result in a 200 with a retry-after header and a suggested retry
// window in the past.
func TestIncidentARI(t *testing.T) {
	wfe, _, _ := setupWFE(t)
	expectSerial := big.NewInt(12345)
	expectSerialString := core.SerialToString(big.NewInt(12345))
	wfe.sa = newMockSAWithIncident(wfe.sa, []string{expectSerialString})

	features.Set(features.Config{ServeRenewalInfo: true})
	defer features.Reset()

	makeGet := func(path, endpoint string) (*http.Request, *web.RequestEvent) {
		return &http.Request{URL: &url.URL{Path: path}, Method: "GET"},
			&web.RequestEvent{Endpoint: endpoint, Extra: map[string]interface{}{}}
	}

	var issuer issuance.NameID
	for k := range wfe.issuerCertificates {
		// Grab the first known issuer.
		issuer = k
		break
	}
	certID := fmt.Sprintf("%s.%s",
		base64.RawURLEncoding.EncodeToString(wfe.issuerCertificates[issuer].SubjectKeyId),
		base64.RawURLEncoding.EncodeToString(expectSerial.Bytes()),
	)
	req, event := makeGet(certID, renewalInfoPath)
	resp := httptest.NewRecorder()
	wfe.RenewalInfo(context.Background(), event, resp, req)
	test.AssertEquals(t, resp.Code, 200)
	test.AssertEquals(t, resp.Header().Get("Retry-After"), "21600")
	var ri core.RenewalInfo
	err := json.Unmarshal(resp.Body.Bytes(), &ri)
	test.AssertNotError(t, err, "unmarshalling renewal info")
	// The start of the window should be in the past.
	test.AssertEquals(t, ri.SuggestedWindow.Start.Before(wfe.clk.Now()), true)
	// The end of the window should be after the start.
	test.AssertEquals(t, ri.SuggestedWindow.End.After(ri.SuggestedWindow.Start), true)
	// The end of the window should also be in the past.
	test.AssertEquals(t, ri.SuggestedWindow.End.Before(wfe.clk.Now()), true)
	// The explanationURL should be set.
	test.AssertEquals(t, ri.ExplanationURL, "http://big.bad/incident")
}

func Test_sendError(t *testing.T) {
	features.Reset()
	wfe, _, _ := setupWFE(t)
	testResponse := httptest.NewRecorder()

	testErr := berrors.RateLimitError(0, "test")
	wfe.sendError(testResponse, &web.RequestEvent{Endpoint: "test"}, probs.RateLimited("test"), testErr)
	// Ensure a 0 value RetryAfter results in no Retry-After header.
	test.AssertEquals(t, testResponse.Header().Get("Retry-After"), "")
	// Ensure the Link header isn't populatsed.
	test.AssertEquals(t, testResponse.Header().Get("Link"), "")

	testErr = berrors.RateLimitError(time.Millisecond*500, "test")
	wfe.sendError(testResponse, &web.RequestEvent{Endpoint: "test"}, probs.RateLimited("test"), testErr)
	// Ensure a 500ms RetryAfter is rounded up to a 1s Retry-After header.
	test.AssertEquals(t, testResponse.Header().Get("Retry-After"), "1")
	// Ensure the Link header is populated.
	test.AssertEquals(t, testResponse.Header().Get("Link"), "<https://letsencrypt.org/docs/rate-limits>;rel=\"help\"")

	// Clear headers for the next test.
	testResponse.Header().Del("Retry-After")
	testResponse.Header().Del("Link")

	testErr = berrors.RateLimitError(time.Millisecond*499, "test")
	wfe.sendError(testResponse, &web.RequestEvent{Endpoint: "test"}, probs.RateLimited("test"), testErr)
	// Ensure a 499ms RetryAfter results in no Retry-After header.
	test.AssertEquals(t, testResponse.Header().Get("Retry-After"), "")
	// Ensure the Link header isn't populatsed.
	test.AssertEquals(t, testResponse.Header().Get("Link"), "")
}

func Test_sendErrorInternalServerError(t *testing.T) {
	features.Reset()
	wfe, _, _ := setupWFE(t)
	testResponse := httptest.NewRecorder()

	wfe.sendError(testResponse, &web.RequestEvent{}, probs.ServerInternal("oh no"), nil)
	test.AssertEquals(t, testResponse.Header().Get("Retry-After"), "60")
}

// mockSAForARI provides a mock SA with the methods required for an issuance and
// a renewal with the ARI `Replaces` field.
//
// Note that FQDNSetTimestampsForWindow always return an empty list, which allows us to act
// as if a certificate is not getting the renewal exemption, even when we are repeatedly
// issuing for the same names.
type mockSAForARI struct {
	sapb.StorageAuthorityReadOnlyClient
	cert *corepb.Certificate
}

func (sa *mockSAForARI) FQDNSetTimestampsForWindow(ctx context.Context, in *sapb.CountFQDNSetsRequest, opts ...grpc.CallOption) (*sapb.Timestamps, error) {
	return &sapb.Timestamps{Timestamps: nil}, nil
}

// GetCertificate returns the inner certificate if it matches the given serial.
func (sa *mockSAForARI) GetCertificate(ctx context.Context, req *sapb.Serial, _ ...grpc.CallOption) (*corepb.Certificate, error) {
	if req.Serial == sa.cert.Serial {
		return sa.cert, nil
	}
	return nil, berrors.NotFoundError("certificate with serial %q not found", req.Serial)
}

func (sa *mockSAForARI) ReplacementOrderExists(ctx context.Context, in *sapb.Serial, opts ...grpc.CallOption) (*sapb.Exists, error) {
	if in.Serial == sa.cert.Serial {
		return &sapb.Exists{Exists: false}, nil

	}
	return &sapb.Exists{Exists: true}, nil
}

func (sa *mockSAForARI) IncidentsForSerial(ctx context.Context, in *sapb.Serial, opts ...grpc.CallOption) (*sapb.Incidents, error) {
	return &sapb.Incidents{}, nil
}

func (sa *mockSAForARI) GetCertificateStatus(ctx context.Context, in *sapb.Serial, opts ...grpc.CallOption) (*corepb.CertificateStatus, error) {
	return &corepb.CertificateStatus{Serial: in.Serial, Status: string(core.OCSPStatusGood)}, nil
}

func TestOrderMatchesReplacement(t *testing.T) {
	wfe, _, _ := setupWFE(t)

	expectExpiry := time.Now().AddDate(0, 0, 1)
	expectSerial := big.NewInt(1337)
	testKey, _ := rsa.GenerateKey(rand.Reader, 1024)
	rawCert := x509.Certificate{
		NotAfter:     expectExpiry,
		DNSNames:     []string{"example.com", "example-a.com"},
		SerialNumber: expectSerial,
	}
	mockDer, err := x509.CreateCertificate(rand.Reader, &rawCert, &rawCert, &testKey.PublicKey, testKey)
	test.AssertNotError(t, err, "failed to create test certificate")

	wfe.sa = &mockSAForARI{
		cert: &corepb.Certificate{
			RegistrationID: 1,
			Serial:         expectSerial.String(),
			Der:            mockDer,
		},
	}

	// Working with a single matching identifier.
	err = wfe.orderMatchesReplacement(context.Background(), &core.Registration{ID: 1}, identifier.ACMEIdentifiers{identifier.NewDNS("example.com")}, expectSerial.String())
	test.AssertNotError(t, err, "failed to check order is replacement")

	// Working with a different matching identifier.
	err = wfe.orderMatchesReplacement(context.Background(), &core.Registration{ID: 1}, identifier.ACMEIdentifiers{identifier.NewDNS("example-a.com")}, expectSerial.String())
	test.AssertNotError(t, err, "failed to check order is replacement")

	// No matching identifiers.
	err = wfe.orderMatchesReplacement(context.Background(), &core.Registration{ID: 1}, identifier.ACMEIdentifiers{identifier.NewDNS("example-b.com")}, expectSerial.String())
	test.AssertErrorIs(t, err, berrors.Malformed)

	// RegID for predecessor order does not match.
	err = wfe.orderMatchesReplacement(context.Background(), &core.Registration{ID: 2}, identifier.ACMEIdentifiers{identifier.NewDNS("example.com")}, expectSerial.String())
	test.AssertErrorIs(t, err, berrors.Unauthorized)

	// Predecessor certificate not found.
	err = wfe.orderMatchesReplacement(context.Background(), &core.Registration{ID: 1}, identifier.ACMEIdentifiers{identifier.NewDNS("example.com")}, "1")
	test.AssertErrorIs(t, err, berrors.NotFound)
}

type mockRA struct {
	rapb.RegistrationAuthorityClient
	expectProfileName string
}

// NewOrder returns an error if the ""
func (sa *mockRA) NewOrder(ctx context.Context, in *rapb.NewOrderRequest, opts ...grpc.CallOption) (*corepb.Order, error) {
	if in.CertificateProfileName != sa.expectProfileName {
		return nil, errors.New("not expected profile name")
	}
	now := time.Now().UTC()
	created := now.AddDate(-30, 0, 0)
	exp := now.AddDate(30, 0, 0)
	return &corepb.Order{
		Id:                     123456789,
		RegistrationID:         987654321,
		Created:                timestamppb.New(created),
		Expires:                timestamppb.New(exp),
		Identifiers:            []*corepb.Identifier{identifier.NewDNS("example.com").ToProto()},
		Status:                 string(core.StatusValid),
		V2Authorizations:       []int64{1},
		CertificateSerial:      "serial",
		Error:                  nil,
		CertificateProfileName: in.CertificateProfileName,
	}, nil
}

func TestNewOrderWithProfile(t *testing.T) {
	wfe, _, signer := setupWFE(t)
	expectProfileName := "test-profile"
	wfe.ra = &mockRA{expectProfileName: expectProfileName}
	mux := wfe.Handler(metrics.NoopRegisterer)
	wfe.certProfiles = map[string]string{expectProfileName: "description"}

	// Test that the newOrder endpoint returns the proper error if an invalid
	// profile is specified.
	invalidOrderBody := `
	{
		"Identifiers": [
		  {"type": "dns", "value": "example.com"}
		],
		"Profile": "bad-profile"
	}`

	responseWriter := httptest.NewRecorder()
	r := signAndPost(signer, newOrderPath, "http://localhost"+newOrderPath, invalidOrderBody)
	mux.ServeHTTP(responseWriter, r)
	test.AssertEquals(t, responseWriter.Code, http.StatusBadRequest)
	var errorResp map[string]interface{}
	err := json.Unmarshal(responseWriter.Body.Bytes(), &errorResp)
	test.AssertNotError(t, err, "Failed to unmarshal error response")
	test.AssertEquals(t, errorResp["type"], "urn:ietf:params:acme:error:invalidProfile")
	test.AssertEquals(t, errorResp["detail"], "profile name \"bad-profile\" not recognized")

	// Test that the newOrder endpoint returns no error if the valid profile is specified.
	validOrderBody := `
	{
		"Identifiers": [
		  {"type": "dns", "value": "example.com"}
		],
		"Profile": "test-profile"
	}`
	responseWriter = httptest.NewRecorder()
	r = signAndPost(signer, newOrderPath, "http://localhost"+newOrderPath, validOrderBody)
	mux.ServeHTTP(responseWriter, r)
	test.AssertEquals(t, responseWriter.Code, http.StatusCreated)
	var errorResp1 map[string]interface{}
	err = json.Unmarshal(responseWriter.Body.Bytes(), &errorResp1)
	test.AssertNotError(t, err, "Failed to unmarshal order response")
	test.AssertEquals(t, errorResp1["status"], "valid")

	// Set the acceptable profiles to the empty set, the WFE should no longer accept any profiles.
	wfe.certProfiles = map[string]string{}
	responseWriter = httptest.NewRecorder()
	r = signAndPost(signer, newOrderPath, "http://localhost"+newOrderPath, validOrderBody)
	mux.ServeHTTP(responseWriter, r)
	test.AssertEquals(t, responseWriter.Code, http.StatusBadRequest)
	var errorResp2 map[string]interface{}
	err = json.Unmarshal(responseWriter.Body.Bytes(), &errorResp2)
	test.AssertNotError(t, err, "Failed to unmarshal error response")
	test.AssertEquals(t, errorResp2["type"], "urn:ietf:params:acme:error:invalidProfile")
	test.AssertEquals(t, errorResp2["detail"], "profile name \"test-profile\" not recognized")
}

func makeARICertID(leaf *x509.Certificate) (string, error) {
	if leaf == nil {
		return "", errors.New("leaf certificate is nil")
	}

	// Marshal the Serial Number into DER.
	der, err := asn1.Marshal(leaf.SerialNumber)
	if err != nil {
		return "", err
	}

	// Check if the DER encoded bytes are sufficient (at least 3 bytes: tag,
	// length, and value).
	if len(der) < 3 {
		return "", errors.New("invalid DER encoding of serial number")
	}

	// Extract only the integer bytes from the DER encoded Serial Number
	// Skipping the first 2 bytes (tag and length). The result is base64url
	// encoded without padding.
	serial := base64.RawURLEncoding.EncodeToString(der[2:])

	// Convert the Authority Key Identifier to base64url encoding without
	// padding.
	aki := base64.RawURLEncoding.EncodeToString(leaf.AuthorityKeyId)

	// Construct the final identifier by concatenating AKI and Serial Number.
	return fmt.Sprintf("%s.%s", aki, serial), nil
}

func TestCountNewOrderWithReplaces(t *testing.T) {
	wfe, fc, signer := setupWFE(t)

	// Pick a random issuer to "issue" expectCert.
	var issuer *issuance.Certificate
	for _, v := range wfe.issuerCertificates {
		issuer = v
		break
	}
	testKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	expectSerial := big.NewInt(1337)
	expectCert := &x509.Certificate{
		NotBefore:      fc.Now(),
		NotAfter:       fc.Now().AddDate(0, 0, 90),
		DNSNames:       []string{"example.com"},
		SerialNumber:   expectSerial,
		AuthorityKeyId: issuer.SubjectKeyId,
	}
	expectCertId, err := makeARICertID(expectCert)
	test.AssertNotError(t, err, "failed to create test cert id")
	expectDer, err := x509.CreateCertificate(rand.Reader, expectCert, expectCert, &testKey.PublicKey, testKey)
	test.AssertNotError(t, err, "failed to create test certificate")

	// MockSA that returns the certificate with the expected serial.
	wfe.sa = &mockSAForARI{
		cert: &corepb.Certificate{
			RegistrationID: 1,
			Serial:         core.SerialToString(expectSerial),
			Der:            expectDer,
			Issued:         timestamppb.New(expectCert.NotBefore),
			Expires:        timestamppb.New(expectCert.NotAfter),
		},
	}
	mux := wfe.Handler(metrics.NoopRegisterer)
	responseWriter := httptest.NewRecorder()

	// Set the fake clock forward to 1s past the suggested renewal window start
	// time.
	renewalWindowStart := core.RenewalInfoSimple(expectCert.NotBefore, expectCert.NotAfter).SuggestedWindow.Start
	fc.Set(renewalWindowStart.Add(time.Second))

	body := fmt.Sprintf(`
	{
		"Identifiers": [
		  {"type": "dns", "value": "example.com"}
		],
		"Replaces": %q
	}`, expectCertId)

	r := signAndPost(signer, newOrderPath, "http://localhost"+newOrderPath, body)
	mux.ServeHTTP(responseWriter, r)
	test.AssertEquals(t, responseWriter.Code, http.StatusCreated)
	test.AssertMetricWithLabelsEquals(t, wfe.stats.ariReplacementOrders, prometheus.Labels{"isReplacement": "true", "limitsExempt": "true"}, 1)
}

func TestNewOrderRateLimits(t *testing.T) {
	wfe, fc, signer := setupWFE(t)

	// Set the default ratelimits to only allow one new order per account per 24
	// hours.
	txnBuilder, err := ratelimits.NewTransactionBuilder(ratelimits.LimitConfigs{
		ratelimits.NewOrdersPerAccount.String(): &ratelimits.LimitConfig{
			Burst:  1,
			Count:  1,
			Period: config.Duration{Duration: time.Hour * 24}},
	})
	test.AssertNotError(t, err, "making transaction composer")
	wfe.txnBuilder = txnBuilder

	// Pick a random issuer to "issue" extantCert.
	var issuer *issuance.Certificate
	for _, v := range wfe.issuerCertificates {
		issuer = v
		break
	}
	testKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "failed to create test key")
	extantCert := &x509.Certificate{
		NotBefore:      fc.Now(),
		NotAfter:       fc.Now().AddDate(0, 0, 90),
		DNSNames:       []string{"example.com"},
		SerialNumber:   big.NewInt(1337),
		AuthorityKeyId: issuer.SubjectKeyId,
	}
	extantCertId, err := makeARICertID(extantCert)
	test.AssertNotError(t, err, "failed to create test cert id")
	extantDer, err := x509.CreateCertificate(rand.Reader, extantCert, extantCert, &testKey.PublicKey, testKey)
	test.AssertNotError(t, err, "failed to create test certificate")

	// Mock SA that returns the certificate with the expected serial.
	wfe.sa = &mockSAForARI{
		cert: &corepb.Certificate{
			RegistrationID: 1,
			Serial:         core.SerialToString(extantCert.SerialNumber),
			Der:            extantDer,
			Issued:         timestamppb.New(extantCert.NotBefore),
			Expires:        timestamppb.New(extantCert.NotAfter),
		},
	}

	// Set the fake clock forward to 1s past the suggested renewal window start
	// time.
	renewalWindowStart := core.RenewalInfoSimple(extantCert.NotBefore, extantCert.NotAfter).SuggestedWindow.Start
	fc.Set(renewalWindowStart.Add(time.Second))

	mux := wfe.Handler(metrics.NoopRegisterer)

	// Request the certificate for the first time. Because we mocked together
	// the certificate, it will have been issued 60 days ago.
	r := signAndPost(signer, newOrderPath, "http://localhost"+newOrderPath,
		`{"Identifiers": [{"type": "dns", "value": "example.com"}]}`)
	responseWriter := httptest.NewRecorder()
	mux.ServeHTTP(responseWriter, r)
	test.AssertEquals(t, responseWriter.Code, http.StatusCreated)

	// Request another, identical certificate. This should fail for violating
	// the NewOrdersPerAccount rate limit.
	r = signAndPost(signer, newOrderPath, "http://localhost"+newOrderPath,
		`{"Identifiers": [{"type": "dns", "value": "example.com"}]}`)
	responseWriter = httptest.NewRecorder()
	mux.ServeHTTP(responseWriter, r)
	features.Set(features.Config{
		UseKvLimitsForNewOrder: true,
	})
	test.AssertEquals(t, responseWriter.Code, http.StatusTooManyRequests)

	// Make a request with the "Replaces" field, which should satisfy ARI checks
	// and therefore bypass the rate limit.
	r = signAndPost(signer, newOrderPath, "http://localhost"+newOrderPath,
		fmt.Sprintf(`{"Identifiers": [{"type": "dns", "value": "example.com"}],	"Replaces": %q}`, extantCertId))
	responseWriter = httptest.NewRecorder()
	mux.ServeHTTP(responseWriter, r)
	test.AssertEquals(t, responseWriter.Code, http.StatusCreated)
}

func TestNewAccountCreatesContacts(t *testing.T) {
	t.Parallel()

	key := loadKey(t, []byte(test2KeyPrivatePEM))
	_, ok := key.(*rsa.PrivateKey)
	test.Assert(t, ok, "Couldn't load test2 key")

	path := newAcctPath
	signedURL := fmt.Sprintf("http://localhost%s", path)

	testCases := []struct {
		name     string
		contacts []string
		expected []string
	}{
		{
			name:     "No email",
			contacts: []string{},
			expected: []string{},
		},
		{
			name:     "One email",
			contacts: []string{"mailto:person@mail.com"},
			expected: []string{"person@mail.com"},
		},
		{
			name:     "Two emails",
			contacts: []string{"mailto:person1@mail.com", "mailto:person2@mail.com"},
			expected: []string{"person1@mail.com", "person2@mail.com"},
		},
		{
			name:     "Invalid email",
			contacts: []string{"mailto:lol@%mail.com"},
			expected: []string{},
		},
		{
			name:     "One valid email, one invalid email",
			contacts: []string{"mailto:person@mail.com", "mailto:lol@%mail.com"},
			expected: []string{},
		},
		{
			name:     "Valid email with non-email prefix",
			contacts: []string{"heliograph:person@mail.com"},
			expected: []string{},
		},
		{
			name: "Non-email prefix with correct field signal instructions",
			contacts: []string{`heliograph:STATION OF RECEPTION: High Ridge above Black Hollow, near Lone Pine.
AZIMUTH TO SIGNAL STATION: Due West, bearing Twin Peaks.
WATCH PERIOD: Third hour post-zenith; observation maintained for 30 minutes.
SIGNAL CODE: Standard Morse, three-flash attention signal.
ALTERNATE SITE: If no reply, move to Observation Point B at Broken Cairn.`},
			expected: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			wfe, _, signer := setupWFE(t)

			mockPardotClient, mockImpl := mocks.NewMockPardotClientImpl()
			wfe.ee = mocks.NewMockExporterImpl(mockPardotClient)

			contactsJSON, err := json.Marshal(tc.contacts)
			test.AssertNotError(t, err, "Failed to marshal contacts")

			payload := fmt.Sprintf(`{"contact":%s,"termsOfServiceAgreed":true}`, contactsJSON)
			_, _, body := signer.embeddedJWK(key, signedURL, payload)
			request := makePostRequestWithPath(path, body)

			responseWriter := httptest.NewRecorder()
			wfe.NewAccount(context.Background(), newRequestEvent(), responseWriter, request)

			for _, email := range tc.expected {
				test.AssertSliceContains(t, mockImpl.GetCreatedContacts(), email)
			}
		})
	}
}
