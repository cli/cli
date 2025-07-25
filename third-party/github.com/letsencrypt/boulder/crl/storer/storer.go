package storer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/big"
	"slices"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/letsencrypt/boulder/crl"
	"github.com/letsencrypt/boulder/crl/idp"
	cspb "github.com/letsencrypt/boulder/crl/storer/proto"
	"github.com/letsencrypt/boulder/issuance"
	blog "github.com/letsencrypt/boulder/log"
)

// simpleS3 matches the subset of the s3.Client interface which we use, to allow
// simpler mocking in tests.
type simpleS3 interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

type crlStorer struct {
	cspb.UnsafeCRLStorerServer
	s3Client         simpleS3
	s3Bucket         string
	issuers          map[issuance.NameID]*issuance.Certificate
	uploadCount      *prometheus.CounterVec
	sizeHistogram    *prometheus.HistogramVec
	latencyHistogram *prometheus.HistogramVec
	log              blog.Logger
	clk              clock.Clock
}

var _ cspb.CRLStorerServer = (*crlStorer)(nil)

func New(
	issuers []*issuance.Certificate,
	s3Client simpleS3,
	s3Bucket string,
	stats prometheus.Registerer,
	log blog.Logger,
	clk clock.Clock,
) (*crlStorer, error) {
	issuersByNameID := make(map[issuance.NameID]*issuance.Certificate, len(issuers))
	for _, issuer := range issuers {
		issuersByNameID[issuer.NameID()] = issuer
	}

	uploadCount := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "crl_storer_uploads",
		Help: "A counter of the number of CRLs uploaded by crl-storer",
	}, []string{"issuer", "result"})
	stats.MustRegister(uploadCount)

	sizeHistogram := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "crl_storer_sizes",
		Help:    "A histogram of the sizes (in bytes) of CRLs uploaded by crl-storer",
		Buckets: []float64{0, 256, 1024, 4096, 16384, 65536},
	}, []string{"issuer"})
	stats.MustRegister(sizeHistogram)

	latencyHistogram := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "crl_storer_upload_times",
		Help:    "A histogram of the time (in seconds) it took crl-storer to upload CRLs",
		Buckets: []float64{0.01, 0.2, 0.5, 1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000},
	}, []string{"issuer"})
	stats.MustRegister(latencyHistogram)

	return &crlStorer{
		issuers:          issuersByNameID,
		s3Client:         s3Client,
		s3Bucket:         s3Bucket,
		uploadCount:      uploadCount,
		sizeHistogram:    sizeHistogram,
		latencyHistogram: latencyHistogram,
		log:              log,
		clk:              clk,
	}, nil
}

// TODO(#6261): Unify all error messages to identify the shard they're working
// on as a JSON object including issuer, crl number, and shard number.

// UploadCRL implements the gRPC method of the same name. It takes a stream of
// bytes as its input, parses and runs some sanity checks on the CRL, and then
// uploads it to S3.
func (cs *crlStorer) UploadCRL(stream grpc.ClientStreamingServer[cspb.UploadCRLRequest, emptypb.Empty]) error {
	var issuer *issuance.Certificate
	var shardIdx int64
	var crlNumber *big.Int
	crlBytes := make([]byte, 0)
	var cacheControl string
	var expires time.Time

	// Read all of the messages from the input stream.
	for {
		in, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		switch payload := in.Payload.(type) {
		case *cspb.UploadCRLRequest_Metadata:
			if crlNumber != nil || issuer != nil {
				return errors.New("got more than one metadata message")
			}
			if payload.Metadata.IssuerNameID == 0 || payload.Metadata.Number == 0 {
				return errors.New("got incomplete metadata message")
			}

			cacheControl = payload.Metadata.CacheControl
			expires = payload.Metadata.Expires.AsTime()

			shardIdx = payload.Metadata.ShardIdx
			crlNumber = crl.Number(time.Unix(0, payload.Metadata.Number))

			var ok bool
			issuer, ok = cs.issuers[issuance.NameID(payload.Metadata.IssuerNameID)]
			if !ok {
				return fmt.Errorf("got unrecognized IssuerID: %d", payload.Metadata.IssuerNameID)
			}

		case *cspb.UploadCRLRequest_CrlChunk:
			crlBytes = append(crlBytes, payload.CrlChunk...)
		}
	}

	// Do some basic sanity checks on the received metadata and CRL.
	if issuer == nil || crlNumber == nil {
		return errors.New("got no metadata message")
	}

	crlId := crl.Id(issuer.NameID(), int(shardIdx), crlNumber)

	cs.sizeHistogram.WithLabelValues(issuer.Subject.CommonName).Observe(float64(len(crlBytes)))

	crl, err := x509.ParseRevocationList(crlBytes)
	if err != nil {
		return fmt.Errorf("parsing CRL for %s: %w", crlId, err)
	}

	if crl.Number.Cmp(crlNumber) != 0 {
		return errors.New("got mismatched CRL Number")
	}

	err = crl.CheckSignatureFrom(issuer.Certificate)
	if err != nil {
		return fmt.Errorf("validating signature for %s: %w", crlId, err)
	}

	// Before uploading this CRL, we want to compare it against the previous CRL
	// to ensure that the CRL Number field is not going backwards. This is an
	// additional safety check against clock skew and potential races, if multiple
	// crl-updaters are working on the same shard at the same time. We only run
	// these checks if we found a CRL, so we don't block uploading brand new CRLs.
	filename := fmt.Sprintf("%d/%d.crl", issuer.NameID(), shardIdx)
	prevObj, err := cs.s3Client.GetObject(stream.Context(), &s3.GetObjectInput{
		Bucket: &cs.s3Bucket,
		Key:    &filename,
	})
	if err != nil {
		var smithyErr *smithyhttp.ResponseError
		if !errors.As(err, &smithyErr) || smithyErr.HTTPStatusCode() != 404 {
			return fmt.Errorf("getting previous CRL for %s: %w", crlId, err)
		}
		cs.log.Infof("No previous CRL found for %s, proceeding", crlId)
	} else {
		prevBytes, err := io.ReadAll(prevObj.Body)
		if err != nil {
			return fmt.Errorf("downloading previous CRL for %s: %w", crlId, err)
		}

		prevCRL, err := x509.ParseRevocationList(prevBytes)
		if err != nil {
			return fmt.Errorf("parsing previous CRL for %s: %w", crlId, err)
		}

		if crl.Number.Cmp(prevCRL.Number) <= 0 {
			return fmt.Errorf("crlNumber not strictly increasing: %d <= %d", crl.Number, prevCRL.Number)
		}

		idpURIs, err := idp.GetIDPURIs(crl.Extensions)
		if err != nil {
			return fmt.Errorf("getting IDP for %s: %w", crlId, err)
		}

		prevURIs, err := idp.GetIDPURIs(prevCRL.Extensions)
		if err != nil {
			return fmt.Errorf("getting previous IDP for %s: %w", crlId, err)
		}

		uriMatch := false
		for _, uri := range idpURIs {
			if slices.Contains(prevURIs, uri) {
				uriMatch = true
				break
			}
		}
		if !uriMatch {
			return fmt.Errorf("IDP does not match previous: %v !∩ %v", idpURIs, prevURIs)
		}
	}

	// Finally actually upload the new CRL.
	start := cs.clk.Now()

	checksum := sha256.Sum256(crlBytes)
	checksumb64 := base64.StdEncoding.EncodeToString(checksum[:])
	crlContentType := "application/pkix-crl"
	_, err = cs.s3Client.PutObject(stream.Context(), &s3.PutObjectInput{
		Bucket:            &cs.s3Bucket,
		Key:               &filename,
		Body:              bytes.NewReader(crlBytes),
		ChecksumAlgorithm: types.ChecksumAlgorithmSha256,
		ChecksumSHA256:    &checksumb64,
		ContentType:       &crlContentType,
		Metadata:          map[string]string{"crlNumber": crlNumber.String()},
		Expires:           &expires,
		CacheControl:      &cacheControl,
	})

	latency := cs.clk.Now().Sub(start)
	cs.latencyHistogram.WithLabelValues(issuer.Subject.CommonName).Observe(latency.Seconds())

	if err != nil {
		cs.uploadCount.WithLabelValues(issuer.Subject.CommonName, "failed").Inc()
		cs.log.AuditErrf("CRL upload failed: id=[%s] err=[%s]", crlId, err)
		return fmt.Errorf("uploading to S3: %w", err)
	}

	cs.uploadCount.WithLabelValues(issuer.Subject.CommonName, "success").Inc()
	cs.log.AuditInfof(
		"CRL uploaded: id=[%s] issuerCN=[%s] thisUpdate=[%s] nextUpdate=[%s] numEntries=[%d]",
		crlId, issuer.Subject.CommonName, crl.ThisUpdate, crl.NextUpdate, len(crl.RevokedCertificateEntries),
	)

	return stream.SendAndClose(&emptypb.Empty{})
}
