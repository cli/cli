package canceled

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Is returns true if err is non-nil and is either context.Canceled, or has a
// grpc code of Canceled. This is useful because cancellations propagate through
// gRPC boundaries, and if we choose to treat in-process cancellations a certain
// way, we usually want to treat cross-process cancellations the same way.
func Is(err error) bool {
	return err == context.Canceled || status.Code(err) == codes.Canceled
}
