// Copied from the auto-generated sa_grpc.pb.go

package proto

import (
	context "context"

	proto "github.com/letsencrypt/boulder/core/proto"
	grpc "google.golang.org/grpc"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// StorageAuthorityCertificateClient is a subset of the sapb.StorageAuthorityClient interface that only reads and writes certificates
type StorageAuthorityCertificateClient interface {
	AddSerial(ctx context.Context, in *AddSerialRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	AddPrecertificate(ctx context.Context, in *AddCertificateRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	AddCertificate(ctx context.Context, in *AddCertificateRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	GetCertificate(ctx context.Context, in *Serial, opts ...grpc.CallOption) (*proto.Certificate, error)
	GetLintPrecertificate(ctx context.Context, in *Serial, opts ...grpc.CallOption) (*proto.Certificate, error)
	SetCertificateStatusReady(ctx context.Context, in *Serial, opts ...grpc.CallOption) (*emptypb.Empty, error)
}
