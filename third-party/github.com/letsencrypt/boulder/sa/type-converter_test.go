package sa

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/identifier"
	"github.com/letsencrypt/boulder/test"

	"github.com/go-jose/go-jose/v4"
)

const JWK1JSON = `{
  "kty": "RSA",
  "n": "vuc785P8lBj3fUxyZchF_uZw6WtbxcorqgTyq-qapF5lrO1U82Tp93rpXlmctj6fyFHBVVB5aXnUHJ7LZeVPod7Wnfl8p5OyhlHQHC8BnzdzCqCMKmWZNX5DtETDId0qzU7dPzh0LP0idt5buU7L9QNaabChw3nnaL47iu_1Di5Wp264p2TwACeedv2hfRDjDlJmaQXuS8Rtv9GnRWyC9JBu7XmGvGDziumnJH7Hyzh3VNu-kSPQD3vuAFgMZS6uUzOztCkT0fpOalZI6hqxtWLvXUMj-crXrn-Maavz8qRhpAyp5kcYk3jiHGgQIi7QSK2JIdRJ8APyX9HlmTN5AQ",
  "e": "AQAB"
}`

func TestAcmeIdentifier(t *testing.T) {
	tc := BoulderTypeConverter{}

	ai := identifier.ACMEIdentifier{Type: "data1", Value: "data2"}
	out := identifier.ACMEIdentifier{}

	marshaledI, err := tc.ToDb(ai)
	test.AssertNotError(t, err, "Could not ToDb")

	scanner, ok := tc.FromDb(&out)
	test.Assert(t, ok, "FromDb failed")
	if !ok {
		t.FailNow()
		return
	}

	marshaled := marshaledI.(string)
	err = scanner.Binder(&marshaled, &out)
	test.AssertNotError(t, err, "failed to scanner.Binder")
	test.AssertMarshaledEquals(t, ai, out)
}

func TestAcmeIdentifierBadJSON(t *testing.T) {
	badJSON := `{`
	tc := BoulderTypeConverter{}
	out := identifier.ACMEIdentifier{}
	scanner, _ := tc.FromDb(&out)
	err := scanner.Binder(&badJSON, &out)
	test.AssertError(t, err, "expected error from scanner.Binder")
	var badJSONErr errBadJSON
	test.AssertErrorWraps(t, err, &badJSONErr)
	test.AssertEquals(t, string(badJSONErr.json), badJSON)
}

func TestJSONWebKey(t *testing.T) {
	tc := BoulderTypeConverter{}

	var jwk, out jose.JSONWebKey
	err := json.Unmarshal([]byte(JWK1JSON), &jwk)
	if err != nil {
		t.Fatal(err)
	}

	marshaledI, err := tc.ToDb(jwk)
	test.AssertNotError(t, err, "Could not ToDb")

	scanner, ok := tc.FromDb(&out)
	test.Assert(t, ok, "FromDb failed")
	if !ok {
		t.FailNow()
		return
	}

	marshaled := marshaledI.(string)
	err = scanner.Binder(&marshaled, &out)
	test.AssertNotError(t, err, "failed to scanner.Binder")
	test.AssertMarshaledEquals(t, jwk, out)
}

func TestJSONWebKeyBadJSON(t *testing.T) {
	badJSON := `{`
	tc := BoulderTypeConverter{}
	out := jose.JSONWebKey{}
	scanner, _ := tc.FromDb(&out)
	err := scanner.Binder(&badJSON, &out)
	test.AssertError(t, err, "expected error from scanner.Binder")
	var badJSONErr errBadJSON
	test.AssertErrorWraps(t, err, &badJSONErr)
	test.AssertEquals(t, string(badJSONErr.json), badJSON)
}

func TestAcmeStatus(t *testing.T) {
	tc := BoulderTypeConverter{}

	var as, out core.AcmeStatus
	as = "core.AcmeStatus"

	marshaledI, err := tc.ToDb(as)
	test.AssertNotError(t, err, "Could not ToDb")

	scanner, ok := tc.FromDb(&out)
	test.Assert(t, ok, "FromDb failed")
	if !ok {
		t.FailNow()
		return
	}

	marshaled := marshaledI.(string)
	err = scanner.Binder(&marshaled, &out)
	test.AssertNotError(t, err, "failed to scanner.Binder")
	test.AssertMarshaledEquals(t, as, out)
}

func TestOCSPStatus(t *testing.T) {
	tc := BoulderTypeConverter{}

	var os, out core.OCSPStatus
	os = "core.OCSPStatus"

	marshaledI, err := tc.ToDb(os)
	test.AssertNotError(t, err, "Could not ToDb")

	scanner, ok := tc.FromDb(&out)
	test.Assert(t, ok, "FromDb failed")
	if !ok {
		t.FailNow()
		return
	}

	marshaled := marshaledI.(string)
	err = scanner.Binder(&marshaled, &out)
	test.AssertNotError(t, err, "failed to scanner.Binder")
	test.AssertMarshaledEquals(t, os, out)
}

func TestStringSlice(t *testing.T) {
	tc := BoulderTypeConverter{}
	var au, out []string

	marshaledI, err := tc.ToDb(au)
	test.AssertNotError(t, err, "Could not ToDb")

	scanner, ok := tc.FromDb(&out)
	test.Assert(t, ok, "FromDb failed")
	if !ok {
		t.FailNow()
		return
	}

	marshaled := marshaledI.(string)
	err = scanner.Binder(&marshaled, &out)
	test.AssertNotError(t, err, "failed to scanner.Binder")
	test.AssertMarshaledEquals(t, au, out)
}

func TestTimeTruncate(t *testing.T) {
	tc := BoulderTypeConverter{}
	preciseTime := time.Date(2024, 06, 20, 00, 00, 00, 999999999, time.UTC)
	dbTime, err := tc.ToDb(preciseTime)
	test.AssertNotError(t, err, "Could not ToDb")
	dbTimeT, ok := dbTime.(time.Time)
	test.Assert(t, ok, "Could not convert dbTime to time.Time")
	test.Assert(t, dbTimeT.Nanosecond() == 0, "Nanosecond not truncated")

	dbTimePtr, err := tc.ToDb(&preciseTime)
	test.AssertNotError(t, err, "Could not ToDb")
	dbTimePtrT, ok := dbTimePtr.(*time.Time)
	test.Assert(t, ok, "Could not convert dbTimePtr to *time.Time")
	test.Assert(t, dbTimePtrT.Nanosecond() == 0, "Nanosecond not truncated")

	var dbTimePtrNil *time.Time
	shouldBeNil, err := tc.ToDb(dbTimePtrNil)
	test.AssertNotError(t, err, "Could not ToDb")
	if shouldBeNil != nil {
		t.Errorf("Expected nil, got %v", shouldBeNil)
	}
}
