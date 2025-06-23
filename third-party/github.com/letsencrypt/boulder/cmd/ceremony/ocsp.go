package main

import (
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/ocsp"
)

func generateOCSPResponse(signer crypto.Signer, issuer, delegatedIssuer, cert *x509.Certificate, thisUpdate, nextUpdate time.Time, status int) ([]byte, error) {
	err := cert.CheckSignatureFrom(issuer)
	if err != nil {
		return nil, fmt.Errorf("invalid signature on certificate from issuer: %s", err)
	}

	signingCert := issuer
	if delegatedIssuer != nil {
		signingCert = delegatedIssuer
		err := delegatedIssuer.CheckSignatureFrom(issuer)
		if err != nil {
			return nil, fmt.Errorf("invalid signature on delegated issuer from issuer: %s", err)
		}

		gotOCSPEKU := false
		for _, eku := range delegatedIssuer.ExtKeyUsage {
			if eku == x509.ExtKeyUsageOCSPSigning {
				gotOCSPEKU = true
				break
			}
		}
		if !gotOCSPEKU {
			return nil, errors.New("delegated issuer certificate doesn't contain OCSPSigning extended key usage")
		}
	}

	if nextUpdate.Before(thisUpdate) {
		return nil, errors.New("thisUpdate must be before nextUpdate")
	}
	if thisUpdate.Before(signingCert.NotBefore) {
		return nil, errors.New("thisUpdate is before signing certificate's notBefore")
	} else if nextUpdate.After(signingCert.NotAfter) {
		return nil, errors.New("nextUpdate is after signing certificate's notAfter")
	}

	template := ocsp.Response{
		SerialNumber: cert.SerialNumber,
		ThisUpdate:   thisUpdate,
		NextUpdate:   nextUpdate,
		Status:       status,
	}
	if delegatedIssuer != nil {
		template.Certificate = delegatedIssuer
	}

	resp, err := ocsp.CreateResponse(issuer, signingCert, template, signer)
	if err != nil {
		return nil, fmt.Errorf("failed to create response: %s", err)
	}

	encodedResp := make([]byte, base64.StdEncoding.EncodedLen(len(resp))+1)
	base64.StdEncoding.Encode(encodedResp, resp)
	encodedResp[len(encodedResp)-1] = '\n'

	return encodedResp, nil
}
