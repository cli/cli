package precert

import (
	"bytes"
	encoding_asn1 "encoding/asn1"
	"errors"
	"fmt"

	"golang.org/x/crypto/cryptobyte"
	"golang.org/x/crypto/cryptobyte/asn1"
)

// Correspond returns nil if the two certificates are a valid precertificate/final certificate pair.
// Order of the arguments matters: the precertificate is first and the final certificate is second.
// Note that RFC 6962 allows the precertificate and final certificate to have different Issuers, but
// this function rejects such pairs.
func Correspond(precertDER, finalDER []byte) error {
	preTBS, err := tbsDERFromCertDER(precertDER)
	if err != nil {
		return fmt.Errorf("parsing precert: %w", err)
	}

	finalTBS, err := tbsDERFromCertDER(finalDER)
	if err != nil {
		return fmt.Errorf("parsing final cert: %w", err)
	}

	// The first 7 fields of TBSCertificate must be byte-for-byte identical.
	// The next 2 fields (issuerUniqueID and subjectUniqueID) are forbidden
	// by the Baseline Requirements so we assume they are not present (if they
	// are, they will fail the next check, for extensions).
	// https://datatracker.ietf.org/doc/html/rfc5280#page-117
	// TBSCertificate  ::=  SEQUENCE  {
	//      version         [0]  Version DEFAULT v1,
	//      serialNumber         CertificateSerialNumber,
	//      signature            AlgorithmIdentifier,
	//      issuer               Name,
	//      validity             Validity,
	//      subject              Name,
	//      subjectPublicKeyInfo SubjectPublicKeyInfo,
	//      issuerUniqueID  [1]  IMPLICIT UniqueIdentifier OPTIONAL,
	//      					 -- If present, version MUST be v2 or v3
	//      subjectUniqueID [2]  IMPLICIT UniqueIdentifier OPTIONAL,
	//      					 -- If present, version MUST be v2 or v3
	//      extensions      [3]  Extensions OPTIONAL
	//      					 -- If present, version MUST be v3 --  }
	for i := range 7 {
		if err := readIdenticalElement(&preTBS, &finalTBS); err != nil {
			return fmt.Errorf("checking for identical field %d: %w", i, err)
		}
	}

	// The extensions should be mostly the same, with these exceptions:
	//  - The precertificate should have exactly one precertificate poison extension
	//    not present in the final certificate.
	//  - The final certificate should have exactly one SCTList extension not present
	//    in the precertificate.
	//  - As a consequence, the byte lengths of the extensions fields will not be the
	//    same, so we ignore the lengths (so long as they parse)
	precertExtensionBytes, err := unwrapExtensions(preTBS)
	if err != nil {
		return fmt.Errorf("parsing precert extensions: %w", err)
	}

	finalCertExtensionBytes, err := unwrapExtensions(finalTBS)
	if err != nil {
		return fmt.Errorf("parsing final cert extensions: %w", err)
	}

	precertParser := extensionParser{bytes: precertExtensionBytes, skippableOID: poisonOID}
	finalCertParser := extensionParser{bytes: finalCertExtensionBytes, skippableOID: sctListOID}

	for i := 0; ; i++ {
		precertExtn, err := precertParser.Next()
		if err != nil {
			return err
		}

		finalCertExtn, err := finalCertParser.Next()
		if err != nil {
			return err
		}

		if !bytes.Equal(precertExtn, finalCertExtn) {
			return fmt.Errorf("precert extension %d (%x) not equal to final cert extension %d (%x)",
				i+precertParser.skipped, precertExtn, i+finalCertParser.skipped, finalCertExtn)
		}

		if precertExtn == nil && finalCertExtn == nil {
			break
		}
	}

	if precertParser.skipped == 0 {
		return fmt.Errorf("no poison extension found in precert")
	}
	if precertParser.skipped > 1 {
		return fmt.Errorf("multiple poison extensions found in precert")
	}
	if finalCertParser.skipped == 0 {
		return fmt.Errorf("no SCTList extension found in final cert")
	}
	if finalCertParser.skipped > 1 {
		return fmt.Errorf("multiple SCTList extensions found in final cert")
	}
	return nil
}

var poisonOID = []int{1, 3, 6, 1, 4, 1, 11129, 2, 4, 3}
var sctListOID = []int{1, 3, 6, 1, 4, 1, 11129, 2, 4, 2}

// extensionParser takes a sequence of bytes representing the inner bytes of the
// `extensions` field. Repeated calls to Next() will return all the extensions
// except those that match the skippableOID. The skipped extensions will be
// counted in `skipped`.
type extensionParser struct {
	skippableOID encoding_asn1.ObjectIdentifier
	bytes        cryptobyte.String
	skipped      int
}

// Next returns the next extension in the sequence, skipping (and counting)
// any extension that matches the skippableOID.
// Returns nil, nil when there are no more extensions.
func (e *extensionParser) Next() (cryptobyte.String, error) {
	if e.bytes.Empty() {
		return nil, nil
	}

	var next cryptobyte.String
	if !e.bytes.ReadASN1(&next, asn1.SEQUENCE) {
		return nil, fmt.Errorf("failed to parse extension")
	}

	var oid encoding_asn1.ObjectIdentifier
	nextCopy := next
	if !nextCopy.ReadASN1ObjectIdentifier(&oid) {
		return nil, fmt.Errorf("failed to parse extension OID")
	}

	if oid.Equal(e.skippableOID) {
		e.skipped++
		return e.Next()
	}

	return next, nil
}

// unwrapExtensions takes a given a sequence of bytes representing the `extensions` field
// of a TBSCertificate and parses away the outermost two layers, returning the inner bytes
// of the Extensions SEQUENCE.
//
// https://datatracker.ietf.org/doc/html/rfc5280#page-117
//
//	TBSCertificate  ::=  SEQUENCE  {
//	   ...
//	   extensions      [3]  Extensions OPTIONAL
//	}
//
// Extensions  ::=  SEQUENCE SIZE (1..MAX) OF Extension
func unwrapExtensions(field cryptobyte.String) (cryptobyte.String, error) {
	var extensions cryptobyte.String
	if !field.ReadASN1(&extensions, asn1.Tag(3).Constructed().ContextSpecific()) {
		return nil, errors.New("error reading extensions")
	}

	var extensionsInner cryptobyte.String
	if !extensions.ReadASN1(&extensionsInner, asn1.SEQUENCE) {
		return nil, errors.New("error reading extensions inner")
	}

	return extensionsInner, nil
}

// readIdenticalElement parses a single ASN1 element and returns an error if
// their tags are different or their contents are different.
func readIdenticalElement(a, b *cryptobyte.String) error {
	var aInner, bInner cryptobyte.String
	var aTag, bTag asn1.Tag
	if !a.ReadAnyASN1Element(&aInner, &aTag) {
		return fmt.Errorf("failed to read element from first input")
	}
	if !b.ReadAnyASN1Element(&bInner, &bTag) {
		return fmt.Errorf("failed to read element from first input")
	}
	if aTag != bTag {
		return fmt.Errorf("tags differ: %d != %d", aTag, bTag)
	}
	if !bytes.Equal([]byte(aInner), []byte(bInner)) {
		return fmt.Errorf("elements differ: %x != %x", aInner, bInner)
	}
	return nil
}

// tbsDERFromCertDER takes a Certificate object encoded as DER, and parses
// away the outermost two SEQUENCEs to get the inner bytes of the TBSCertificate.
//
// https://datatracker.ietf.org/doc/html/rfc5280#page-116
//
//		Certificate  ::=  SEQUENCE  {
//		    tbsCertificate       TBSCertificate,
//		    ...
//
//		TBSCertificate  ::=  SEQUENCE  {
//		    version         [0]  Version DEFAULT v1,
//		    serialNumber         CertificateSerialNumber,
//	     ...
func tbsDERFromCertDER(certDER []byte) (cryptobyte.String, error) {
	var inner cryptobyte.String
	input := cryptobyte.String(certDER)

	if !input.ReadASN1(&inner, asn1.SEQUENCE) {
		return nil, fmt.Errorf("failed to read outer sequence")
	}

	var tbsCertificate cryptobyte.String
	if !inner.ReadASN1(&tbsCertificate, asn1.SEQUENCE) {
		return nil, fmt.Errorf("failed to read tbsCertificate")
	}

	return tbsCertificate, nil
}
