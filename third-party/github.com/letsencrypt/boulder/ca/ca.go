package ca

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	mrand "math/rand"
	"strings"
	"time"

	ct "github.com/google/certificate-transparency-go"
	cttls "github.com/google/certificate-transparency-go/tls"
	"github.com/jmhodges/clock"
	"github.com/miekg/pkcs11"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/zmap/zlint/v3/lint"
	"golang.org/x/crypto/cryptobyte"
	cryptobyte_asn1 "golang.org/x/crypto/cryptobyte/asn1"
	"golang.org/x/crypto/ocsp"
	"google.golang.org/protobuf/types/known/timestamppb"

	capb "github.com/letsencrypt/boulder/ca/proto"
	"github.com/letsencrypt/boulder/core"
	corepb "github.com/letsencrypt/boulder/core/proto"
	csrlib "github.com/letsencrypt/boulder/csr"
	berrors "github.com/letsencrypt/boulder/errors"
	"github.com/letsencrypt/boulder/features"
	"github.com/letsencrypt/boulder/goodkey"
	"github.com/letsencrypt/boulder/issuance"
	"github.com/letsencrypt/boulder/linter"
	blog "github.com/letsencrypt/boulder/log"
	sapb "github.com/letsencrypt/boulder/sa/proto"
)

type certificateType string

const (
	precertType = certificateType("precertificate")
	certType    = certificateType("certificate")
)

// Two maps of keys to Issuers. Lookup by PublicKeyAlgorithm is useful for
// determining the set of issuers which can sign a given (pre)cert, based on its
// PublicKeyAlgorithm. Lookup by NameID is useful for looking up a specific
// issuer based on the issuer of a given (pre)certificate.
type issuerMaps struct {
	byAlg    map[x509.PublicKeyAlgorithm][]*issuance.Issuer
	byNameID map[issuance.NameID]*issuance.Issuer
}

type certProfileWithID struct {
	// name is a human readable name used to refer to the certificate profile.
	name string
	// hash is SHA256 sum over every exported field of an issuance.ProfileConfig
	// used to generate the embedded *issuance.Profile.
	hash    [32]byte
	profile *issuance.Profile
}

// certProfilesMaps allows looking up the human-readable name of a certificate
// profile to retrieve the actual profile. The default profile to be used is
// stored alongside the maps.
type certProfilesMaps struct {
	// The name of the profile that will be selected if no explicit profile name
	// is provided via gRPC.
	defaultName string

	profileByHash map[[32]byte]*certProfileWithID
	profileByName map[string]*certProfileWithID
}

// caMetrics holds various metrics which are shared between caImpl, ocspImpl,
// and crlImpl.
type caMetrics struct {
	signatureCount *prometheus.CounterVec
	signErrorCount *prometheus.CounterVec
	lintErrorCount prometheus.Counter
}

func NewCAMetrics(stats prometheus.Registerer) *caMetrics {
	signatureCount := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "signatures",
			Help: "Number of signatures",
		},
		[]string{"purpose", "issuer"})
	stats.MustRegister(signatureCount)

	signErrorCount := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "signature_errors",
		Help: "A counter of signature errors labelled by error type",
	}, []string{"type"})
	stats.MustRegister(signErrorCount)

	lintErrorCount := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "lint_errors",
			Help: "Number of issuances that were halted by linting errors",
		})
	stats.MustRegister(lintErrorCount)

	return &caMetrics{signatureCount, signErrorCount, lintErrorCount}
}

func (m *caMetrics) noteSignError(err error) {
	var pkcs11Error pkcs11.Error
	if errors.As(err, &pkcs11Error) {
		m.signErrorCount.WithLabelValues("HSM").Inc()
	}
}

// certificateAuthorityImpl represents a CA that signs certificates.
// It can sign OCSP responses as well, but only via delegation to an ocspImpl.
type certificateAuthorityImpl struct {
	capb.UnsafeCertificateAuthorityServer
	sa           sapb.StorageAuthorityCertificateClient
	pa           core.PolicyAuthority
	issuers      issuerMaps
	certProfiles certProfilesMaps

	// This is temporary, and will be used for testing and slow roll-out
	// of ECDSA issuance, but will then be removed.
	ecdsaAllowList *ECDSAAllowList
	prefix         int // Prepended to the serial number
	validityPeriod time.Duration
	backdate       time.Duration
	maxNames       int
	keyPolicy      goodkey.KeyPolicy
	clk            clock.Clock
	log            blog.Logger
	metrics        *caMetrics
}

var _ capb.CertificateAuthorityServer = (*certificateAuthorityImpl)(nil)

// makeIssuerMaps processes a list of issuers into a set of maps for easy
// lookup either by key algorithm (useful for picking an issuer for a precert)
// or by unique ID (useful for final certs, OCSP, and CRLs). If two issuers with
// the same unique ID are encountered, an error is returned.
func makeIssuerMaps(issuers []*issuance.Issuer) (issuerMaps, error) {
	issuersByAlg := make(map[x509.PublicKeyAlgorithm][]*issuance.Issuer, 2)
	issuersByNameID := make(map[issuance.NameID]*issuance.Issuer, len(issuers))
	for _, issuer := range issuers {
		if _, found := issuersByNameID[issuer.NameID()]; found {
			return issuerMaps{}, fmt.Errorf("two issuers with same NameID %d (%s) configured", issuer.NameID(), issuer.Name())
		}
		issuersByNameID[issuer.NameID()] = issuer
		if issuer.IsActive() {
			issuersByAlg[issuer.KeyType()] = append(issuersByAlg[issuer.KeyType()], issuer)
		}
	}
	if i, ok := issuersByAlg[x509.ECDSA]; !ok || len(i) == 0 {
		return issuerMaps{}, errors.New("no ECDSA issuers configured")
	}
	if i, ok := issuersByAlg[x509.RSA]; !ok || len(i) == 0 {
		return issuerMaps{}, errors.New("no RSA issuers configured")
	}
	return issuerMaps{issuersByAlg, issuersByNameID}, nil
}

// makeCertificateProfilesMap processes a set of named certificate issuance
// profile configs into a two pre-computed maps: 1) a human-readable name to the
// profile and 2) a unique hash over contents of the profile to the profile
// itself. It returns the maps or an error if a duplicate name or hash is found.
// It also associates the given lint registry with each profile.
//
// The unique hash is used in the case of
//   - RA instructs CA1 to issue a precertificate
//   - CA1 returns the precertificate DER bytes and profile hash to the RA
//   - RA instructs CA2 to issue a final certificate, but CA2 does not contain a
//     profile corresponding to that hash and an issuance is prevented.
func makeCertificateProfilesMap(defaultName string, profiles map[string]issuance.ProfileConfig, lints lint.Registry) (certProfilesMaps, error) {
	if len(profiles) <= 0 {
		return certProfilesMaps{}, fmt.Errorf("must pass at least one certificate profile")
	}

	// Check that a profile exists with the configured default profile name.
	_, ok := profiles[defaultName]
	if !ok {
		return certProfilesMaps{}, fmt.Errorf("defaultCertificateProfileName:\"%s\" was configured, but a profile object was not found for that name", defaultName)
	}

	profileByName := make(map[string]*certProfileWithID, len(profiles))
	profileByHash := make(map[[32]byte]*certProfileWithID, len(profiles))

	for name, profileConfig := range profiles {
		profile, err := issuance.NewProfile(profileConfig, lints)
		if err != nil {
			return certProfilesMaps{}, err
		}

		// gob can only encode exported fields, of which an issuance.Profile has
		// none. However, since we're already in a loop iteration having access
		// to the issuance.ProfileConfig used to generate the issuance.Profile,
		// we'll generate the hash from that.
		var encodedProfile bytes.Buffer
		enc := gob.NewEncoder(&encodedProfile)
		err = enc.Encode(profileConfig)
		if err != nil {
			return certProfilesMaps{}, err
		}
		if len(encodedProfile.Bytes()) <= 0 {
			return certProfilesMaps{}, fmt.Errorf("certificate profile encoding returned 0 bytes")
		}
		hash := sha256.Sum256(encodedProfile.Bytes())

		_, ok := profileByName[name]
		if !ok {
			profileByName[name] = &certProfileWithID{
				name:    name,
				hash:    hash,
				profile: profile,
			}
		} else {
			return certProfilesMaps{}, fmt.Errorf("duplicate certificate profile name %s", name)
		}

		_, ok = profileByHash[hash]
		if !ok {
			profileByHash[hash] = &certProfileWithID{
				name:    name,
				hash:    hash,
				profile: profile,
			}
		} else {
			return certProfilesMaps{}, fmt.Errorf("duplicate certificate profile hash %d", hash)
		}
	}

	return certProfilesMaps{defaultName, profileByHash, profileByName}, nil
}

// NewCertificateAuthorityImpl creates a CA instance that can sign certificates
// from any number of issuance.Issuers according to their profiles, and can sign
// OCSP (via delegation to an ocspImpl and its issuers).
func NewCertificateAuthorityImpl(
	sa sapb.StorageAuthorityCertificateClient,
	pa core.PolicyAuthority,
	boulderIssuers []*issuance.Issuer,
	defaultCertProfileName string,
	certificateProfiles map[string]issuance.ProfileConfig,
	lints lint.Registry,
	ecdsaAllowList *ECDSAAllowList,
	certExpiry time.Duration,
	certBackdate time.Duration,
	serialPrefix int,
	maxNames int,
	keyPolicy goodkey.KeyPolicy,
	logger blog.Logger,
	metrics *caMetrics,
	clk clock.Clock,
) (*certificateAuthorityImpl, error) {
	var ca *certificateAuthorityImpl
	var err error

	// TODO(briansmith): Make the backdate setting mandatory after the
	// production ca.json has been updated to include it. Until then, manually
	// default to 1h, which is the backdating duration we currently use.
	if certBackdate == 0 {
		certBackdate = time.Hour
	}

	if serialPrefix < 1 || serialPrefix > 127 {
		err = errors.New("serial prefix must be between 1 and 127")
		return nil, err
	}

	if len(boulderIssuers) == 0 {
		return nil, errors.New("must have at least one issuer")
	}

	certProfiles, err := makeCertificateProfilesMap(defaultCertProfileName, certificateProfiles, lints)
	if err != nil {
		return nil, err
	}

	issuers, err := makeIssuerMaps(boulderIssuers)
	if err != nil {
		return nil, err
	}

	ca = &certificateAuthorityImpl{
		sa:             sa,
		pa:             pa,
		issuers:        issuers,
		certProfiles:   certProfiles,
		validityPeriod: certExpiry,
		backdate:       certBackdate,
		prefix:         serialPrefix,
		maxNames:       maxNames,
		keyPolicy:      keyPolicy,
		log:            logger,
		metrics:        metrics,
		clk:            clk,
		ecdsaAllowList: ecdsaAllowList,
	}

	return ca, nil
}

var ocspStatusToCode = map[string]int{
	"good":    ocsp.Good,
	"revoked": ocsp.Revoked,
	"unknown": ocsp.Unknown,
}

// IssuePrecertificate is the first step in the [issuance cycle]. It allocates and stores a serial number,
// selects a certificate profile, generates and stores a linting certificate, sets the serial's status to
// "wait", signs and stores a precertificate, updates the serial's status to "good", then returns the
// precertificate.
//
// Subsequent final issuance based on this precertificate must happen at most once, and must use the same
// certificate profile. The certificate profile is identified by a hash to ensure an exact match even if
// the configuration for a specific profile _name_ changes.
//
// [issuance cycle]: https://github.com/letsencrypt/boulder/blob/main/docs/ISSUANCE-CYCLE.md
func (ca *certificateAuthorityImpl) IssuePrecertificate(ctx context.Context, issueReq *capb.IssueCertificateRequest) (*capb.IssuePrecertificateResponse, error) {
	// issueReq.orderID may be zero, for ACMEv1 requests.
	// issueReq.CertProfileName may be empty and will be populated in
	// issuePrecertificateInner if so.
	if core.IsAnyNilOrZero(issueReq, issueReq.Csr, issueReq.RegistrationID) {
		return nil, berrors.InternalServerError("Incomplete issue certificate request")
	}

	serialBigInt, validity, err := ca.generateSerialNumberAndValidity()
	if err != nil {
		return nil, err
	}

	serialHex := core.SerialToString(serialBigInt)
	regID := issueReq.RegistrationID
	_, err = ca.sa.AddSerial(ctx, &sapb.AddSerialRequest{
		Serial:  serialHex,
		RegID:   regID,
		Created: timestamppb.New(ca.clk.Now()),
		Expires: timestamppb.New(validity.NotAfter),
	})
	if err != nil {
		return nil, err
	}

	precertDER, cpwid, err := ca.issuePrecertificateInner(ctx, issueReq, serialBigInt, validity)
	if err != nil {
		return nil, err
	}

	_, err = ca.sa.SetCertificateStatusReady(ctx, &sapb.Serial{Serial: serialHex})
	if err != nil {
		return nil, err
	}

	return &capb.IssuePrecertificateResponse{
		DER:             precertDER,
		CertProfileName: cpwid.name,
		CertProfileHash: cpwid.hash[:],
	}, nil
}

// IssueCertificateForPrecertificate final step in the [issuance cycle].
//
// Given a precertificate and a set of SCTs for that precertificate, it generates
// a linting final certificate, then signs a final certificate using a real issuer.
// The poison extension is removed from the precertificate and a
// SCT list extension is inserted in its place. Except for this and the
// signature the final certificate exactly matches the precertificate.
//
// It's critical not to sign two different final certificates for the same
// precertificate. This can happen, for instance, if the caller provides a
// different set of SCTs on subsequent calls to  IssueCertificateForPrecertificate.
// We rely on the RA not to call IssueCertificateForPrecertificate twice for the
// same serial. This is accomplished by the fact that
// IssueCertificateForPrecertificate is only ever called in a straight-through
// RPC path without retries. If there is any error, including a networking
// error, the whole certificate issuance attempt fails and any subsequent
// issuance will use a different serial number.
//
// We also check that the provided serial number does not already exist as a
// final certificate, but this is just a belt-and-suspenders measure, since
// there could be race conditions where two goroutines are issuing for the same
// serial number at the same time.
//
// [issuance cycle]: https://github.com/letsencrypt/boulder/blob/main/docs/ISSUANCE-CYCLE.md
func (ca *certificateAuthorityImpl) IssueCertificateForPrecertificate(ctx context.Context, req *capb.IssueCertificateForPrecertificateRequest) (*corepb.Certificate, error) {
	// issueReq.orderID may be zero, for ACMEv1 requests.
	if core.IsAnyNilOrZero(req, req.DER, req.SCTs, req.RegistrationID, req.CertProfileHash) {
		return nil, berrors.InternalServerError("Incomplete cert for precertificate request")
	}

	// The certificate profile hash is checked here instead of the name because
	// the hash is over the entire contents of a *ProfileConfig giving assurance
	// that the certificate profile has remained unchanged during the roundtrip
	// from a CA, to the RA, then back to a (potentially different) CA node.
	certProfile, ok := ca.certProfiles.profileByHash[[32]byte(req.CertProfileHash)]
	if !ok {
		return nil, fmt.Errorf("the CA is incapable of using a profile with hash %d", req.CertProfileHash)
	}

	precert, err := x509.ParseCertificate(req.DER)
	if err != nil {
		return nil, err
	}

	serialHex := core.SerialToString(precert.SerialNumber)
	if _, err = ca.sa.GetCertificate(ctx, &sapb.Serial{Serial: serialHex}); err == nil {
		err = berrors.InternalServerError("issuance of duplicate final certificate requested: %s", serialHex)
		ca.log.AuditErr(err.Error())
		return nil, err
	} else if !errors.Is(err, berrors.NotFound) {
		return nil, fmt.Errorf("error checking for duplicate issuance of %s: %s", serialHex, err)
	}
	var scts []ct.SignedCertificateTimestamp
	for _, sctBytes := range req.SCTs {
		var sct ct.SignedCertificateTimestamp
		_, err = cttls.Unmarshal(sctBytes, &sct)
		if err != nil {
			return nil, err
		}
		scts = append(scts, sct)
	}

	issuer, ok := ca.issuers.byNameID[issuance.IssuerNameID(precert)]
	if !ok {
		return nil, berrors.InternalServerError("no issuer found for Issuer Name %s", precert.Issuer)
	}

	issuanceReq, err := issuance.RequestFromPrecert(precert, scts)
	if err != nil {
		return nil, err
	}

	names := strings.Join(issuanceReq.DNSNames, ", ")
	ca.log.AuditInfof("Signing cert: issuer=[%s] serial=[%s] regID=[%d] names=[%s] certProfileName=[%s] certProfileHash=[%x] precert=[%s]",
		issuer.Name(), serialHex, req.RegistrationID, names, certProfile.name, certProfile.hash, hex.EncodeToString(precert.Raw))

	lintCertBytes, issuanceToken, err := issuer.Prepare(certProfile.profile, issuanceReq)
	if err != nil {
		ca.log.AuditErrf("Preparing cert failed: issuer=[%s] serial=[%s] regID=[%d] names=[%s] certProfileName=[%s] certProfileHash=[%x] err=[%v]",
			issuer.Name(), serialHex, req.RegistrationID, names, certProfile.name, certProfile.hash, err)
		return nil, berrors.InternalServerError("failed to prepare certificate signing: %s", err)
	}

	certDER, err := issuer.Issue(issuanceToken)
	if err != nil {
		ca.metrics.noteSignError(err)
		ca.log.AuditErrf("Signing cert failed: issuer=[%s] serial=[%s] regID=[%d] names=[%s] certProfileName=[%s] certProfileHash=[%x] err=[%v]",
			issuer.Name(), serialHex, req.RegistrationID, names, certProfile.name, certProfile.hash, err)
		return nil, berrors.InternalServerError("failed to sign certificate: %s", err)
	}

	err = tbsCertIsDeterministic(lintCertBytes, certDER)
	if err != nil {
		return nil, err
	}

	ca.metrics.signatureCount.With(prometheus.Labels{"purpose": string(certType), "issuer": issuer.Name()}).Inc()
	ca.log.AuditInfof("Signing cert success: issuer=[%s] serial=[%s] regID=[%d] names=[%s] certificate=[%s] certProfileName=[%s] certProfileHash=[%x]",
		issuer.Name(), serialHex, req.RegistrationID, names, hex.EncodeToString(certDER), certProfile.name, certProfile.hash)

	_, err = ca.sa.AddCertificate(ctx, &sapb.AddCertificateRequest{
		Der:    certDER,
		RegID:  req.RegistrationID,
		Issued: timestamppb.New(ca.clk.Now()),
	})
	if err != nil {
		ca.log.AuditErrf("Failed RPC to store at SA: issuer=[%s] serial=[%s] cert=[%s] regID=[%d] orderID=[%d] certProfileName=[%s] certProfileHash=[%x] err=[%v]",
			issuer.Name(), serialHex, hex.EncodeToString(certDER), req.RegistrationID, req.OrderID, certProfile.name, certProfile.hash, err)
		return nil, err
	}

	return &corepb.Certificate{
		RegistrationID: req.RegistrationID,
		Serial:         core.SerialToString(precert.SerialNumber),
		Der:            certDER,
		Digest:         core.Fingerprint256(certDER),
		Issued:         timestamppb.New(precert.NotBefore),
		Expires:        timestamppb.New(precert.NotAfter),
	}, nil
}

type validity struct {
	NotBefore time.Time
	NotAfter  time.Time
}

func (ca *certificateAuthorityImpl) generateSerialNumberAndValidity() (*big.Int, validity, error) {
	// We want 136 bits of random number, plus an 8-bit instance id prefix.
	const randBits = 136
	serialBytes := make([]byte, randBits/8+1)
	serialBytes[0] = byte(ca.prefix)
	_, err := rand.Read(serialBytes[1:])
	if err != nil {
		err = berrors.InternalServerError("failed to generate serial: %s", err)
		ca.log.AuditErrf("Serial randomness failed, err=[%v]", err)
		return nil, validity{}, err
	}
	serialBigInt := big.NewInt(0)
	serialBigInt = serialBigInt.SetBytes(serialBytes)

	notBefore := ca.clk.Now().Add(-ca.backdate)
	validity := validity{
		NotBefore: notBefore,
		NotAfter:  notBefore.Add(ca.validityPeriod - time.Second),
	}

	return serialBigInt, validity, nil
}

// generateSKID computes the Subject Key Identifier using one of the methods in
// RFC 7093 Section 2 Additional Methods for Generating Key Identifiers:
// The keyIdentifier [may be] composed of the leftmost 160-bits of the
// SHA-256 hash of the value of the BIT STRING subjectPublicKey
// (excluding the tag, length, and number of unused bits).
func generateSKID(pk crypto.PublicKey) ([]byte, error) {
	pkBytes, err := x509.MarshalPKIXPublicKey(pk)
	if err != nil {
		return nil, err
	}

	var pkixPublicKey struct {
		Algo      pkix.AlgorithmIdentifier
		BitString asn1.BitString
	}
	if _, err := asn1.Unmarshal(pkBytes, &pkixPublicKey); err != nil {
		return nil, err
	}

	skid := sha256.Sum256(pkixPublicKey.BitString.Bytes)
	return skid[0:20:20], nil
}

func (ca *certificateAuthorityImpl) issuePrecertificateInner(ctx context.Context, issueReq *capb.IssueCertificateRequest, serialBigInt *big.Int, validity validity) ([]byte, *certProfileWithID, error) {
	// The CA must check if it is capable of issuing for the given certificate
	// profile name. The name is checked here instead of the hash because the RA
	// is unaware of what certificate profiles exist. Pre-existing orders stored
	// in the database may not have an associated certificate profile name and
	// will take the default name stored alongside the map.
	if issueReq.CertProfileName == "" {
		issueReq.CertProfileName = ca.certProfiles.defaultName
	}
	certProfile, ok := ca.certProfiles.profileByName[issueReq.CertProfileName]
	if !ok {
		return nil, nil, fmt.Errorf("the CA is incapable of using a profile named %s", issueReq.CertProfileName)
	}

	csr, err := x509.ParseCertificateRequest(issueReq.Csr)
	if err != nil {
		return nil, nil, err
	}

	err = csrlib.VerifyCSR(ctx, csr, ca.maxNames, &ca.keyPolicy, ca.pa)
	if err != nil {
		ca.log.AuditErr(err.Error())
		// VerifyCSR returns berror instances that can be passed through as-is
		// without wrapping.
		return nil, nil, err
	}

	// Select which pool of issuers to use, based on the to-be-issued cert's key
	// type and whether we're using the ECDSA Allow List.
	alg := csr.PublicKeyAlgorithm
	if alg == x509.ECDSA && !features.Get().ECDSAForAll && ca.ecdsaAllowList != nil && !ca.ecdsaAllowList.permitted(issueReq.RegistrationID) {
		alg = x509.RSA
	}

	// Select a random issuer from among the active issuers of this key type.
	issuerPool, ok := ca.issuers.byAlg[alg]
	if !ok || len(issuerPool) == 0 {
		return nil, nil, berrors.InternalServerError("no issuers found for public key algorithm %s", csr.PublicKeyAlgorithm)
	}
	issuer := issuerPool[mrand.Intn(len(issuerPool))]

	if issuer.Cert.NotAfter.Before(validity.NotAfter) {
		err = berrors.InternalServerError("cannot issue a certificate that expires after the issuer certificate")
		ca.log.AuditErr(err.Error())
		return nil, nil, err
	}

	subjectKeyId, err := generateSKID(csr.PublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("computing subject key ID: %w", err)
	}

	serialHex := core.SerialToString(serialBigInt)

	ca.log.AuditInfof("Signing precert: serial=[%s] regID=[%d] names=[%s] csr=[%s]",
		serialHex, issueReq.RegistrationID, strings.Join(csr.DNSNames, ", "), hex.EncodeToString(csr.Raw))

	names := csrlib.NamesFromCSR(csr)
	req := &issuance.IssuanceRequest{
		PublicKey:         csr.PublicKey,
		SubjectKeyId:      subjectKeyId,
		Serial:            serialBigInt.Bytes(),
		DNSNames:          names.SANs,
		CommonName:        names.CN,
		IncludeCTPoison:   true,
		IncludeMustStaple: issuance.ContainsMustStaple(csr.Extensions),
		NotBefore:         validity.NotBefore,
		NotAfter:          validity.NotAfter,
	}

	lintCertBytes, issuanceToken, err := issuer.Prepare(certProfile.profile, req)
	if err != nil {
		ca.log.AuditErrf("Preparing precert failed: issuer=[%s] serial=[%s] regID=[%d] names=[%s] certProfileName=[%s] certProfileHash=[%x] err=[%v]",
			issuer.Name(), serialHex, issueReq.RegistrationID, strings.Join(csr.DNSNames, ", "), certProfile.name, certProfile.hash, err)
		if errors.Is(err, linter.ErrLinting) {
			ca.metrics.lintErrorCount.Inc()
		}
		return nil, nil, berrors.InternalServerError("failed to prepare precertificate signing: %s", err)
	}

	_, err = ca.sa.AddPrecertificate(context.Background(), &sapb.AddCertificateRequest{
		Der:          lintCertBytes,
		RegID:        issueReq.RegistrationID,
		Issued:       timestamppb.New(ca.clk.Now()),
		IssuerNameID: int64(issuer.NameID()),
		OcspNotReady: true,
	})
	if err != nil {
		return nil, nil, err
	}

	certDER, err := issuer.Issue(issuanceToken)
	if err != nil {
		ca.metrics.noteSignError(err)
		ca.log.AuditErrf("Signing precert failed: issuer=[%s] serial=[%s] regID=[%d] names=[%s] certProfileName=[%s] certProfileHash=[%x] err=[%v]",
			issuer.Name(), serialHex, issueReq.RegistrationID, strings.Join(csr.DNSNames, ", "), certProfile.name, certProfile.hash, err)
		return nil, nil, berrors.InternalServerError("failed to sign precertificate: %s", err)
	}

	err = tbsCertIsDeterministic(lintCertBytes, certDER)
	if err != nil {
		return nil, nil, err
	}

	ca.metrics.signatureCount.With(prometheus.Labels{"purpose": string(precertType), "issuer": issuer.Name()}).Inc()
	ca.log.AuditInfof("Signing precert success: issuer=[%s] serial=[%s] regID=[%d] names=[%s] precertificate=[%s] certProfileName=[%s] certProfileHash=[%x]",
		issuer.Name(), serialHex, issueReq.RegistrationID, strings.Join(csr.DNSNames, ", "), hex.EncodeToString(certDER), certProfile.name, certProfile.hash)

	return certDER, &certProfileWithID{certProfile.name, certProfile.hash, nil}, nil
}

// verifyTBSCertIsDeterministic verifies that x509.CreateCertificate signing
// operation is deterministic and produced identical DER bytes between the given
// lint certificate and leaf certificate. If the DER byte equality check fails
// it's mississuance, but it's better to know about the problem sooner than
// later. The caller is responsible for passing the appropriate valid
// certificate bytes in the correct position.
func tbsCertIsDeterministic(lintCertBytes []byte, leafCertBytes []byte) error {
	if core.IsAnyNilOrZero(lintCertBytes, leafCertBytes) {
		return fmt.Errorf("lintCertBytes of leafCertBytes were nil")
	}

	// extractTBSCertBytes is a partial copy of //crypto/x509/parser.go to
	// extract the RawTBSCertificate field from given DER bytes. It the
	// RawTBSCertificate field bytes or an error if the given bytes cannot be
	// parsed. This is far more performant than parsing the entire *Certificate
	// structure with x509.ParseCertificate().
	//
	// RFC 5280, Section 4.1
	//    Certificate  ::=  SEQUENCE  {
	//      tbsCertificate       TBSCertificate,
	//      signatureAlgorithm   AlgorithmIdentifier,
	//      signatureValue       BIT STRING  }
	//
	//    TBSCertificate  ::=  SEQUENCE  {
	//      ..
	extractTBSCertBytes := func(inputDERBytes *[]byte) ([]byte, error) {
		input := cryptobyte.String(*inputDERBytes)

		// Extract the Certificate bytes
		if !input.ReadASN1(&input, cryptobyte_asn1.SEQUENCE) {
			return nil, errors.New("malformed certificate")
		}

		var tbs cryptobyte.String
		// Extract the TBSCertificate bytes from the Certificate bytes
		if !input.ReadASN1(&tbs, cryptobyte_asn1.SEQUENCE) {
			return nil, errors.New("malformed tbs certificate")
		}

		if tbs.Empty() {
			return nil, errors.New("parsed RawTBSCertificate field was empty")
		}

		return tbs, nil
	}

	lintRawTBSCert, err := extractTBSCertBytes(&lintCertBytes)
	if err != nil {
		return fmt.Errorf("while extracting lint TBS cert: %w", err)
	}

	leafRawTBSCert, err := extractTBSCertBytes(&leafCertBytes)
	if err != nil {
		return fmt.Errorf("while extracting leaf TBS cert: %w", err)
	}

	if !bytes.Equal(lintRawTBSCert, leafRawTBSCert) {
		return fmt.Errorf("mismatch between lintCert and leafCert RawTBSCertificate DER bytes: \"%x\" != \"%x\"", lintRawTBSCert, leafRawTBSCert)
	}

	return nil
}
