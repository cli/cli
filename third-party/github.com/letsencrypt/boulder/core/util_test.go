package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/netip"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/letsencrypt/boulder/identifier"
	"github.com/letsencrypt/boulder/test"
)

// challenges.go
func TestNewToken(t *testing.T) {
	token := NewToken()
	fmt.Println(token)
	tokenLength := int(math.Ceil(32 * 8 / 6.0)) // 32 bytes, b64 encoded
	if len(token) != tokenLength {
		t.Fatalf("Expected token of length %d, got %d", tokenLength, len(token))
	}
	collider := map[string]bool{}
	// Test for very blatant RNG failures:
	// Try 2^20 birthdays in a 2^72 search space...
	// our naive collision probability here is  2^-32...
	for range 1000000 {
		token = NewToken()[:12] // just sample a portion
		test.Assert(t, !collider[token], "Token collision!")
		collider[token] = true
	}
}

func TestLooksLikeAToken(t *testing.T) {
	test.Assert(t, !looksLikeAToken("R-UL_7MrV3tUUjO9v5ym2srK3dGGCwlxbVyKBdwLOS"), "Accepted short token")
	test.Assert(t, !looksLikeAToken("R-UL_7MrV3tUUjO9v5ym2srK3dGGCwlxbVyKBdwLOS%"), "Accepted invalid token")
	test.Assert(t, looksLikeAToken("R-UL_7MrV3tUUjO9v5ym2srK3dGGCwlxbVyKBdwLOSU"), "Rejected valid token")
}

func TestSerialUtils(t *testing.T) {
	serial := SerialToString(big.NewInt(100000000000000000))
	test.AssertEquals(t, serial, "00000000000000000000016345785d8a0000")

	serialNum, err := StringToSerial("00000000000000000000016345785d8a0000")
	test.AssertNotError(t, err, "Couldn't convert serial number to *big.Int")
	if serialNum.Cmp(big.NewInt(100000000000000000)) != 0 {
		t.Fatalf("Incorrect conversion, got %d", serialNum)
	}

	badSerial, err := StringToSerial("doop!!!!000")
	test.AssertContains(t, err.Error(), "invalid serial number")
	fmt.Println(badSerial)
}

func TestBuildID(t *testing.T) {
	test.AssertEquals(t, Unspecified, GetBuildID())
}

const JWK1JSON = `{
  "kty": "RSA",
  "n": "vuc785P8lBj3fUxyZchF_uZw6WtbxcorqgTyq-qapF5lrO1U82Tp93rpXlmctj6fyFHBVVB5aXnUHJ7LZeVPod7Wnfl8p5OyhlHQHC8BnzdzCqCMKmWZNX5DtETDId0qzU7dPzh0LP0idt5buU7L9QNaabChw3nnaL47iu_1Di5Wp264p2TwACeedv2hfRDjDlJmaQXuS8Rtv9GnRWyC9JBu7XmGvGDziumnJH7Hyzh3VNu-kSPQD3vuAFgMZS6uUzOztCkT0fpOalZI6hqxtWLvXUMj-crXrn-Maavz8qRhpAyp5kcYk3jiHGgQIi7QSK2JIdRJ8APyX9HlmTN5AQ",
  "e": "AQAB"
}`
const JWK1Digest = `ul04Iq07ulKnnrebv2hv3yxCGgVvoHs8hjq2tVKx3mc=`
const JWK2JSON = `{
  "kty":"RSA",
  "n":"yTsLkI8n4lg9UuSKNRC0UPHsVjNdCYk8rGXIqeb_rRYaEev3D9-kxXY8HrYfGkVt5CiIVJ-n2t50BKT8oBEMuilmypSQqJw0pCgtUm-e6Z0Eg3Ly6DMXFlycyikegiZ0b-rVX7i5OCEZRDkENAYwFNX4G7NNCwEZcH7HUMUmty9dchAqDS9YWzPh_dde1A9oy9JMH07nRGDcOzIh1rCPwc71nwfPPYeeS4tTvkjanjeigOYBFkBLQuv7iBB4LPozsGF1XdoKiIIi-8ye44McdhOTPDcQp3xKxj89aO02pQhBECv61rmbPinvjMG9DYxJmZvjsKF4bN2oy0DxdC1jDw",
  "e":"AQAB"
}`

func TestKeyDigest(t *testing.T) {
	// Test with JWK (value, reference, and direct)
	var jwk jose.JSONWebKey
	err := json.Unmarshal([]byte(JWK1JSON), &jwk)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := KeyDigestB64(jwk)
	test.Assert(t, err == nil && digest == JWK1Digest, "Failed to digest JWK by value")
	digest, err = KeyDigestB64(&jwk)
	test.Assert(t, err == nil && digest == JWK1Digest, "Failed to digest JWK by reference")
	digest, err = KeyDigestB64(jwk.Key)
	test.Assert(t, err == nil && digest == JWK1Digest, "Failed to digest bare key")

	// Test with unknown key type
	_, err = KeyDigestB64(struct{}{})
	test.Assert(t, err != nil, "Should have rejected unknown key type")
}

func TestKeyDigestEquals(t *testing.T) {
	var jwk1, jwk2 jose.JSONWebKey
	err := json.Unmarshal([]byte(JWK1JSON), &jwk1)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal([]byte(JWK2JSON), &jwk2)
	if err != nil {
		t.Fatal(err)
	}

	test.Assert(t, KeyDigestEquals(jwk1, jwk1), "Key digests for same key should match")
	test.Assert(t, !KeyDigestEquals(jwk1, jwk2), "Key digests for different keys should not match")
	test.Assert(t, !KeyDigestEquals(jwk1, struct{}{}), "Unknown key types should not match anything")
	test.Assert(t, !KeyDigestEquals(struct{}{}, struct{}{}), "Unknown key types should not match anything")
}

func TestIsAnyNilOrZero(t *testing.T) {
	test.Assert(t, IsAnyNilOrZero(nil), "Nil seen as non-zero")

	test.Assert(t, IsAnyNilOrZero(false), "False bool seen as non-zero")
	test.Assert(t, !IsAnyNilOrZero(true), "True bool seen as zero")

	test.Assert(t, IsAnyNilOrZero(0), "Untyped constant zero seen as non-zero")
	test.Assert(t, !IsAnyNilOrZero(1), "Untyped constant 1 seen as zero")
	test.Assert(t, IsAnyNilOrZero(int(0)), "int(0) seen as non-zero")
	test.Assert(t, !IsAnyNilOrZero(int(1)), "int(1) seen as zero")
	test.Assert(t, IsAnyNilOrZero(int8(0)), "int8(0) seen as non-zero")
	test.Assert(t, !IsAnyNilOrZero(int8(1)), "int8(1) seen as zero")
	test.Assert(t, IsAnyNilOrZero(int16(0)), "int16(0) seen as non-zero")
	test.Assert(t, !IsAnyNilOrZero(int16(1)), "int16(1) seen as zero")
	test.Assert(t, IsAnyNilOrZero(int32(0)), "int32(0) seen as non-zero")
	test.Assert(t, !IsAnyNilOrZero(int32(1)), "int32(1) seen as zero")
	test.Assert(t, IsAnyNilOrZero(int64(0)), "int64(0) seen as non-zero")
	test.Assert(t, !IsAnyNilOrZero(int64(1)), "int64(1) seen as zero")

	test.Assert(t, IsAnyNilOrZero(uint(0)), "uint(0) seen as non-zero")
	test.Assert(t, !IsAnyNilOrZero(uint(1)), "uint(1) seen as zero")
	test.Assert(t, IsAnyNilOrZero(uint8(0)), "uint8(0) seen as non-zero")
	test.Assert(t, !IsAnyNilOrZero(uint8(1)), "uint8(1) seen as zero")
	test.Assert(t, IsAnyNilOrZero(uint16(0)), "uint16(0) seen as non-zero")
	test.Assert(t, !IsAnyNilOrZero(uint16(1)), "uint16(1) seen as zero")
	test.Assert(t, IsAnyNilOrZero(uint32(0)), "uint32(0) seen as non-zero")
	test.Assert(t, !IsAnyNilOrZero(uint32(1)), "uint32(1) seen as zero")
	test.Assert(t, IsAnyNilOrZero(uint64(0)), "uint64(0) seen as non-zero")
	test.Assert(t, !IsAnyNilOrZero(uint64(1)), "uint64(1) seen as zero")

	test.Assert(t, !IsAnyNilOrZero(-12.345), "Untyped float32 seen as zero")
	test.Assert(t, !IsAnyNilOrZero(float32(6.66)), "Non-empty float32 seen as zero")
	test.Assert(t, IsAnyNilOrZero(float32(0)), "Empty float32 seen as non-zero")

	test.Assert(t, !IsAnyNilOrZero(float64(7.77)), "Non-empty float64 seen as zero")
	test.Assert(t, IsAnyNilOrZero(float64(0)), "Empty float64 seen as non-zero")

	test.Assert(t, IsAnyNilOrZero(""), "Empty string seen as non-zero")
	test.Assert(t, !IsAnyNilOrZero("string"), "Non-empty string seen as zero")

	test.Assert(t, IsAnyNilOrZero([]string{}), "Empty string slice seen as non-zero")
	test.Assert(t, !IsAnyNilOrZero([]string{"barncats"}), "Non-empty string slice seen as zero")

	test.Assert(t, IsAnyNilOrZero([]byte{}), "Empty byte slice seen as non-zero")
	test.Assert(t, !IsAnyNilOrZero([]byte("byte")), "Non-empty byte slice seen as zero")

	test.Assert(t, IsAnyNilOrZero(time.Time{}), "No specified time value seen as non-zero")
	test.Assert(t, !IsAnyNilOrZero(time.Now()), "Current time seen as zero")

	type Foo struct {
		foo int
	}
	test.Assert(t, IsAnyNilOrZero(Foo{}), "Empty struct seen as non-zero")
	test.Assert(t, !IsAnyNilOrZero(Foo{5}), "Non-empty struct seen as zero")
	var f *Foo
	test.Assert(t, IsAnyNilOrZero(f), "Pointer to uninitialized struct seen as non-zero")

	test.Assert(t, IsAnyNilOrZero(1, ""), "Mixed values seen as non-zero")
	test.Assert(t, IsAnyNilOrZero("", 1), "Mixed values seen as non-zero")

	var p *timestamppb.Timestamp
	test.Assert(t, IsAnyNilOrZero(p), "Pointer to uninitialized timestamppb.Timestamp seen as non-zero")
	test.Assert(t, IsAnyNilOrZero(timestamppb.New(time.Time{})), "*timestamppb.Timestamp containing an uninitialized inner time.Time{} is seen as non-zero")
	test.Assert(t, !IsAnyNilOrZero(timestamppb.Now()), "A *timestamppb.Timestamp with valid inner time is seen as zero")

	var d *durationpb.Duration
	var zeroDuration time.Duration
	test.Assert(t, IsAnyNilOrZero(d), "Pointer to uninitialized durationpb.Duration seen as non-zero")
	test.Assert(t, IsAnyNilOrZero(durationpb.New(zeroDuration)), "*durationpb.Duration containing an zero value time.Duration is seen as non-zero")
	test.Assert(t, !IsAnyNilOrZero(durationpb.New(666)), "A *durationpb.Duration with valid inner duration is seen as zero")
}

func BenchmarkIsAnyNilOrZero(b *testing.B) {
	var thyme *time.Time
	var sage *time.Duration
	var table = []struct {
		input interface{}
	}{
		{input: int(0)},
		{input: int(1)},
		{input: int8(0)},
		{input: int8(1)},
		{input: int16(0)},
		{input: int16(1)},
		{input: int32(0)},
		{input: int32(1)},
		{input: int64(0)},
		{input: int64(1)},
		{input: uint(0)},
		{input: uint(1)},
		{input: uint8(0)},
		{input: uint8(1)},
		{input: uint16(0)},
		{input: uint16(1)},
		{input: uint32(0)},
		{input: uint32(1)},
		{input: uint64(0)},
		{input: uint64(1)},
		{input: float32(0)},
		{input: float32(0.1)},
		{input: float64(0)},
		{input: float64(0.1)},
		{input: ""},
		{input: "ahoyhoy"},
		{input: []string{}},
		{input: []string{""}},
		{input: []string{"oodley_doodley"}},
		{input: []byte{}},
		{input: []byte{0}},
		{input: []byte{1}},
		{input: []rune{}},
		{input: []rune{2}},
		{input: []rune{3}},
		{input: nil},
		{input: false},
		{input: true},
		{input: thyme},
		{input: time.Time{}},
		{input: time.Date(2015, time.June, 04, 11, 04, 38, 0, time.UTC)},
		{input: sage},
		{input: time.Duration(1)},
		{input: time.Duration(0)},
	}

	for _, v := range table {
		b.Run(fmt.Sprintf("input_%T_%v", v.input, v.input), func(b *testing.B) {
			for range b.N {
				_ = IsAnyNilOrZero(v.input)
			}
		})
	}
}

func TestUniqueLowerNames(t *testing.T) {
	u := UniqueLowerNames([]string{"foobar.com", "fooBAR.com", "baz.com", "foobar.com", "bar.com", "bar.com", "a.com"})
	sort.Strings(u)
	test.AssertDeepEquals(t, []string{"a.com", "bar.com", "baz.com", "foobar.com"}, u)
}

func TestValidSerial(t *testing.T) {
	notLength32Or36 := "A"
	length32 := strings.Repeat("A", 32)
	length36 := strings.Repeat("A", 36)
	isValidSerial := ValidSerial(notLength32Or36)
	test.AssertEquals(t, isValidSerial, false)
	isValidSerial = ValidSerial(length32)
	test.AssertEquals(t, isValidSerial, true)
	isValidSerial = ValidSerial(length36)
	test.AssertEquals(t, isValidSerial, true)
}

func TestLoadCert(t *testing.T) {
	var osPathErr *os.PathError
	_, err := LoadCert("")
	test.AssertError(t, err, "Loading empty path did not error")
	test.AssertErrorWraps(t, err, &osPathErr)

	_, err = LoadCert("totally/fake/path")
	test.AssertError(t, err, "Loading nonexistent path did not error")
	test.AssertErrorWraps(t, err, &osPathErr)

	_, err = LoadCert("../test/hierarchy/README.md")
	test.AssertError(t, err, "Loading non-PEM file did not error")
	test.AssertContains(t, err.Error(), "no data in cert PEM file")

	_, err = LoadCert("../test/hierarchy/int-e1.key.pem")
	test.AssertError(t, err, "Loading non-cert PEM file did not error")
	test.AssertContains(t, err.Error(), "x509: malformed tbs certificate")

	cert, err := LoadCert("../test/hierarchy/int-r3.cert.pem")
	test.AssertNotError(t, err, "Failed to load cert PEM file")
	test.AssertEquals(t, cert.Subject.CommonName, "(TEST) Radical Rhino R3")
}

func TestRetryBackoff(t *testing.T) {
	assertBetween := func(a, b, c float64) {
		t.Helper()
		if a < b || a > c {
			t.Fatalf("%f is not between %f and %f", a, b, c)
		}
	}

	factor := 1.5
	base := time.Minute
	max := 10 * time.Minute

	backoff := RetryBackoff(0, base, max, factor)
	assertBetween(float64(backoff), 0, 0)

	expected := base
	backoff = RetryBackoff(1, base, max, factor)
	assertBetween(float64(backoff), float64(expected)*0.8, float64(expected)*1.2)

	expected = time.Second * 90
	backoff = RetryBackoff(2, base, max, factor)
	assertBetween(float64(backoff), float64(expected)*0.8, float64(expected)*1.2)

	expected = time.Minute * 10
	// should be truncated
	backoff = RetryBackoff(7, base, max, factor)
	assertBetween(float64(backoff), float64(expected)*0.8, float64(expected)*1.2)

}

func TestHashIdentifiers(t *testing.T) {
	dns1 := identifier.NewDNS("example.com")
	dns1_caps := identifier.NewDNS("eXaMpLe.COM")
	dns2 := identifier.NewDNS("high-energy-cheese-lab.nrc-cnrc.gc.ca")
	dns2_caps := identifier.NewDNS("HIGH-ENERGY-CHEESE-LAB.NRC-CNRC.GC.CA")
	ipv4_1 := identifier.NewIP(netip.MustParseAddr("10.10.10.10"))
	ipv4_2 := identifier.NewIP(netip.MustParseAddr("172.16.16.16"))
	ipv6_1 := identifier.NewIP(netip.MustParseAddr("2001:0db8:0bad:0dab:c0ff:fee0:0007:1337"))
	ipv6_2 := identifier.NewIP(netip.MustParseAddr("3fff::"))

	testCases := []struct {
		Name          string
		Idents1       identifier.ACMEIdentifiers
		Idents2       identifier.ACMEIdentifiers
		ExpectedEqual bool
	}{
		{
			Name:          "Deterministic for DNS",
			Idents1:       identifier.ACMEIdentifiers{dns1},
			Idents2:       identifier.ACMEIdentifiers{dns1},
			ExpectedEqual: true,
		},
		{
			Name:          "Deterministic for IPv4",
			Idents1:       identifier.ACMEIdentifiers{ipv4_1},
			Idents2:       identifier.ACMEIdentifiers{ipv4_1},
			ExpectedEqual: true,
		},
		{
			Name:          "Deterministic for IPv6",
			Idents1:       identifier.ACMEIdentifiers{ipv6_1},
			Idents2:       identifier.ACMEIdentifiers{ipv6_1},
			ExpectedEqual: true,
		},
		{
			Name:          "Differentiates for DNS",
			Idents1:       identifier.ACMEIdentifiers{dns1},
			Idents2:       identifier.ACMEIdentifiers{dns2},
			ExpectedEqual: false,
		},
		{
			Name:          "Differentiates for IPv4",
			Idents1:       identifier.ACMEIdentifiers{ipv4_1},
			Idents2:       identifier.ACMEIdentifiers{ipv4_2},
			ExpectedEqual: false,
		},
		{
			Name:          "Differentiates for IPv6",
			Idents1:       identifier.ACMEIdentifiers{ipv6_1},
			Idents2:       identifier.ACMEIdentifiers{ipv6_2},
			ExpectedEqual: false,
		},
		{
			Name: "Not subject to ordering",
			Idents1: identifier.ACMEIdentifiers{
				dns1, dns2, ipv4_1, ipv4_2, ipv6_1, ipv6_2,
			},
			Idents2: identifier.ACMEIdentifiers{
				ipv6_1, dns2, ipv4_2, dns1, ipv4_1, ipv6_2,
			},
			ExpectedEqual: true,
		},
		{
			Name: "Not case sensitive",
			Idents1: identifier.ACMEIdentifiers{
				dns1, dns2,
			},
			Idents2: identifier.ACMEIdentifiers{
				dns1_caps, dns2_caps,
			},
			ExpectedEqual: true,
		},
		{
			Name: "Not subject to duplication",
			Idents1: identifier.ACMEIdentifiers{
				dns1, dns1,
			},
			Idents2:       identifier.ACMEIdentifiers{dns1},
			ExpectedEqual: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			h1 := HashIdentifiers(tc.Idents1)
			h2 := HashIdentifiers(tc.Idents2)
			if slices.Equal(h1, h2) != tc.ExpectedEqual {
				t.Errorf("Comparing hashes of idents %#v and %#v, expected equality to be %v", tc.Idents1, tc.Idents2, tc.ExpectedEqual)
			}
		})
	}
}

func TestIsCanceled(t *testing.T) {
	if !IsCanceled(context.Canceled) {
		t.Errorf("Expected context.Canceled to be canceled, but wasn't.")
	}
	if !IsCanceled(status.Errorf(codes.Canceled, "hi")) {
		t.Errorf("Expected gRPC cancellation to be canceled, but wasn't.")
	}
	if IsCanceled(errors.New("hi")) {
		t.Errorf("Expected random error to not be canceled, but was.")
	}
}
