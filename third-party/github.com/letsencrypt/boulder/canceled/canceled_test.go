package canceled

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestCanceled(t *testing.T) {
	if !Is(context.Canceled) {
		t.Errorf("Expected context.Canceled to be canceled, but wasn't.")
	}
	if !Is(status.Errorf(codes.Canceled, "hi")) {
		t.Errorf("Expected gRPC cancellation to be cancelled, but wasn't.")
	}
	if Is(errors.New("hi")) {
		t.Errorf("Expected random error to not be cancelled, but was.")
	}
}
