package lints

import (
	"bytes"
	"net/url"
	"time"

	"github.com/zmap/zcrypto/encoding/asn1"
	"github.com/zmap/zcrypto/x509/pkix"
	"github.com/zmap/zlint/v3/lint"
	"golang.org/x/crypto/cryptobyte"
	cryptobyte_asn1 "golang.org/x/crypto/cryptobyte/asn1"
)

const (
	// CABF Baseline Requirements 6.3.2 Certificate operational periods:
	// For the purpose of calculations, a day is measured as 86,400 seconds.
	// Any amount of time greater than this, including fractional seconds and/or
	// leap seconds, shall represent an additional day.
	BRDay time.Duration = 86400 * time.Second

	// Declare our own Sources for use in zlint registry filtering.
	LetsEncryptCPS lint.LintSource = "LECPS"
	ChromeCTPolicy lint.LintSource = "ChromeCT"
)

var (
	CPSV33Date           = time.Date(2021, time.June, 8, 0, 0, 0, 0, time.UTC)
	MozillaPolicy281Date = time.Date(2023, time.February, 15, 0, 0, 0, 0, time.UTC)
)

// IssuingDistributionPoint stores the IA5STRING value(s) of the optional
// distributionPoint, and the (implied OPTIONAL) BOOLEAN values of
// onlyContainsUserCerts and onlyContainsCACerts.
//
//	RFC 5280
//	* Section 5.2.5
//	  IssuingDistributionPoint ::= SEQUENCE {
//	    distributionPoint          [0] DistributionPointName OPTIONAL,
//	    onlyContainsUserCerts      [1] BOOLEAN DEFAULT FALSE,
//	    onlyContainsCACerts        [2] BOOLEAN DEFAULT FALSE,
//	    ...
//	  }
//
//	* Section 4.2.1.13
//	  DistributionPointName ::= CHOICE {
//	    fullName                [0]     GeneralNames,
//	    ... }
//
//	* Appendix A.1, Page 128
//	  GeneralNames ::= SEQUENCE SIZE (1..MAX) OF GeneralName
//	  GeneralName ::= CHOICE {
//	    ...
//	        uniformResourceIdentifier [6]  IA5String,
//	    ... }
//
// Because this struct is used by cryptobyte (not by encoding/asn1), and because
// we only care about the uniformResourceIdentifier flavor of GeneralName, we
// are able to flatten the DistributionPointName down into a slice of URIs.
type IssuingDistributionPoint struct {
	DistributionPointURIs []*url.URL
	OnlyContainsUserCerts bool
	OnlyContainsCACerts   bool
}

// NewIssuingDistributionPoint is a constructor which returns an
// IssuingDistributionPoint with each field set to zero values.
func NewIssuingDistributionPoint() *IssuingDistributionPoint {
	return &IssuingDistributionPoint{}
}

// GetExtWithOID is a helper for several of our custom lints. It returns the
// extension with the given OID if it exists, or nil otherwise.
func GetExtWithOID(exts []pkix.Extension, oid asn1.ObjectIdentifier) *pkix.Extension {
	for _, ext := range exts {
		if ext.Id.Equal(oid) {
			return &ext
		}
	}
	return nil
}

// ReadOptionalASN1BooleanWithTag attempts to read and advance incoming to
// search for an optional DER-encoded ASN.1 element tagged with the given tag.
// Unless out is nil, it stores whether an element with the tag was found in
// out, otherwise out will take the default value. It reports whether all reads
// were successful.
func ReadOptionalASN1BooleanWithTag(incoming *cryptobyte.String, out *bool, tag cryptobyte_asn1.Tag, defaultValue bool) bool {
	// ReadOptionalASN1 performs a peek and will not advance if the tag is
	// missing, meaning that incoming will retain bytes.
	var valuePresent bool
	var valueBytes cryptobyte.String
	if !incoming.ReadOptionalASN1(&valueBytes, &valuePresent, tag) {
		return false
	}
	val := defaultValue
	if valuePresent {
		/*
			X.690 (07/2002)
			https://www.itu.int/rec/T-REC-X.690-200207-S/en

			Section 8.2.2:
				If the boolean value is:
				FALSE
				the octet shall be zero.
				If the boolean value is
				TRUE
				the octet shall have any non-zero value, as a sender's option.

			Section 11.1 Boolean values:
				If the encoding represents the boolean value TRUE, its single contents octet shall have all eight
				bits set to one. (Contrast with 8.2.2.)

			Succinctly, BER encoding states any nonzero value is TRUE. The DER
			encoding restricts the value 0xFF as TRUE and any other: 0x01,
			0x23, 0xFE, etc as invalid encoding.
		*/
		boolBytes := []byte(valueBytes)
		if bytes.Equal(boolBytes, []byte{0xFF}) {
			val = true
		} else if bytes.Equal(boolBytes, []byte{0x00}) {
			val = false
		} else {
			// Unrecognized DER encoding of boolean!
			return false
		}
	}
	if out != nil {
		*out = val
	}

	// All reads were successful.
	return true
}
