// Package errors provides internal-facing error types for use in Boulder. Many
// of these are transformed directly into Problem Details documents by the WFE.
// Some, like NotFound, may be handled internally. We avoid using Problem
// Details documents as part of our internal error system to avoid layering
// confusions.
//
// These errors are specifically for use in errors that cross RPC boundaries.
// An error type that does not need to be passed through an RPC can use a plain
// Go type locally. Our gRPC code is aware of these error types and will
// serialize and deserialize them automatically.
package errors

import (
	"fmt"
	"time"

	"github.com/letsencrypt/boulder/identifier"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	_
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
	// The requesteed serial number does not exist in the `serials` table.
	UnknownSerial
	// The certificate being indicated for replacement already has a replacement
	// order.
	Conflict
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

func (be *BoulderError) Error() string {
	return be.Detail
}

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

// New is a convenience function for creating a new BoulderError
func New(errType ErrorType, msg string, args ...interface{}) error {
	return &BoulderError{
		Type:   errType,
		Detail: fmt.Sprintf(msg, args...),
	}
}

func InternalServerError(msg string, args ...interface{}) error {
	return New(InternalServer, msg, args...)
}

func MalformedError(msg string, args ...interface{}) error {
	return New(Malformed, msg, args...)
}

func UnauthorizedError(msg string, args ...interface{}) error {
	return New(Unauthorized, msg, args...)
}

func NotFoundError(msg string, args ...interface{}) error {
	return New(NotFound, msg, args...)
}

func RateLimitError(retryAfter time.Duration, msg string, args ...interface{}) error {
	return &BoulderError{
		Type:       RateLimit,
		Detail:     fmt.Sprintf(msg+": see https://letsencrypt.org/docs/rate-limits/", args...),
		RetryAfter: retryAfter,
	}
}

func DuplicateCertificateError(retryAfter time.Duration, msg string, args ...interface{}) error {
	return &BoulderError{
		Type:       RateLimit,
		Detail:     fmt.Sprintf(msg+": see https://letsencrypt.org/docs/duplicate-certificate-limit/", args...),
		RetryAfter: retryAfter,
	}
}

func FailedValidationError(retryAfter time.Duration, msg string, args ...interface{}) error {
	return &BoulderError{
		Type:       RateLimit,
		Detail:     fmt.Sprintf(msg+": see https://letsencrypt.org/docs/failed-validation-limit/", args...),
		RetryAfter: retryAfter,
	}
}

func RegistrationsPerIPError(retryAfter time.Duration, msg string, args ...interface{}) error {
	return &BoulderError{
		Type:       RateLimit,
		Detail:     fmt.Sprintf(msg+": see https://letsencrypt.org/docs/too-many-registrations-for-this-ip/", args...),
		RetryAfter: retryAfter,
	}
}

func RejectedIdentifierError(msg string, args ...interface{}) error {
	return New(RejectedIdentifier, msg, args...)
}

func InvalidEmailError(msg string, args ...interface{}) error {
	return New(InvalidEmail, msg, args...)
}

func UnsupportedContactError(msg string, args ...interface{}) error {
	return New(UnsupportedContact, msg, args...)
}

func ConnectionFailureError(msg string, args ...interface{}) error {
	return New(ConnectionFailure, msg, args...)
}

func CAAError(msg string, args ...interface{}) error {
	return New(CAA, msg, args...)
}

func MissingSCTsError(msg string, args ...interface{}) error {
	return New(MissingSCTs, msg, args...)
}

func DuplicateError(msg string, args ...interface{}) error {
	return New(Duplicate, msg, args...)
}

func OrderNotReadyError(msg string, args ...interface{}) error {
	return New(OrderNotReady, msg, args...)
}

func DNSError(msg string, args ...interface{}) error {
	return New(DNS, msg, args...)
}

func BadPublicKeyError(msg string, args ...interface{}) error {
	return New(BadPublicKey, msg, args...)
}

func BadCSRError(msg string, args ...interface{}) error {
	return New(BadCSR, msg, args...)
}

func AlreadyRevokedError(msg string, args ...interface{}) error {
	return New(AlreadyRevoked, msg, args...)
}

func BadRevocationReasonError(reason int64) error {
	return New(BadRevocationReason, "disallowed revocation reason: %d", reason)
}

func UnknownSerialError() error {
	return New(UnknownSerial, "unknown serial")
}

func ConflictError(msg string, args ...interface{}) error {
	return New(Conflict, msg, args...)
}
