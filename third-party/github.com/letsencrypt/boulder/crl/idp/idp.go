package idp

import (
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
)

var idpOID = asn1.ObjectIdentifier{2, 5, 29, 28} // id-ce-issuingDistributionPoint

// issuingDistributionPoint represents the ASN.1 IssuingDistributionPoint
// SEQUENCE as defined in RFC 5280 Section 5.2.5. We only use three of the
// fields, so the others are omitted.
type issuingDistributionPoint struct {
	DistributionPoint     distributionPointName `asn1:"optional,tag:0"`
	OnlyContainsUserCerts bool                  `asn1:"optional,tag:1"`
	OnlyContainsCACerts   bool                  `asn1:"optional,tag:2"`
}

// distributionPointName represents the ASN.1 DistributionPointName CHOICE as
// defined in RFC 5280 Section 4.2.1.13. We only use one of the fields, so the
// others are omitted.
type distributionPointName struct {
	// Technically, FullName is of type GeneralNames, which is of type SEQUENCE OF
	// GeneralName. But GeneralName itself is of type CHOICE, and the asn1.Marshal
	// function doesn't support marshalling structs to CHOICEs, so we have to use
	// asn1.RawValue and encode the GeneralName ourselves.
	FullName []asn1.RawValue `asn1:"optional,tag:0"`
}

// MakeUserCertsExt returns a critical IssuingDistributionPoint extension
// containing the given URLs and with the OnlyContainsUserCerts boolean set to
// true.
func MakeUserCertsExt(urls []string) (pkix.Extension, error) {
	var gns []asn1.RawValue
	for _, url := range urls {
		gns = append(gns, asn1.RawValue{ // GeneralName
			Class: 2, // context-specific
			Tag:   6, // uniformResourceIdentifier, IA5String
			Bytes: []byte(url),
		})
	}

	val := issuingDistributionPoint{
		DistributionPoint:     distributionPointName{FullName: gns},
		OnlyContainsUserCerts: true,
	}

	valBytes, err := asn1.Marshal(val)
	if err != nil {
		return pkix.Extension{}, err
	}

	return pkix.Extension{
		Id:       idpOID,
		Value:    valBytes,
		Critical: true,
	}, nil
}

// MakeCACertsExt returns a critical IssuingDistributionPoint extension
// asserting the OnlyContainsCACerts boolean.
func MakeCACertsExt() (*pkix.Extension, error) {
	val := issuingDistributionPoint{
		OnlyContainsCACerts: true,
	}

	valBytes, err := asn1.Marshal(val)
	if err != nil {
		return nil, err
	}

	return &pkix.Extension{
		Id:       idpOID,
		Value:    valBytes,
		Critical: true,
	}, nil
}

// GetIDPURIs returns the URIs contained within the issuingDistributionPoint
// extension, if present, or an error otherwise.
func GetIDPURIs(exts []pkix.Extension) ([]string, error) {
	for _, ext := range exts {
		if ext.Id.Equal(idpOID) {
			val := issuingDistributionPoint{}
			rest, err := asn1.Unmarshal(ext.Value, &val)
			if err != nil {
				return nil, fmt.Errorf("parsing IssuingDistributionPoint extension: %w", err)
			}
			if len(rest) != 0 {
				return nil, fmt.Errorf("parsing IssuingDistributionPoint extension: got %d unexpected trailing bytes", len(rest))
			}
			var uris []string
			for _, generalName := range val.DistributionPoint.FullName {
				uris = append(uris, string(generalName.Bytes))
			}
			return uris, nil
		}
	}
	return nil, errors.New("no IssuingDistributionPoint extension found")
}
