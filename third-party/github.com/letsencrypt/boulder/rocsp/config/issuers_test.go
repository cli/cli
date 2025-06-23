package rocsp_config

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/letsencrypt/boulder/test"
	"golang.org/x/crypto/ocsp"
)

func TestLoadIssuers(t *testing.T) {
	input := map[string]int{
		"../../test/hierarchy/int-e1.cert.pem": 23,
		"../../test/hierarchy/int-r3.cert.pem": 99,
	}
	output, err := LoadIssuers(input)
	if err != nil {
		t.Fatal(err)
	}

	var e1 *ShortIDIssuer
	var r3 *ShortIDIssuer

	for i, v := range output {
		if strings.Contains(v.Certificate.Subject.String(), "E1") {
			e1 = &output[i]
		}
		if strings.Contains(v.Certificate.Subject.String(), "R3") {
			r3 = &output[i]
		}
	}

	test.AssertEquals(t, e1.Subject.String(), "CN=(TEST) Elegant Elephant E1,O=Boulder Test,C=XX")
	test.AssertEquals(t, r3.Subject.String(), "CN=(TEST) Radical Rhino R3,O=Boulder Test,C=XX")
	test.AssertEquals(t, e1.shortID, uint8(23))
	test.AssertEquals(t, r3.shortID, uint8(99))
}

func TestFindIssuerByName(t *testing.T) {
	input := map[string]int{
		"../../test/hierarchy/int-e1.cert.pem": 23,
		"../../test/hierarchy/int-r3.cert.pem": 99,
	}
	issuers, err := LoadIssuers(input)
	if err != nil {
		t.Fatal(err)
	}

	elephant, err := hex.DecodeString("3049310b300906035504061302585831153013060355040a130c426f756c6465722054657374312330210603550403131a28544553542920456c6567616e7420456c657068616e74204531")
	if err != nil {
		t.Fatal(err)
	}
	rhino, err := hex.DecodeString("3046310b300906035504061302585831153013060355040a130c426f756c64657220546573743120301e06035504031317285445535429205261646963616c205268696e6f205233")
	if err != nil {
		t.Fatal(err)
	}

	ocspResp := &ocsp.Response{
		RawResponderName: elephant,
	}

	issuer, err := FindIssuerByName(ocspResp, issuers)
	if err != nil {
		t.Fatalf("couldn't find issuer: %s", err)
	}

	test.AssertEquals(t, issuer.shortID, uint8(23))

	ocspResp = &ocsp.Response{
		RawResponderName: rhino,
	}

	issuer, err = FindIssuerByName(ocspResp, issuers)
	if err != nil {
		t.Fatalf("couldn't find issuer: %s", err)
	}

	test.AssertEquals(t, issuer.shortID, uint8(99))
}

func TestFindIssuerByID(t *testing.T) {
	input := map[string]int{
		"../../test/hierarchy/int-e1.cert.pem": 23,
		"../../test/hierarchy/int-r3.cert.pem": 99,
	}
	issuers, err := LoadIssuers(input)
	if err != nil {
		t.Fatal(err)
	}

	// an IssuerNameID
	issuer, err := FindIssuerByID(66283756913588288, issuers)
	if err != nil {
		t.Fatalf("couldn't find issuer: %s", err)
	}
	test.AssertEquals(t, issuer.shortID, uint8(23))

	// an IssuerNameID
	issuer, err = FindIssuerByID(58923463773186183, issuers)
	if err != nil {
		t.Fatalf("couldn't find issuer: %s", err)
	}
	test.AssertEquals(t, issuer.shortID, uint8(99))
}
