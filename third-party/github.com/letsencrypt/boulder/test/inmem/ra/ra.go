package ra

import (
	"context"

	"github.com/letsencrypt/boulder/ra"
	rapb "github.com/letsencrypt/boulder/ra/proto"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

// RA meets the `rapb.RegistrationAuthorityClient` interface and acts as a
// wrapper for an inner `*ra.RegistrationAuthorityImpl` (which in turn meets
// the `rapb.RegistrationAuthorityServer` interface). Only methods used by
// unit tests need to be implemented.
type RA struct {
	rapb.RegistrationAuthorityClient
	Impl *ra.RegistrationAuthorityImpl
}

// AdministrativelyRevokeCertificate is a wrapper for `*ra.RegistrationAuthorityImpl.AdministrativelyRevokeCertificate`.
func (ra RA) AdministrativelyRevokeCertificate(ctx context.Context, req *rapb.AdministrativelyRevokeCertificateRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return ra.Impl.AdministrativelyRevokeCertificate(ctx, req)
}
