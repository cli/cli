package ca

import (
	"crypto/sha256"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"strings"

	"google.golang.org/grpc"

	"github.com/prometheus/client_golang/prometheus"

	capb "github.com/letsencrypt/boulder/ca/proto"
	"github.com/letsencrypt/boulder/core"
	corepb "github.com/letsencrypt/boulder/core/proto"
	bcrl "github.com/letsencrypt/boulder/crl"
	"github.com/letsencrypt/boulder/issuance"
	blog "github.com/letsencrypt/boulder/log"
)

type crlImpl struct {
	capb.UnsafeCRLGeneratorServer
	issuers   map[issuance.NameID]*issuance.Issuer
	profile   *issuance.CRLProfile
	maxLogLen int
	log       blog.Logger
	metrics   *caMetrics
}

var _ capb.CRLGeneratorServer = (*crlImpl)(nil)

// NewCRLImpl returns a new object which fulfils the ca.proto CRLGenerator
// interface. It uses the list of issuers to determine what issuers it can
// issue CRLs from. lifetime sets the validity period (inclusive) of the
// resulting CRLs.
func NewCRLImpl(
	issuers []*issuance.Issuer,
	profileConfig issuance.CRLProfileConfig,
	maxLogLen int,
	logger blog.Logger,
	metrics *caMetrics,
) (*crlImpl, error) {
	issuersByNameID := make(map[issuance.NameID]*issuance.Issuer, len(issuers))
	for _, issuer := range issuers {
		issuersByNameID[issuer.NameID()] = issuer
	}

	profile, err := issuance.NewCRLProfile(profileConfig)
	if err != nil {
		return nil, fmt.Errorf("loading CRL profile: %w", err)
	}

	return &crlImpl{
		issuers:   issuersByNameID,
		profile:   profile,
		maxLogLen: maxLogLen,
		log:       logger,
		metrics:   metrics,
	}, nil
}

func (ci *crlImpl) GenerateCRL(stream grpc.BidiStreamingServer[capb.GenerateCRLRequest, capb.GenerateCRLResponse]) error {
	var issuer *issuance.Issuer
	var req *issuance.CRLRequest
	rcs := make([]x509.RevocationListEntry, 0)

	for {
		in, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		switch payload := in.Payload.(type) {
		case *capb.GenerateCRLRequest_Metadata:
			if req != nil {
				return errors.New("got more than one metadata message")
			}

			req, err = ci.metadataToRequest(payload.Metadata)
			if err != nil {
				return err
			}

			var ok bool
			issuer, ok = ci.issuers[issuance.NameID(payload.Metadata.IssuerNameID)]
			if !ok {
				return fmt.Errorf("got unrecognized IssuerNameID: %d", payload.Metadata.IssuerNameID)
			}

		case *capb.GenerateCRLRequest_Entry:
			rc, err := ci.entryToRevokedCertificate(payload.Entry)
			if err != nil {
				return err
			}

			rcs = append(rcs, *rc)

		default:
			return errors.New("got empty or malformed message in input stream")
		}
	}

	if req == nil {
		return errors.New("no crl metadata received")
	}

	// Compute a unique ID for this issuer-number-shard combo, to tie together all
	// the audit log lines related to its issuance.
	logID := blog.LogLineChecksum(fmt.Sprintf("%d", issuer.NameID()) + req.Number.String() + fmt.Sprintf("%d", req.Shard))
	ci.log.AuditInfof(
		"Signing CRL: logID=[%s] issuer=[%s] number=[%s] shard=[%d] thisUpdate=[%s] numEntries=[%d]",
		logID, issuer.Cert.Subject.CommonName, req.Number.String(), req.Shard, req.ThisUpdate, len(rcs),
	)

	if len(rcs) > 0 {
		builder := strings.Builder{}
		for i := range len(rcs) {
			if builder.Len() == 0 {
				fmt.Fprintf(&builder, "Signing CRL: logID=[%s] entries=[", logID)
			}

			fmt.Fprintf(&builder, "%x:%d,", rcs[i].SerialNumber.Bytes(), rcs[i].ReasonCode)

			if builder.Len() >= ci.maxLogLen {
				fmt.Fprint(&builder, "]")
				ci.log.AuditInfo(builder.String())
				builder = strings.Builder{}
			}
		}
		fmt.Fprint(&builder, "]")
		ci.log.AuditInfo(builder.String())
	}

	req.Entries = rcs

	crlBytes, err := issuer.IssueCRL(ci.profile, req)
	if err != nil {
		ci.metrics.noteSignError(err)
		return fmt.Errorf("signing crl: %w", err)
	}
	ci.metrics.signatureCount.With(prometheus.Labels{"purpose": "crl", "issuer": issuer.Name()}).Inc()

	hash := sha256.Sum256(crlBytes)
	ci.log.AuditInfof(
		"Signing CRL success: logID=[%s] size=[%d] hash=[%x]",
		logID, len(crlBytes), hash,
	)

	for i := 0; i < len(crlBytes); i += 1000 {
		j := i + 1000
		if j > len(crlBytes) {
			j = len(crlBytes)
		}
		err = stream.Send(&capb.GenerateCRLResponse{
			Chunk: crlBytes[i:j],
		})
		if err != nil {
			return err
		}
		if i%1000 == 0 {
			ci.log.Debugf("Wrote %d bytes to output stream", i*1000)
		}
	}

	return nil
}

func (ci *crlImpl) metadataToRequest(meta *capb.CRLMetadata) (*issuance.CRLRequest, error) {
	if core.IsAnyNilOrZero(meta.IssuerNameID, meta.ThisUpdate, meta.ShardIdx) {
		return nil, errors.New("got incomplete metadata message")
	}
	thisUpdate := meta.ThisUpdate.AsTime()
	number := bcrl.Number(thisUpdate)

	return &issuance.CRLRequest{
		Number:     number,
		Shard:      meta.ShardIdx,
		ThisUpdate: thisUpdate,
	}, nil
}

func (ci *crlImpl) entryToRevokedCertificate(entry *corepb.CRLEntry) (*x509.RevocationListEntry, error) {
	serial, err := core.StringToSerial(entry.Serial)
	if err != nil {
		return nil, err
	}

	if core.IsAnyNilOrZero(entry.RevokedAt) {
		return nil, errors.New("got empty or zero revocation timestamp")
	}
	revokedAt := entry.RevokedAt.AsTime()

	return &x509.RevocationListEntry{
		SerialNumber:   serial,
		RevocationTime: revokedAt,
		ReasonCode:     int(entry.Reason),
	}, nil
}
