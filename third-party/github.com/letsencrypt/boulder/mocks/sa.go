package mocks

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"math/rand/v2"
	"os"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/jmhodges/clock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/letsencrypt/boulder/core"
	corepb "github.com/letsencrypt/boulder/core/proto"
	berrors "github.com/letsencrypt/boulder/errors"
	bgrpc "github.com/letsencrypt/boulder/grpc"
	"github.com/letsencrypt/boulder/identifier"
	sapb "github.com/letsencrypt/boulder/sa/proto"
)

// StorageAuthorityReadOnly is a mock of sapb.StorageAuthorityReadOnlyClient
type StorageAuthorityReadOnly struct {
	clk clock.Clock
}

// NewStorageAuthorityReadOnly creates a new mock read-only storage authority
// with the given clock.
func NewStorageAuthorityReadOnly(clk clock.Clock) *StorageAuthorityReadOnly {
	return &StorageAuthorityReadOnly{clk}
}

// StorageAuthority is a mock of sapb.StorageAuthorityClient
type StorageAuthority struct {
	StorageAuthorityReadOnly
}

// NewStorageAuthority creates a new mock storage authority
// with the given clock.
func NewStorageAuthority(clk clock.Clock) *StorageAuthority {
	return &StorageAuthority{StorageAuthorityReadOnly{clk}}
}

const (
	test1KeyPublicJSON  = `{"kty":"RSA","n":"yNWVhtYEKJR21y9xsHV-PD_bYwbXSeNuFal46xYxVfRL5mqha7vttvjB_vc7Xg2RvgCxHPCqoxgMPTzHrZT75LjCwIW2K_klBYN8oYvTwwmeSkAz6ut7ZxPv-nZaT5TJhGk0NT2kh_zSpdriEJ_3vW-mqxYbbBmpvHqsa1_zx9fSuHYctAZJWzxzUZXykbWMWQZpEiE0J4ajj51fInEzVn7VxV-mzfMyboQjujPh7aNJxAWSq4oQEJJDgWwSh9leyoJoPpONHxh5nEE5AjE01FkGICSxjpZsF-w8hOTI3XXohUdu29Se26k2B0PolDSuj0GIQU6-W9TdLXSjBb2SpQ","e":"AQAB"}`
	test2KeyPublicJSON  = `{"kty":"RSA","n":"qnARLrT7Xz4gRcKyLdydmCr-ey9OuPImX4X40thk3on26FkMznR3fRjs66eLK7mmPcBZ6uOJseURU6wAaZNmemoYx1dMvqvWWIyiQleHSD7Q8vBrhR6uIoO4jAzJZR-ChzZuSDt7iHN-3xUVspu5XGwXU_MVJZshTwp4TaFx5elHIT_ObnTvTOU3Xhish07AbgZKmWsVbXh5s-CrIicU4OexJPgunWZ_YJJueOKmTvnLlTV4MzKR2oZlBKZ27S0-SfdV_QDx_ydle5oMAyKVtlAV35cyPMIsYNwgUGBCdY_2Uzi5eX0lTc7MPRwz6qR1kip-i59VcGcUQgqHV6Fyqw","e":"AQAB"}`
	testE1KeyPublicJSON = `{"kty":"EC","crv":"P-256","x":"FwvSZpu06i3frSk_mz9HcD9nETn4wf3mQ-zDtG21Gao","y":"S8rR-0dWa8nAcw1fbunF_ajS3PQZ-QwLps-2adgLgPk"}`
	testE2KeyPublicJSON = `{"kty":"EC","crv":"P-256","x":"S8FOmrZ3ywj4yyFqt0etAD90U-EnkNaOBSLfQmf7pNg","y":"vMvpDyqFDRHjGfZ1siDOm5LS6xNdR5xTpyoQGLDOX2Q"}`
	test3KeyPublicJSON  = `{"kty":"RSA","n":"uTQER6vUA1RDixS8xsfCRiKUNGRzzyIK0MhbS2biClShbb0hSx2mPP7gBvis2lizZ9r-y9hL57kNQoYCKndOBg0FYsHzrQ3O9AcoV1z2Mq-XhHZbFrVYaXI0M3oY9BJCWog0dyi3XC0x8AxC1npd1U61cToHx-3uSvgZOuQA5ffEn5L38Dz1Ti7OV3E4XahnRJvejadUmTkki7phLBUXm5MnnyFm0CPpf6ApV7zhLjN5W-nV0WL17o7v8aDgV_t9nIdi1Y26c3PlCEtiVHZcebDH5F1Deta3oLLg9-g6rWnTqPbY3knffhp4m0scLD6e33k8MtzxDX_D7vHsg0_X1w","e":"AQAB"}`
	test4KeyPublicJSON  = `{"kty":"RSA","n":"qih-cx32M0wq8MhhN-kBi2xPE-wnw4_iIg1hWO5wtBfpt2PtWikgPuBT6jvK9oyQwAWbSfwqlVZatMPY_-3IyytMNb9R9OatNr6o5HROBoyZnDVSiC4iMRd7bRl_PWSIqj_MjhPNa9cYwBdW5iC3jM5TaOgmp0-YFm4tkLGirDcIBDkQYlnv9NKILvuwqkapZ7XBixeqdCcikUcTRXW5unqygO6bnapzw-YtPsPPlj4Ih3SvK4doyziPV96U8u5lbNYYEzYiW1mbu9n0KLvmKDikGcdOpf6-yRa_10kMZyYQatY1eclIKI0xb54kbluEl0GQDaL5FxLmiKeVnsapzw","e":"AQAB"}`

	agreementURL = "http://example.invalid/terms"
)

// GetRegistration is a mock
func (sa *StorageAuthorityReadOnly) GetRegistration(_ context.Context, req *sapb.RegistrationID, _ ...grpc.CallOption) (*corepb.Registration, error) {
	if req.Id == 100 {
		// Tag meaning "Missing"
		return nil, errors.New("missing")
	}
	if req.Id == 101 {
		// Tag meaning "Malformed"
		return &corepb.Registration{}, nil
	}
	if req.Id == 102 {
		// Tag meaning "Not Found"
		return nil, berrors.NotFoundError("Dave's not here man")
	}

	goodReg := &corepb.Registration{
		Id:        req.Id,
		Key:       []byte(test1KeyPublicJSON),
		Agreement: agreementURL,
		Contact:   []string{"mailto:person@mail.com"},
		Status:    string(core.StatusValid),
	}

	// Return a populated registration with contacts for ID == 1 or ID == 5
	if req.Id == 1 || req.Id == 5 {
		return goodReg, nil
	}

	// Return a populated registration with a different key for ID == 2
	if req.Id == 2 {
		goodReg.Key = []byte(test2KeyPublicJSON)
		return goodReg, nil
	}

	// Return a deactivated registration with a different key for ID == 3
	if req.Id == 3 {
		goodReg.Key = []byte(test3KeyPublicJSON)
		goodReg.Status = string(core.StatusDeactivated)
		return goodReg, nil
	}

	// Return a populated registration with a different key for ID == 4
	if req.Id == 4 {
		goodReg.Key = []byte(test4KeyPublicJSON)
		return goodReg, nil
	}

	// Return a registration without the agreement set for ID == 6
	if req.Id == 6 {
		goodReg.Agreement = ""
		return goodReg, nil
	}

	goodReg.CreatedAt = timestamppb.New(time.Date(2003, 9, 27, 0, 0, 0, 0, time.UTC))
	return goodReg, nil
}

// GetRegistrationByKey is a mock
func (sa *StorageAuthorityReadOnly) GetRegistrationByKey(_ context.Context, req *sapb.JSONWebKey, _ ...grpc.CallOption) (*corepb.Registration, error) {
	test5KeyBytes, err := os.ReadFile("../test/test-key-5.der")
	if err != nil {
		return nil, err
	}
	test5KeyPriv, err := x509.ParsePKCS1PrivateKey(test5KeyBytes)
	if err != nil {
		return nil, err
	}
	test5KeyPublic := jose.JSONWebKey{Key: test5KeyPriv.Public()}
	test5KeyPublicJSON, err := test5KeyPublic.MarshalJSON()
	if err != nil {
		return nil, err
	}

	contacts := []string{"mailto:person@mail.com"}

	if bytes.Equal(req.Jwk, []byte(test1KeyPublicJSON)) {
		return &corepb.Registration{
			Id:        1,
			Key:       req.Jwk,
			Agreement: agreementURL,
			Contact:   contacts,
			Status:    string(core.StatusValid),
		}, nil
	}

	if bytes.Equal(req.Jwk, []byte(test2KeyPublicJSON)) {
		// No key found
		return &corepb.Registration{Id: 2}, berrors.NotFoundError("reg not found")
	}

	if bytes.Equal(req.Jwk, []byte(test4KeyPublicJSON)) {
		// No key found
		return &corepb.Registration{Id: 5}, berrors.NotFoundError("reg not found")
	}

	if bytes.Equal(req.Jwk, test5KeyPublicJSON) {
		// No key found
		return &corepb.Registration{Id: 5}, berrors.NotFoundError("reg not found")
	}

	if bytes.Equal(req.Jwk, []byte(testE1KeyPublicJSON)) {
		return &corepb.Registration{Id: 3, Key: req.Jwk, Agreement: agreementURL}, nil
	}

	if bytes.Equal(req.Jwk, []byte(testE2KeyPublicJSON)) {
		return &corepb.Registration{Id: 4}, berrors.NotFoundError("reg not found")
	}

	if bytes.Equal(req.Jwk, []byte(test3KeyPublicJSON)) {
		// deactivated registration
		return &corepb.Registration{
			Id:        2,
			Key:       req.Jwk,
			Agreement: agreementURL,
			Contact:   contacts,
			Status:    string(core.StatusDeactivated),
		}, nil
	}

	// Return a fake registration. Make sure to fill the key field to avoid marshaling errors.
	return &corepb.Registration{
		Id:        1,
		Key:       []byte(test1KeyPublicJSON),
		Agreement: agreementURL,
		Status:    string(core.StatusValid),
	}, nil
}

// GetSerialMetadata is a mock
func (sa *StorageAuthorityReadOnly) GetSerialMetadata(ctx context.Context, req *sapb.Serial, _ ...grpc.CallOption) (*sapb.SerialMetadata, error) {
	now := sa.clk.Now()
	created := now.Add(-1 * time.Hour)
	expires := now.Add(2159 * time.Hour)
	return &sapb.SerialMetadata{
		Serial:         req.Serial,
		RegistrationID: 1,
		Created:        timestamppb.New(created),
		Expires:        timestamppb.New(expires),
	}, nil
}

// GetCertificate is a mock
func (sa *StorageAuthorityReadOnly) GetCertificate(_ context.Context, req *sapb.Serial, _ ...grpc.CallOption) (*corepb.Certificate, error) {
	if req.Serial == "000000000000000000000000000000626164" {
		return nil, errors.New("bad")
	} else {
		return nil, berrors.NotFoundError("No cert")
	}
}

// GetLintPrecertificate is a mock
func (sa *StorageAuthorityReadOnly) GetLintPrecertificate(_ context.Context, req *sapb.Serial, _ ...grpc.CallOption) (*corepb.Certificate, error) {
	return nil, berrors.NotFoundError("No cert")
}

// GetCertificateStatus is a mock
func (sa *StorageAuthorityReadOnly) GetCertificateStatus(_ context.Context, req *sapb.Serial, _ ...grpc.CallOption) (*corepb.CertificateStatus, error) {
	return nil, errors.New("no cert status")
}

func (sa *StorageAuthorityReadOnly) SetCertificateStatusReady(ctx context.Context, req *sapb.Serial, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unimplemented, "unimplemented mock")
}

// GetRevocationStatus is a mock
func (sa *StorageAuthorityReadOnly) GetRevocationStatus(_ context.Context, req *sapb.Serial, _ ...grpc.CallOption) (*sapb.RevocationStatus, error) {
	return nil, nil
}

// SerialsForIncident is a mock
func (sa *StorageAuthorityReadOnly) SerialsForIncident(ctx context.Context, _ *sapb.SerialsForIncidentRequest, _ ...grpc.CallOption) (sapb.StorageAuthorityReadOnly_SerialsForIncidentClient, error) {
	return &ServerStreamClient[sapb.IncidentSerial]{}, nil
}

// SerialsForIncident is a mock
func (sa *StorageAuthority) SerialsForIncident(ctx context.Context, _ *sapb.SerialsForIncidentRequest, _ ...grpc.CallOption) (sapb.StorageAuthority_SerialsForIncidentClient, error) {
	return &ServerStreamClient[sapb.IncidentSerial]{}, nil
}

// CheckIdentifiersPaused is a mock
func (sa *StorageAuthorityReadOnly) CheckIdentifiersPaused(_ context.Context, _ *sapb.PauseRequest, _ ...grpc.CallOption) (*sapb.Identifiers, error) {
	return nil, nil
}

// CheckIdentifiersPaused is a mock
func (sa *StorageAuthority) CheckIdentifiersPaused(_ context.Context, _ *sapb.PauseRequest, _ ...grpc.CallOption) (*sapb.Identifiers, error) {
	return nil, nil
}

// GetPausedIdentifiers is a mock
func (sa *StorageAuthorityReadOnly) GetPausedIdentifiers(_ context.Context, _ *sapb.RegistrationID, _ ...grpc.CallOption) (*sapb.Identifiers, error) {
	return nil, nil
}

// GetPausedIdentifiers is a mock
func (sa *StorageAuthority) GetPausedIdentifiers(_ context.Context, _ *sapb.RegistrationID, _ ...grpc.CallOption) (*sapb.Identifiers, error) {
	return nil, nil
}

// GetRevokedCerts is a mock
func (sa *StorageAuthorityReadOnly) GetRevokedCerts(ctx context.Context, _ *sapb.GetRevokedCertsRequest, _ ...grpc.CallOption) (sapb.StorageAuthorityReadOnly_GetRevokedCertsClient, error) {
	return &ServerStreamClient[corepb.CRLEntry]{}, nil
}

// GetRevokedCerts is a mock
func (sa *StorageAuthority) GetRevokedCerts(ctx context.Context, _ *sapb.GetRevokedCertsRequest, _ ...grpc.CallOption) (sapb.StorageAuthority_GetRevokedCertsClient, error) {
	return &ServerStreamClient[corepb.CRLEntry]{}, nil
}

// GetRevokedCertsByShard is a mock
func (sa *StorageAuthorityReadOnly) GetRevokedCertsByShard(ctx context.Context, _ *sapb.GetRevokedCertsByShardRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[corepb.CRLEntry], error) {
	return &ServerStreamClient[corepb.CRLEntry]{}, nil
}

// GetMaxExpiration is a mock
func (sa *StorageAuthorityReadOnly) GetMaxExpiration(_ context.Context, req *emptypb.Empty, _ ...grpc.CallOption) (*timestamppb.Timestamp, error) {
	return nil, nil
}

// AddRateLimitOverride is a mock
func (sa *StorageAuthority) AddRateLimitOverride(_ context.Context, req *sapb.AddRateLimitOverrideRequest, _ ...grpc.CallOption) (*sapb.AddRateLimitOverrideResponse, error) {
	return nil, nil
}

// DisableRateLimitOverride is a mock
func (sa *StorageAuthority) DisableRateLimitOverride(ctx context.Context, req *sapb.DisableRateLimitOverrideRequest) (*emptypb.Empty, error) {
	return nil, nil
}

// EnableRateLimitOverride is a mock
func (sa *StorageAuthority) EnableRateLimitOverride(ctx context.Context, req *sapb.EnableRateLimitOverrideRequest) (*emptypb.Empty, error) {
	return nil, nil
}

// GetRateLimitOverride is a mock
func (sa *StorageAuthorityReadOnly) GetRateLimitOverride(_ context.Context, req *sapb.GetRateLimitOverrideRequest, _ ...grpc.CallOption) (*sapb.RateLimitOverrideResponse, error) {
	return nil, nil
}

// GetEnabledRateLimitOverrides is a mock
func (sa *StorageAuthorityReadOnly) GetEnabledRateLimitOverrides(_ context.Context, _ *emptypb.Empty, _ ...grpc.CallOption) (sapb.StorageAuthorityReadOnly_GetEnabledRateLimitOverridesClient, error) {
	return nil, nil
}

// AddPrecertificate is a mock
func (sa *StorageAuthority) AddPrecertificate(ctx context.Context, req *sapb.AddCertificateRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}

// AddSerial is a mock
func (sa *StorageAuthority) AddSerial(ctx context.Context, req *sapb.AddSerialRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}

// AddCertificate is a mock
func (sa *StorageAuthority) AddCertificate(_ context.Context, _ *sapb.AddCertificateRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}

// NewRegistration is a mock
func (sa *StorageAuthority) NewRegistration(_ context.Context, _ *corepb.Registration, _ ...grpc.CallOption) (*corepb.Registration, error) {
	return &corepb.Registration{}, nil
}

// UpdateRegistration is a mock
func (sa *StorageAuthority) UpdateRegistration(_ context.Context, _ *corepb.Registration, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

// FQDNSetTimestampsForWindow is a mock
func (sa *StorageAuthorityReadOnly) FQDNSetTimestampsForWindow(_ context.Context, _ *sapb.CountFQDNSetsRequest, _ ...grpc.CallOption) (*sapb.Timestamps, error) {
	return &sapb.Timestamps{}, nil
}

// FQDNSetExists is a mock
func (sa *StorageAuthorityReadOnly) FQDNSetExists(_ context.Context, _ *sapb.FQDNSetExistsRequest, _ ...grpc.CallOption) (*sapb.Exists, error) {
	return &sapb.Exists{Exists: false}, nil
}

// DeactivateRegistration is a mock
func (sa *StorageAuthority) DeactivateRegistration(_ context.Context, _ *sapb.RegistrationID, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

// NewOrderAndAuthzs is a mock
func (sa *StorageAuthority) NewOrderAndAuthzs(_ context.Context, req *sapb.NewOrderAndAuthzsRequest, _ ...grpc.CallOption) (*corepb.Order, error) {
	response := &corepb.Order{
		// Fields from the input new order request.
		RegistrationID:   req.NewOrder.RegistrationID,
		Expires:          req.NewOrder.Expires,
		Identifiers:      req.NewOrder.Identifiers,
		V2Authorizations: req.NewOrder.V2Authorizations,
		// Mock new fields generated by the database transaction.
		Id:      rand.Int64(),
		Created: timestamppb.Now(),
		// A new order is never processing because it can't have been finalized yet.
		BeganProcessing:        false,
		Status:                 string(core.StatusPending),
		CertificateProfileName: req.NewOrder.CertificateProfileName,
	}
	return response, nil
}

// SetOrderProcessing is a mock
func (sa *StorageAuthority) SetOrderProcessing(_ context.Context, req *sapb.OrderRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

// SetOrderError is a mock
func (sa *StorageAuthority) SetOrderError(_ context.Context, req *sapb.SetOrderErrorRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

// FinalizeOrder is a mock
func (sa *StorageAuthority) FinalizeOrder(_ context.Context, req *sapb.FinalizeOrderRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

// GetOrder is a mock
func (sa *StorageAuthorityReadOnly) GetOrder(_ context.Context, req *sapb.OrderRequest, _ ...grpc.CallOption) (*corepb.Order, error) {
	if req.Id == 2 {
		return nil, berrors.NotFoundError("bad")
	} else if req.Id == 3 {
		return nil, errors.New("very bad")
	}

	now := sa.clk.Now()
	created := now.AddDate(-30, 0, 0)
	exp := now.AddDate(30, 0, 0)
	validOrder := &corepb.Order{
		Id:                     req.Id,
		RegistrationID:         1,
		Created:                timestamppb.New(created),
		Expires:                timestamppb.New(exp),
		Identifiers:            []*corepb.Identifier{identifier.NewDNS("example.com").ToProto()},
		Status:                 string(core.StatusValid),
		V2Authorizations:       []int64{1},
		CertificateSerial:      "serial",
		Error:                  nil,
		CertificateProfileName: "default",
	}

	// Order ID doesn't have a certificate serial yet
	if req.Id == 4 {
		validOrder.Status = string(core.StatusPending)
		validOrder.Id = req.Id
		validOrder.CertificateSerial = ""
		validOrder.Error = nil
		return validOrder, nil
	}

	// Order ID 6 belongs to reg ID 6
	if req.Id == 6 {
		validOrder.Id = 6
		validOrder.RegistrationID = 6
	}

	// Order ID 7 is ready, but expired
	if req.Id == 7 {
		validOrder.Status = string(core.StatusReady)
		validOrder.Expires = timestamppb.New(now.AddDate(-30, 0, 0))
	}

	if req.Id == 8 {
		validOrder.Status = string(core.StatusReady)
	}

	// Order 9 is fresh
	if req.Id == 9 {
		validOrder.Created = timestamppb.New(now.AddDate(0, 0, 1))
	}

	// Order 10 is processing
	if req.Id == 10 {
		validOrder.Status = string(core.StatusProcessing)
	}

	return validOrder, nil
}

func (sa *StorageAuthorityReadOnly) GetOrderForNames(_ context.Context, _ *sapb.GetOrderForNamesRequest, _ ...grpc.CallOption) (*corepb.Order, error) {
	return nil, nil
}

func (sa *StorageAuthority) FinalizeAuthorization2(ctx context.Context, req *sapb.FinalizeAuthorizationRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (sa *StorageAuthority) DeactivateAuthorization2(ctx context.Context, req *sapb.AuthorizationID2, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}

func (sa *StorageAuthorityReadOnly) CountPendingAuthorizations2(ctx context.Context, req *sapb.RegistrationID, _ ...grpc.CallOption) (*sapb.Count, error) {
	return &sapb.Count{}, nil
}

func (sa *StorageAuthorityReadOnly) GetValidOrderAuthorizations2(ctx context.Context, req *sapb.GetValidOrderAuthorizationsRequest, _ ...grpc.CallOption) (*sapb.Authorizations, error) {
	return nil, nil
}

func (sa *StorageAuthorityReadOnly) CountInvalidAuthorizations2(ctx context.Context, req *sapb.CountInvalidAuthorizationsRequest, _ ...grpc.CallOption) (*sapb.Count, error) {
	return &sapb.Count{}, nil
}

func (sa *StorageAuthorityReadOnly) GetValidAuthorizations2(ctx context.Context, req *sapb.GetValidAuthorizationsRequest, _ ...grpc.CallOption) (*sapb.Authorizations, error) {
	if req.RegistrationID != 1 && req.RegistrationID != 5 && req.RegistrationID != 4 {
		return &sapb.Authorizations{}, nil
	}
	expiryCutoff := req.ValidUntil.AsTime()
	auths := &sapb.Authorizations{}
	for _, ident := range req.Identifiers {
		exp := expiryCutoff.AddDate(100, 0, 0)
		authzPB, err := bgrpc.AuthzToPB(core.Authorization{
			Status:         core.StatusValid,
			RegistrationID: req.RegistrationID,
			Expires:        &exp,
			Identifier:     identifier.FromProto(ident),
			Challenges: []core.Challenge{
				{
					Status:    core.StatusValid,
					Type:      core.ChallengeTypeDNS01,
					Token:     "exampleToken",
					Validated: &expiryCutoff,
				},
			},
		})
		if err != nil {
			return nil, err
		}
		auths.Authzs = append(auths.Authzs, authzPB)
	}
	return auths, nil
}

func (sa *StorageAuthorityReadOnly) GetAuthorizations2(ctx context.Context, req *sapb.GetAuthorizationsRequest, _ ...grpc.CallOption) (*sapb.Authorizations, error) {
	return &sapb.Authorizations{}, nil
}

// GetAuthorization2 is a mock
func (sa *StorageAuthorityReadOnly) GetAuthorization2(ctx context.Context, id *sapb.AuthorizationID2, _ ...grpc.CallOption) (*corepb.Authorization, error) {
	return &corepb.Authorization{}, nil
}

// GetSerialsByKey is a mock
func (sa *StorageAuthorityReadOnly) GetSerialsByKey(ctx context.Context, _ *sapb.SPKIHash, _ ...grpc.CallOption) (sapb.StorageAuthorityReadOnly_GetSerialsByKeyClient, error) {
	return &ServerStreamClient[sapb.Serial]{}, nil
}

// GetSerialsByKey is a mock
func (sa *StorageAuthority) GetSerialsByKey(ctx context.Context, _ *sapb.SPKIHash, _ ...grpc.CallOption) (sapb.StorageAuthority_GetSerialsByKeyClient, error) {
	return &ServerStreamClient[sapb.Serial]{}, nil
}

// GetSerialsByAccount is a mock
func (sa *StorageAuthorityReadOnly) GetSerialsByAccount(ctx context.Context, _ *sapb.RegistrationID, _ ...grpc.CallOption) (sapb.StorageAuthorityReadOnly_GetSerialsByAccountClient, error) {
	return &ServerStreamClient[sapb.Serial]{}, nil
}

// GetSerialsByAccount is a mock
func (sa *StorageAuthority) GetSerialsByAccount(ctx context.Context, _ *sapb.RegistrationID, _ ...grpc.CallOption) (sapb.StorageAuthority_GetSerialsByAccountClient, error) {
	return &ServerStreamClient[sapb.Serial]{}, nil
}

// RevokeCertificate is a mock
func (sa *StorageAuthority) RevokeCertificate(ctx context.Context, req *sapb.RevokeCertificateRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}

// UpdateRevokedCertificate is a mock
func (sa *StorageAuthority) UpdateRevokedCertificate(ctx context.Context, req *sapb.RevokeCertificateRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}

// AddBlockedKey is a mock
func (sa *StorageAuthority) AddBlockedKey(ctx context.Context, req *sapb.AddBlockedKeyRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

// KeyBlocked is a mock
func (sa *StorageAuthorityReadOnly) KeyBlocked(ctx context.Context, req *sapb.SPKIHash, _ ...grpc.CallOption) (*sapb.Exists, error) {
	return &sapb.Exists{Exists: false}, nil
}

// IncidentsForSerial is a mock.
func (sa *StorageAuthorityReadOnly) IncidentsForSerial(ctx context.Context, req *sapb.Serial, _ ...grpc.CallOption) (*sapb.Incidents, error) {
	return &sapb.Incidents{}, nil
}

// LeaseCRLShard is a mock.
func (sa *StorageAuthority) LeaseCRLShard(ctx context.Context, req *sapb.LeaseCRLShardRequest, _ ...grpc.CallOption) (*sapb.LeaseCRLShardResponse, error) {
	return nil, errors.New("unimplemented")
}

// UpdateCRLShard is a mock.
func (sa *StorageAuthority) UpdateCRLShard(ctx context.Context, req *sapb.UpdateCRLShardRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, errors.New("unimplemented")
}

// ReplacementOrderExists is a mock.
func (sa *StorageAuthorityReadOnly) ReplacementOrderExists(ctx context.Context, req *sapb.Serial, _ ...grpc.CallOption) (*sapb.Exists, error) {
	return nil, nil
}
