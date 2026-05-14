package cmdutil

import (
	"context"
	"errors"
	"testing"

	"github.com/AlecAivazis/survey/v2/terminal"
)

func TestIsUserCancellation(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "cancel error", err: CancelError, want: true},
		{name: "terminal interrupt", err: terminal.InterruptErr, want: true},
		{name: "context canceled", err: context.Canceled, want: true},
		{name: "other error", err: errors.New("boom"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUserCancellation(tt.err); got != tt.want {
				t.Fatalf("IsUserCancellation(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
