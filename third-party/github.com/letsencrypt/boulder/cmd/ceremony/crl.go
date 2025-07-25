package main

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/letsencrypt/boulder/crl/idp"
	"github.com/letsencrypt/boulder/linter"
)

func generateCRL(signer crypto.Signer, issuer *x509.Certificate, thisUpdate, nextUpdate time.Time, number int64, revokedCertificates []x509.RevocationListEntry) ([]byte, error) {
	template := &x509.RevocationList{
		RevokedCertificateEntries: revokedCertificates,
		Number:                    big.NewInt(number),
		ThisUpdate:                thisUpdate,
		NextUpdate:                nextUpdate,
	}

	if nextUpdate.Before(thisUpdate) {
		return nil, errors.New("thisUpdate must be before nextUpdate")
	}
	if thisUpdate.Before(issuer.NotBefore) {
		return nil, errors.New("thisUpdate is before issuing certificate's notBefore")
	} else if nextUpdate.After(issuer.NotAfter) {
		return nil, errors.New("nextUpdate is after issuing certificate's notAfter")
	}

	// Verify that the CRL is not valid for more than 12 months as specified in
	// CABF BRs Section 4.9.7
	if nextUpdate.Sub(thisUpdate) > time.Hour*24*365 {
		return nil, errors.New("nextUpdate must be less than 12 months after thisUpdate")
	}
	// Add the Issuing Distribution Point extension.
	idp, err := idp.MakeCACertsExt()
	if err != nil {
		return nil, fmt.Errorf("creating IDP extension: %w", err)
	}
	template.ExtraExtensions = append(template.ExtraExtensions, *idp)

	err = linter.CheckCRL(template, issuer, signer, []string{})
	if err != nil {
		return nil, fmt.Errorf("crl failed pre-issuance lint: %w", err)
	}

	// x509.CreateRevocationList uses an io.Reader here for signing methods that require
	// a source of randomness. Since PKCS#11 based signing generates needed randomness
	// at the HSM we don't need to pass a real reader. Instead of passing a nil reader
	// we use one that always returns errors in case the internal usage of this reader
	// changes.
	crlBytes, err := x509.CreateRevocationList(&failReader{}, template, issuer, signer)
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: crlBytes}), nil
}
