package live

import (
	"context"
	"errors"

	capb "github.com/letsencrypt/boulder/ca/proto"
	"github.com/letsencrypt/boulder/core"
	berrors "github.com/letsencrypt/boulder/errors"
	"github.com/letsencrypt/boulder/ocsp/responder"
	rapb "github.com/letsencrypt/boulder/ra/proto"
	"github.com/letsencrypt/boulder/semaphore"
	"golang.org/x/crypto/ocsp"
	"google.golang.org/grpc"
)

type ocspGenerator interface {
	GenerateOCSP(ctx context.Context, in *rapb.GenerateOCSPRequest, opts ...grpc.CallOption) (*capb.OCSPResponse, error)
}

type Source struct {
	ra  ocspGenerator
	sem *semaphore.Weighted
}

func New(ra ocspGenerator, maxInflight int64, maxWaiters int) *Source {
	return &Source{
		ra:  ra,
		sem: semaphore.NewWeighted(maxInflight, maxWaiters),
	}
}

func (s *Source) Response(ctx context.Context, req *ocsp.Request) (*responder.Response, error) {
	err := s.sem.Acquire(ctx, 1)
	if err != nil {
		return nil, err
	}
	defer s.sem.Release(1)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	resp, err := s.ra.GenerateOCSP(ctx, &rapb.GenerateOCSPRequest{
		Serial: core.SerialToString(req.SerialNumber),
	})
	if err != nil {
		if errors.Is(err, berrors.NotFound) {
			return nil, responder.ErrNotFound
		}
		return nil, err
	}
	parsed, err := ocsp.ParseResponse(resp.Response, nil)
	if err != nil {
		return nil, err
	}
	return &responder.Response{
		Raw:      resp.Response,
		Response: parsed,
	}, nil
}
