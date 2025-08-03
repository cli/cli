// Package errors provide a special error type for use in Boulder. This error
// type carries additional type information with it, and has two special powers:
//
// 1. It is recognized by our gRPC code, and the type metadata and detail string
// will cross gRPC boundaries intact.
//
// 2. It is recognized by our frontend API "rendering" code, and will be
// automatically converted to the corresponding urn:ietf:params:acme:error:...
// ACME Problem Document.
//
// This means that a deeply-nested service (such as the SA) that wants to ensure
// that the ACME client sees a particular problem document (such as NotFound)
// can return a BoulderError and be sure that it will be propagated all the way
// to the client.
//
// Note, however, that any additional context wrapped *around* the BoulderError
// (such as by fmt.Errorf("oops: %w")) will be lost when the error is converted
// into a problem document. Similarly, any type information wrapped *by* a
// BoulderError (such as a sql.ErrNoRows) is lost at the gRPC serialization
// boundary.
package errors

import (
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/letsencrypt/boulder/identifier"
)

// ErrorType provides a coarse category for BoulderErrors.
// Objects of type ErrorType should never be directly returned by other
// functions; instead use the methods below to create an appropriate
// BoulderError wrapping one of these types.
type ErrorType int

// These numeric constants are used when sending berrors through gRPC.
const (
	// InternalServer is deprecated. Instead, pass a plain Go error. That will get
	// turned into a probs.InternalServerError by the WFE.
	InternalServer ErrorType = iota
	_                        // Reserved, previously NotSupported
	Malformed
	Unauthorized
	NotFound
	RateLimit
	RejectedIdentifier
	InvalidEmail
	ConnectionFailure
	_ // Reserved, previously WrongAuthorizationState
	CAA
	MissingSCTs
	Duplicate
	OrderNotReady
	DNS
	BadPublicKey
	BadCSR
	AlreadyRevoked
	BadRevocationReason
	UnsupportedContact
	// The requested serial number does not exist in the `serials` table.
	UnknownSerial
	Conflict
	// Defined in https://datatracker.ietf.org/doc/draft-aaron-acme-profiles/00/
	InvalidProfile
	// The certificate being indicated for replacement already has a replacement
	// order.
	AlreadyReplaced
	BadSignatureAlgorithm
	AccountDoesNotExist
	BadNonce
)

func (ErrorType) Error() string {
	return "urn:ietf:params:acme:error"
}

// BoulderError represents internal Boulder errors
type BoulderError struct {
	Type      ErrorType
	Detail    string
	SubErrors []SubBoulderError

	// RetryAfter the duration a client should wait before retrying the request
	// which resulted in this error.
	RetryAfter time.Duration
}

// SubBoulderError represents sub-errors specific to an identifier that are
// related to a top-level internal Boulder error.
type SubBoulderError struct {
	*BoulderError
	Identifier identifier.ACMEIdentifier
}

// Error implements the error interface, returning a string representation of
// this error.
func (be *BoulderError) Error() string {
	return be.Detail
}

// Unwrap implements the optional error-unwrapping interface. It returns the
// underlying type, all of when themselves implement the error interface, so
// that `if errors.Is(someError, berrors.Malformed)` works.
func (be *BoulderError) Unwrap() error {
	return be.Type
}

// GRPCStatus implements the interface implicitly defined by gRPC's
// status.FromError, which uses this function to detect if the error produced
// by the gRPC server implementation code is a gRPC status.Status. Implementing
// this means that BoulderErrors serialized in gRPC response metadata can be
// accompanied by a gRPC status other than "UNKNOWN".
func (be *BoulderError) GRPCStatus() *status.Status {
	var c codes.Code
	switch be.Type {
	case InternalServer:
		c = codes.Internal
	case Malformed:
		c = codes.InvalidArgument
	case Unauthorized:
		c = codes.PermissionDenied
	case NotFound:
		c = codes.NotFound
	case RateLimit:
		c = codes.Unknown
	case RejectedIdentifier:
		c = codes.InvalidArgument
	case InvalidEmail:
		c = codes.InvalidArgument
	case ConnectionFailure:
		c = codes.Unavailable
	case CAA:
		c = codes.FailedPrecondition
	case MissingSCTs:
		c = codes.Internal
	case Duplicate:
		c = codes.AlreadyExists
	case OrderNotReady:
		c = codes.FailedPrecondition
	case DNS:
		c = codes.Unknown
	case BadPublicKey:
		c = codes.InvalidArgument
	case BadCSR:
		c = codes.InvalidArgument
	case AlreadyRevoked:
		c = codes.AlreadyExists
	case BadRevocationReason:
		c = codes.InvalidArgument
	case UnsupportedContact:
		c = codes.InvalidArgument
	default:
		c = codes.Unknown
	}
	return status.New(c, be.Error())
}

// WithSubErrors returns a new BoulderError instance created by adding the
// provided subErrs to the existing BoulderError.
func (be *BoulderError) WithSubErrors(subErrs []SubBoulderError) *BoulderError {
	return &BoulderError{
		Type:       be.Type,
		Detail:     be.Detail,
		SubErrors:  append(be.SubErrors, subErrs...),
		RetryAfter: be.RetryAfter,
	}
}

// New is a convenience function for creating a new BoulderError.
func New(errType ErrorType, msg string) error {
	return &BoulderError{
		Type:   errType,
		Detail: msg,
	}
}

// newf is a convenience function for creating a new BoulderError with a
// formatted message.
func newf(errType ErrorType, msg string, args ...any) error {
	return &BoulderError{
		Type:   errType,
		Detail: fmt.Sprintf(msg, args...),
	}
}

func InternalServerError(msg string, args ...any) error {
	return newf(InternalServer, msg, args...)
}

func MalformedError(msg string, args ...any) error {
	return newf(Malformed, msg, args...)
}

func UnauthorizedError(msg string, args ...any) error {
	return newf(Unauthorized, msg, args...)
}

func NotFoundError(msg string, args ...any) error {
	return newf(NotFound, msg, args...)
}

func RateLimitError(retryAfter time.Duration, msg string, args ...any) error {
	return &BoulderError{
		Type:       RateLimit,
		Detail:     fmt.Sprintf(msg+": see https://letsencrypt.org/docs/rate-limits/", args...),
		RetryAfter: retryAfter,
	}
}

func RegistrationsPerIPAddressError(retryAfter time.Duration, msg string, args ...any) error {
	return &BoulderError{
		Type:       RateLimit,
		Detail:     fmt.Sprintf(msg+": see https://letsencrypt.org/docs/rate-limits/#new-registrations-per-ip-address", args...),
		RetryAfter: retryAfter,
	}
}

func RegistrationsPerIPv6RangeError(retryAfter time.Duration, msg string, args ...any) error {
	return &BoulderError{
		Type:       RateLimit,
		Detail:     fmt.Sprintf(msg+": see https://letsencrypt.org/docs/rate-limits/#new-registrations-per-ipv6-range", args...),
		RetryAfter: retryAfter,
	}
}

func NewOrdersPerAccountError(retryAfter time.Duration, msg string, args ...any) error {
	return &BoulderError{
		Type:       RateLimit,
		Detail:     fmt.Sprintf(msg+": see https://letsencrypt.org/docs/rate-limits/#new-orders-per-account", args...),
		RetryAfter: retryAfter,
	}
}

func CertificatesPerDomainError(retryAfter time.Duration, msg string, args ...any) error {
	return &BoulderError{
		Type:       RateLimit,
		Detail:     fmt.Sprintf(msg+": see https://letsencrypt.org/docs/rate-limits/#new-certificates-per-registered-domain", args...),
		RetryAfter: retryAfter,
	}
}

func CertificatesPerFQDNSetError(retryAfter time.Duration, msg string, args ...any) error {
	return &BoulderError{
		Type:       RateLimit,
		Detail:     fmt.Sprintf(msg+": see https://letsencrypt.org/docs/rate-limits/#new-certificates-per-exact-set-of-hostnames", args...),
		RetryAfter: retryAfter,
	}
}

func FailedAuthorizationsPerDomainPerAccountError(retryAfter time.Duration, msg string, args ...any) error {
	return &BoulderError{
		Type:       RateLimit,
		Detail:     fmt.Sprintf(msg+": see https://letsencrypt.org/docs/rate-limits/#authorization-failures-per-hostname-per-account", args...),
		RetryAfter: retryAfter,
	}
}

func RejectedIdentifierError(msg string, args ...any) error {
	return newf(RejectedIdentifier, msg, args...)
}

func InvalidEmailError(msg string, args ...any) error {
	return newf(InvalidEmail, msg, args...)
}

func UnsupportedContactError(msg string, args ...any) error {
	return newf(UnsupportedContact, msg, args...)
}

func ConnectionFailureError(msg string, args ...any) error {
	return newf(ConnectionFailure, msg, args...)
}

func CAAError(msg string, args ...any) error {
	return newf(CAA, msg, args...)
}

func MissingSCTsError(msg string, args ...any) error {
	return newf(MissingSCTs, msg, args...)
}

func DuplicateError(msg string, args ...any) error {
	return newf(Duplicate, msg, args...)
}

func OrderNotReadyError(msg string, args ...any) error {
	return newf(OrderNotReady, msg, args...)
}

func DNSError(msg string, args ...any) error {
	return newf(DNS, msg, args...)
}

func BadPublicKeyError(msg string, args ...any) error {
	return newf(BadPublicKey, msg, args...)
}

func BadCSRError(msg string, args ...any) error {
	return newf(BadCSR, msg, args...)
}

func AlreadyReplacedError(msg string, args ...any) error {
	return newf(AlreadyReplaced, msg, args...)
}

func AlreadyRevokedError(msg string, args ...any) error {
	return newf(AlreadyRevoked, msg, args...)
}

func BadRevocationReasonError(reason int64) error {
	return newf(BadRevocationReason, "disallowed revocation reason: %d", reason)
}

func UnknownSerialError() error {
	return newf(UnknownSerial, "unknown serial")
}

func InvalidProfileError(msg string, args ...any) error {
	return newf(InvalidProfile, msg, args...)
}

func BadSignatureAlgorithmError(msg string, args ...any) error {
	return newf(BadSignatureAlgorithm, msg, args...)
}

func AccountDoesNotExistError(msg string, args ...any) error {
	return newf(AccountDoesNotExist, msg, args...)
}

func BadNonceError(msg string, args ...any) error {
	return newf(BadNonce, msg, args...)
}
