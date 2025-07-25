package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	berrors "github.com/letsencrypt/boulder/errors"
)

// wrapError wraps the internal error types we use for transport across the gRPC
// layer and appends an appropriate errortype to the gRPC trailer via the provided
// context. errors.BoulderError error types are encoded using the grpc/metadata
// in the context.Context for the RPC which is considered to be the 'proper'
// method of encoding custom error types (grpc/grpc#4543 and grpc/grpc-go#478)
func wrapError(ctx context.Context, appErr error) error {
	if appErr == nil {
		return nil
	}

	var berr *berrors.BoulderError
	if errors.As(appErr, &berr) {
		pairs := []string{
			"errortype", strconv.Itoa(int(berr.Type)),
		}

		// If there are suberrors then extend the metadata pairs to include the JSON
		// marshaling of the suberrors. Errors in marshaling are not ignored and
		// instead result in a return of an explicit InternalServerError and not
		// a wrapped error missing suberrors.
		if len(berr.SubErrors) > 0 {
			jsonSubErrs, err := json.Marshal(berr.SubErrors)
			if err != nil {
				return berrors.InternalServerError(
					"error marshaling json SubErrors, orig error %q", err)
			}
			headerSafeSubErrs := strconv.QuoteToASCII(string(jsonSubErrs))
			pairs = append(pairs, "suberrors", headerSafeSubErrs)
		}

		// If there is a RetryAfter value then extend the metadata pairs to
		// include the value.
		if berr.RetryAfter != 0 {
			pairs = append(pairs, "retryafter", berr.RetryAfter.String())
		}

		err := grpc.SetTrailer(ctx, metadata.Pairs(pairs...))
		if err != nil {
			return berrors.InternalServerError(
				"error setting gRPC error metadata, orig error %q", appErr)
		}
	}

	return appErr
}

// unwrapError unwraps errors returned from gRPC client calls which were wrapped
// with wrapError to their proper internal error type. If the provided metadata
// object has an "errortype" field, that will be used to set the type of the
// error.
func unwrapError(err error, md metadata.MD) error {
	if err == nil {
		return nil
	}

	errTypeStrs, ok := md["errortype"]
	if !ok {
		return err
	}

	inErrMsg := status.Convert(err).Message()
	if len(errTypeStrs) != 1 {
		return berrors.InternalServerError(
			"multiple 'errortype' metadata, wrapped error %q",
			inErrMsg,
		)
	}

	inErrType, decErr := strconv.Atoi(errTypeStrs[0])
	if decErr != nil {
		return berrors.InternalServerError(
			"failed to decode error type, decoding error %q, wrapped error %q",
			decErr,
			inErrMsg,
		)
	}
	inErr := berrors.New(berrors.ErrorType(inErrType), inErrMsg)
	var outErr *berrors.BoulderError
	if !errors.As(inErr, &outErr) {
		return fmt.Errorf(
			"expected type of inErr to be %T got %T: %q",
			outErr,
			inErr,
			inErr.Error(),
		)
	}

	subErrorsVal, ok := md["suberrors"]
	if ok {
		if len(subErrorsVal) != 1 {
			return berrors.InternalServerError(
				"multiple 'suberrors' in metadata, wrapped error %q",
				inErrMsg,
			)
		}

		unquotedSubErrors, unquoteErr := strconv.Unquote(subErrorsVal[0])
		if unquoteErr != nil {
			return fmt.Errorf(
				"unquoting 'suberrors' %q, wrapped error %q: %w",
				subErrorsVal[0],
				inErrMsg,
				unquoteErr,
			)
		}

		unmarshalErr := json.Unmarshal([]byte(unquotedSubErrors), &outErr.SubErrors)
		if unmarshalErr != nil {
			return berrors.InternalServerError(
				"JSON unmarshaling 'suberrors' %q, wrapped error %q: %s",
				subErrorsVal[0],
				inErrMsg,
				unmarshalErr,
			)
		}
	}

	retryAfterVal, ok := md["retryafter"]
	if ok {
		if len(retryAfterVal) != 1 {
			return berrors.InternalServerError(
				"multiple 'retryafter' in metadata, wrapped error %q",
				inErrMsg,
			)
		}
		var parseErr error
		outErr.RetryAfter, parseErr = time.ParseDuration(retryAfterVal[0])
		if parseErr != nil {
			return berrors.InternalServerError(
				"parsing 'retryafter' as int64, wrapped error %q, parsing error: %s",
				inErrMsg,
				parseErr,
			)
		}
	}
	return outErr
}
