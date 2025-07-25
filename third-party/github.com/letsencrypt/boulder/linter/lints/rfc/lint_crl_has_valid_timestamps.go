package rfc

import (
	"errors"
	"fmt"
	"time"

	"github.com/zmap/zcrypto/x509"
	"github.com/zmap/zlint/v3/lint"
	"github.com/zmap/zlint/v3/util"
	"golang.org/x/crypto/cryptobyte"
	cryptobyte_asn1 "golang.org/x/crypto/cryptobyte/asn1"
)

const (
	utcTimeFormat         = "YYMMDDHHMMSSZ"
	generalizedTimeFormat = "YYYYMMDDHHMMSSZ"
)

type crlHasValidTimestamps struct{}

/************************************************
RFC 5280: 5.1.2.4
CRL issuers conforming to this profile MUST encode thisUpdate as UTCTime for
dates through the year 2049. CRL issuers conforming to this profile MUST encode
thisUpdate as GeneralizedTime for dates in the year 2050 or later. Conforming
applications MUST be able to process dates that are encoded in either UTCTime or
GeneralizedTime.

Where encoded as UTCTime, thisUpdate MUST be specified and interpreted as
defined in Section 4.1.2.5.1. Where encoded as GeneralizedTime, thisUpdate MUST
be specified and interpreted as defined in Section 4.1.2.5.2.

RFC 5280: 5.1.2.5
CRL issuers conforming to this profile MUST encode nextUpdate as UTCTime for
dates through the year 2049. CRL issuers conforming to this profile MUST encode
nextUpdate as GeneralizedTime for dates in the year 2050 or later. Conforming
applications MUST be able to process dates that are encoded in either UTCTime or
GeneralizedTime.

Where encoded as UTCTime, nextUpdate MUST be specified and interpreted as
defined in Section 4.1.2.5.1. Where encoded as GeneralizedTime, nextUpdate MUST
be specified and interpreted as defined in Section 4.1.2.5.2.

RFC 5280: 5.1.2.6
The time for revocationDate MUST be expressed as described in Section 5.1.2.4.

RFC 5280: 4.1.2.5.1
UTCTime values MUST be expressed in Greenwich Mean Time (Zulu) and MUST include
seconds (i.e., times are YYMMDDHHMMSSZ), even where the number of seconds is
zero.

RFC 5280: 4.1.2.5.2
GeneralizedTime values MUST be expressed in Greenwich Mean Time (Zulu) and MUST
include seconds (i.e., times are YYYYMMDDHHMMSSZ), even where the number of
seconds is zero. GeneralizedTime values MUST NOT include fractional seconds.
************************************************/

func init() {
	lint.RegisterRevocationListLint(&lint.RevocationListLint{
		LintMetadata: lint.LintMetadata{
			Name:          "e_crl_has_valid_timestamps",
			Description:   "CRL thisUpdate, nextUpdate, and revocationDates must be properly encoded",
			Citation:      "RFC 5280: 5.1.2.4, 5.1.2.5, and 5.1.2.6",
			Source:        lint.RFC5280,
			EffectiveDate: util.RFC5280Date,
		},
		Lint: NewCrlHasValidTimestamps,
	})
}

func NewCrlHasValidTimestamps() lint.RevocationListLintInterface {
	return &crlHasValidTimestamps{}
}

func (l *crlHasValidTimestamps) CheckApplies(c *x509.RevocationList) bool {
	return true
}

func (l *crlHasValidTimestamps) Execute(c *x509.RevocationList) *lint.LintResult {
	input := cryptobyte.String(c.RawTBSRevocationList)
	lintFail := lint.LintResult{
		Status:  lint.Error,
		Details: "Failed to re-parse tbsCertList during linting",
	}

	// Read tbsCertList.
	var tbs cryptobyte.String
	if !input.ReadASN1(&tbs, cryptobyte_asn1.SEQUENCE) {
		return &lintFail
	}

	// Skip (optional) version.
	if !tbs.SkipOptionalASN1(cryptobyte_asn1.INTEGER) {
		return &lintFail
	}

	// Skip signature.
	if !tbs.SkipASN1(cryptobyte_asn1.SEQUENCE) {
		return &lintFail
	}

	// Skip issuer.
	if !tbs.SkipASN1(cryptobyte_asn1.SEQUENCE) {
		return &lintFail
	}

	// Read thisUpdate.
	var thisUpdate cryptobyte.String
	var thisUpdateTag cryptobyte_asn1.Tag
	if !tbs.ReadAnyASN1Element(&thisUpdate, &thisUpdateTag) {
		return &lintFail
	}

	// Lint thisUpdate.
	err := lintTimestamp(&thisUpdate, thisUpdateTag)
	if err != nil {
		return &lint.LintResult{Status: lint.Error, Details: err.Error()}
	}

	// Peek (optional) nextUpdate.
	if tbs.PeekASN1Tag(cryptobyte_asn1.UTCTime) || tbs.PeekASN1Tag(cryptobyte_asn1.GeneralizedTime) {
		// Read nextUpdate.
		var nextUpdate cryptobyte.String
		var nextUpdateTag cryptobyte_asn1.Tag
		if !tbs.ReadAnyASN1Element(&nextUpdate, &nextUpdateTag) {
			return &lintFail
		}

		// Lint nextUpdate.
		err = lintTimestamp(&nextUpdate, nextUpdateTag)
		if err != nil {
			return &lint.LintResult{Status: lint.Error, Details: err.Error()}
		}
	}

	// Peek (optional) revokedCertificates.
	if tbs.PeekASN1Tag(cryptobyte_asn1.SEQUENCE) {
		// Read sequence of revokedCertificate.
		var revokedSeq cryptobyte.String
		if !tbs.ReadASN1(&revokedSeq, cryptobyte_asn1.SEQUENCE) {
			return &lintFail
		}

		// Iterate over each revokedCertificate sequence.
		for !revokedSeq.Empty() {
			// Read revokedCertificate.
			var certSeq cryptobyte.String
			if !revokedSeq.ReadASN1Element(&certSeq, cryptobyte_asn1.SEQUENCE) {
				return &lintFail
			}

			if !certSeq.ReadASN1(&certSeq, cryptobyte_asn1.SEQUENCE) {
				return &lintFail
			}

			// Skip userCertificate (serial number).
			if !certSeq.SkipASN1(cryptobyte_asn1.INTEGER) {
				return &lintFail
			}

			// Read revocationDate.
			var revocationDate cryptobyte.String
			var revocationDateTag cryptobyte_asn1.Tag
			if !certSeq.ReadAnyASN1Element(&revocationDate, &revocationDateTag) {
				return &lintFail
			}

			// Lint revocationDate.
			err = lintTimestamp(&revocationDate, revocationDateTag)
			if err != nil {
				return &lint.LintResult{Status: lint.Error, Details: err.Error()}
			}
		}
	}
	return &lint.LintResult{Status: lint.Pass}
}

func lintTimestamp(der *cryptobyte.String, tag cryptobyte_asn1.Tag) error {
	// Preserve the original timestamp for length checking.
	derBytes := *der
	var tsBytes cryptobyte.String
	if !derBytes.ReadASN1(&tsBytes, tag) {
		return errors.New("failed to read timestamp")
	}
	tsLen := len(string(tsBytes))

	var parsedTime time.Time
	switch tag {
	case cryptobyte_asn1.UTCTime:
		// Verify that the timestamp is properly formatted.
		if tsLen != len(utcTimeFormat) {
			return fmt.Errorf("timestamps encoded using UTCTime MUST be specified in the format %q", utcTimeFormat)
		}

		if !der.ReadASN1UTCTime(&parsedTime) {
			return errors.New("failed to read timestamp encoded using UTCTime")
		}

		// Verify that the timestamp is prior to the year 2050. This should
		// really never happen.
		if parsedTime.Year() > 2049 {
			return errors.New("ReadASN1UTCTime returned a UTCTime after 2049")
		}
	case cryptobyte_asn1.GeneralizedTime:
		// Verify that the timestamp is properly formatted.
		if tsLen != len(generalizedTimeFormat) {
			return fmt.Errorf(
				"timestamps encoded using GeneralizedTime MUST be specified in the format %q", generalizedTimeFormat,
			)
		}

		if !der.ReadASN1GeneralizedTime(&parsedTime) {
			return fmt.Errorf("failed to read timestamp encoded using GeneralizedTime")
		}

		// Verify that the timestamp occurred after the year 2049.
		if parsedTime.Year() < 2050 {
			return errors.New("timestamps prior to 2050 MUST be encoded using UTCTime")
		}
	default:
		return errors.New("unsupported time format")
	}

	// Verify that the location is UTC.
	if parsedTime.Location() != time.UTC {
		return errors.New("time must be in UTC")
	}
	return nil
}
