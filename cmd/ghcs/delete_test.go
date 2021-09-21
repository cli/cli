package ghcs

import (
	"context"
	"testing"
)

func TestDelete(t *testing.T) {
	tests := []struct {
		name    string
		opts    deleteOptions
		wantErr bool
	}{
		{
			name: "by name",
			opts: deleteOptions{
				codespaceName: "foo-bar-123",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := delete(context.Background(), tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("delete() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
