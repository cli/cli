package responder

import (
	"bytes"
	"context"
	"crypto"
	"crypto/sha1" //nolint: gosec // SHA1 is required by the RFC 5019 Lightweight OCSP Profile
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/ocsp"

	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/issuance"
	blog "github.com/letsencrypt/boulder/log"
)

// responderID contains the SHA1 hashes of an issuer certificate's name and key,
// exactly as the issuerNameHash and issuerKeyHash fields of an OCSP request
// should be computed by OCSP clients that are compliant with RFC 5019, the
// Lightweight OCSP Profile for High-Volume Environments. It also contains the
// Subject Common Name of the issuer certificate, for our own observability.
type responderID struct {
	nameHash   []byte
	keyHash    []byte
	commonName string
}

// computeLightweightResponderID builds a responderID from an issuer certificate.
func computeLightweightResponderID(ic *issuance.Certificate) (responderID, error) {
	// nameHash is the SHA1 hash over the DER encoding of the issuer certificate's
	// Subject Distinguished Name.
	nameHash := sha1.Sum(ic.RawSubject)

	// keyHash is the SHA1 hash over the DER encoding of the issuer certificate's
	// Subject Public Key Info. We can't use MarshalPKIXPublicKey for this since
	// it encodes keys using the SPKI structure itself, and we just want the
	// contents of the subjectPublicKey for the hash, so we need to extract it
	// ourselves.
	var spki struct {
		Algorithm pkix.AlgorithmIdentifier
		PublicKey asn1.BitString
	}
	_, err := asn1.Unmarshal(ic.RawSubjectPublicKeyInfo, &spki)
	if err != nil {
		return responderID{}, err
	}
	keyHash := sha1.Sum(spki.PublicKey.RightAlign())

	return responderID{nameHash[:], keyHash[:], ic.Subject.CommonName}, nil
}

type filterSource struct {
	wrapped        Source
	hashAlgorithm  crypto.Hash
	issuers        map[issuance.NameID]responderID
	serialPrefixes []string
	counter        *prometheus.CounterVec
	log            blog.Logger
	clk            clock.Clock
}

// NewFilterSource returns a filterSource which performs various checks on the
// OCSP requests sent to the wrapped Source, and the OCSP responses returned
// by it.
func NewFilterSource(issuerCerts []*issuance.Certificate, serialPrefixes []string, wrapped Source, stats prometheus.Registerer, log blog.Logger, clk clock.Clock) (*filterSource, error) {
	if len(issuerCerts) < 1 {
		return nil, errors.New("filter must include at least 1 issuer cert")
	}

	issuersByNameId := make(map[issuance.NameID]responderID)
	for _, issuerCert := range issuerCerts {
		rid, err := computeLightweightResponderID(issuerCert)
		if err != nil {
			return nil, fmt.Errorf("computing lightweight OCSP responder ID: %w", err)
		}
		issuersByNameId[issuerCert.NameID()] = rid
	}

	counter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ocsp_filter_responses",
		Help: "Count of OCSP requests/responses by action taken by the filter",
	}, []string{"result", "issuer"})
	stats.MustRegister(counter)

	return &filterSource{
		wrapped:        wrapped,
		hashAlgorithm:  crypto.SHA1,
		issuers:        issuersByNameId,
		serialPrefixes: serialPrefixes,
		counter:        counter,
		log:            log,
		clk:            clk,
	}, nil
}

// Response implements the Source interface. It checks the incoming request
// to ensure that we want to handle it, fetches the response from the wrapped
// Source, and checks that the response matches the request.
func (src *filterSource) Response(ctx context.Context, req *ocsp.Request) (*Response, error) {
	iss, err := src.checkRequest(req)
	if err != nil {
		src.log.Debugf("Not responding to filtered OCSP request: %s", err.Error())
		src.counter.WithLabelValues("request_filtered", "none").Inc()
		return nil, err
	}

	counter := src.counter.MustCurryWith(prometheus.Labels{"issuer": src.issuers[iss].commonName})

	resp, err := src.wrapped.Response(ctx, req)
	if err != nil {
		counter.WithLabelValues("wrapped_error").Inc()
		return nil, err
	}

	err = src.checkResponse(iss, resp)
	if err != nil {
		src.log.Warningf("OCSP Response not sent for CA=%s, Serial=%s, err: %s", hex.EncodeToString(req.IssuerKeyHash), core.SerialToString(req.SerialNumber), err)
		counter.WithLabelValues("response_filtered").Inc()
		return nil, err
	}

	counter.WithLabelValues("success").Inc()
	return resp, nil
}

// checkNextUpdate evaluates whether the nextUpdate field of the requested OCSP
// response is in the past. If so, `errOCSPResponseExpired` will be returned.
func (src *filterSource) checkNextUpdate(resp *Response) error {
	if src.clk.Now().Before(resp.NextUpdate) {
		return nil
	}
	return errOCSPResponseExpired
}

// checkRequest returns a descriptive error if the request does not satisfy any of
// the requirements of an OCSP request, or nil if the request should be handled.
// If the request passes all checks, then checkRequest returns the unique id of
// the issuer cert specified in the request.
func (src *filterSource) checkRequest(req *ocsp.Request) (issuance.NameID, error) {
	if req.HashAlgorithm != src.hashAlgorithm {
		return 0, fmt.Errorf("unsupported issuer key/name hash algorithm %s: %w", req.HashAlgorithm, ErrNotFound)
	}

	if len(src.serialPrefixes) > 0 {
		serialString := core.SerialToString(req.SerialNumber)
		match := false
		for _, prefix := range src.serialPrefixes {
			if strings.HasPrefix(serialString, prefix) {
				match = true
				break
			}
		}
		if !match {
			return 0, fmt.Errorf("unrecognized serial prefix: %w", ErrNotFound)
		}
	}

	for nameID, rid := range src.issuers {
		if bytes.Equal(req.IssuerNameHash, rid.nameHash) && bytes.Equal(req.IssuerKeyHash, rid.keyHash) {
			return nameID, nil
		}
	}
	return 0, fmt.Errorf("unrecognized issuer key hash %s: %w", hex.EncodeToString(req.IssuerKeyHash), ErrNotFound)
}

// checkResponse returns nil if the ocsp response was generated by the same
// issuer as was identified in the request, or an error otherwise. This filters
// out, for example, responses which are for a serial that we issued, but from a
// different issuer than that contained in the request.
func (src *filterSource) checkResponse(reqIssuerID issuance.NameID, resp *Response) error {
	respIssuerID := issuance.ResponderNameID(resp.Response)
	if reqIssuerID != respIssuerID {
		// This would be allowed if we used delegated responders, but we don't.
		return fmt.Errorf("responder name does not match requested issuer name")
	}

	err := src.checkNextUpdate(resp)
	if err != nil {
		return err
	}

	// In an ideal world, we'd also compare the Issuer Key Hash from the request's
	// CertID (equivalent to looking up the key hash in src.issuers) against the
	// Issuer Key Hash contained in the response's CertID. However, the Go OCSP
	// library does not provide access to the response's CertID, so we can't.
	// Specifically, we want to compare `src.issuers[reqIssuerID].keyHash` against
	// something like resp.CertID.IssuerKeyHash, but the latter does not exist.

	return nil
}
