package sa

import (
	"context"
	"io"

	corepb "github.com/letsencrypt/boulder/core/proto"
	"github.com/letsencrypt/boulder/sa"
	sapb "github.com/letsencrypt/boulder/sa/proto"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

// SA meets the `sapb.StorageAuthorityClient` interface and acts as a
// wrapper for an inner `sa.SQLStorageAuthority` (which in turn meets
// the `sapb.StorageAuthorityServer` interface). Only methods used by
// unit tests need to be implemented.
type SA struct {
	sapb.StorageAuthorityClient
	Impl *sa.SQLStorageAuthority
}

func (sa SA) NewRegistration(ctx context.Context, req *corepb.Registration, _ ...grpc.CallOption) (*corepb.Registration, error) {
	return sa.Impl.NewRegistration(ctx, req)
}

func (sa SA) GetRegistration(ctx context.Context, req *sapb.RegistrationID, _ ...grpc.CallOption) (*corepb.Registration, error) {
	return sa.Impl.GetRegistration(ctx, req)
}

func (sa SA) DeactivateRegistration(ctx context.Context, req *sapb.RegistrationID, _ ...grpc.CallOption) (*corepb.Registration, error) {
	return sa.Impl.DeactivateRegistration(ctx, req)
}

func (sa SA) GetAuthorization2(ctx context.Context, req *sapb.AuthorizationID2, _ ...grpc.CallOption) (*corepb.Authorization, error) {
	return sa.Impl.GetAuthorization2(ctx, req)
}

func (sa SA) GetAuthorizations2(ctx context.Context, req *sapb.GetAuthorizationsRequest, _ ...grpc.CallOption) (*sapb.Authorizations, error) {
	return sa.Impl.GetAuthorizations2(ctx, req)
}

func (sa SA) GetValidAuthorizations2(ctx context.Context, req *sapb.GetValidAuthorizationsRequest, _ ...grpc.CallOption) (*sapb.Authorizations, error) {
	return sa.Impl.GetValidAuthorizations2(ctx, req)
}

func (sa SA) GetValidOrderAuthorizations2(ctx context.Context, req *sapb.GetValidOrderAuthorizationsRequest, _ ...grpc.CallOption) (*sapb.Authorizations, error) {
	return sa.Impl.GetValidOrderAuthorizations2(ctx, req)
}

func (sa SA) CountPendingAuthorizations2(ctx context.Context, req *sapb.RegistrationID, _ ...grpc.CallOption) (*sapb.Count, error) {
	return sa.Impl.CountPendingAuthorizations2(ctx, req)
}

func (sa SA) DeactivateAuthorization2(ctx context.Context, req *sapb.AuthorizationID2, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return sa.Impl.DeactivateAuthorization2(ctx, req)
}

func (sa SA) FinalizeAuthorization2(ctx context.Context, req *sapb.FinalizeAuthorizationRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return sa.Impl.FinalizeAuthorization2(ctx, req)
}

func (sa SA) NewOrderAndAuthzs(ctx context.Context, req *sapb.NewOrderAndAuthzsRequest, _ ...grpc.CallOption) (*corepb.Order, error) {
	return sa.Impl.NewOrderAndAuthzs(ctx, req)
}

func (sa SA) GetOrder(ctx context.Context, req *sapb.OrderRequest, _ ...grpc.CallOption) (*corepb.Order, error) {
	return sa.Impl.GetOrder(ctx, req)
}

func (sa SA) GetOrderForNames(ctx context.Context, req *sapb.GetOrderForNamesRequest, _ ...grpc.CallOption) (*corepb.Order, error) {
	return sa.Impl.GetOrderForNames(ctx, req)
}

func (sa SA) SetOrderError(ctx context.Context, req *sapb.SetOrderErrorRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return sa.Impl.SetOrderError(ctx, req)
}

func (sa SA) SetOrderProcessing(ctx context.Context, req *sapb.OrderRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return sa.Impl.SetOrderProcessing(ctx, req)
}

func (sa SA) FinalizeOrder(ctx context.Context, req *sapb.FinalizeOrderRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return sa.Impl.FinalizeOrder(ctx, req)
}

func (sa SA) AddPrecertificate(ctx context.Context, req *sapb.AddCertificateRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return sa.Impl.AddPrecertificate(ctx, req)
}

func (sa SA) AddCertificate(ctx context.Context, req *sapb.AddCertificateRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return sa.Impl.AddCertificate(ctx, req)
}

func (sa SA) RevokeCertificate(ctx context.Context, req *sapb.RevokeCertificateRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return sa.Impl.RevokeCertificate(ctx, req)
}

func (sa SA) GetLintPrecertificate(ctx context.Context, req *sapb.Serial, _ ...grpc.CallOption) (*corepb.Certificate, error) {
	return sa.Impl.GetLintPrecertificate(ctx, req)
}

func (sa SA) GetCertificateStatus(ctx context.Context, req *sapb.Serial, _ ...grpc.CallOption) (*corepb.CertificateStatus, error) {
	return sa.Impl.GetCertificateStatus(ctx, req)
}

func (sa SA) AddBlockedKey(ctx context.Context, req *sapb.AddBlockedKeyRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return sa.Impl.AddBlockedKey(ctx, req)
}

func (sa SA) FQDNSetExists(ctx context.Context, req *sapb.FQDNSetExistsRequest, _ ...grpc.CallOption) (*sapb.Exists, error) {
	return sa.Impl.FQDNSetExists(ctx, req)
}

func (sa SA) FQDNSetTimestampsForWindow(ctx context.Context, req *sapb.CountFQDNSetsRequest, _ ...grpc.CallOption) (*sapb.Timestamps, error) {
	return sa.Impl.FQDNSetTimestampsForWindow(ctx, req)
}

func (sa SA) PauseIdentifiers(ctx context.Context, req *sapb.PauseRequest, _ ...grpc.CallOption) (*sapb.PauseIdentifiersResponse, error) {
	return sa.Impl.PauseIdentifiers(ctx, req)
}

type mockStreamResult[T any] struct {
	val T
	err error
}

type mockClientStream[T any] struct {
	grpc.ClientStream
	stream <-chan mockStreamResult[T]
}

func (c mockClientStream[T]) Recv() (T, error) {
	result := <-c.stream
	return result.val, result.err
}

type mockServerStream[T any] struct {
	grpc.ServerStream
	context context.Context
	stream  chan<- mockStreamResult[T]
}

func (s mockServerStream[T]) Send(val T) error {
	s.stream <- mockStreamResult[T]{val: val, err: nil}
	return nil
}

func (s mockServerStream[T]) Context() context.Context {
	return s.context
}

func (sa SA) SerialsForIncident(ctx context.Context, req *sapb.SerialsForIncidentRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[sapb.IncidentSerial], error) {
	streamChan := make(chan mockStreamResult[*sapb.IncidentSerial])
	client := mockClientStream[*sapb.IncidentSerial]{stream: streamChan}
	server := mockServerStream[*sapb.IncidentSerial]{context: ctx, stream: streamChan}
	go func() {
		err := sa.Impl.SerialsForIncident(req, server)
		if err != nil {
			streamChan <- mockStreamResult[*sapb.IncidentSerial]{nil, err}
		}
		streamChan <- mockStreamResult[*sapb.IncidentSerial]{nil, io.EOF}
		close(streamChan)
	}()
	return client, nil
}
