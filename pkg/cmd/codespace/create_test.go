package codespace

import (
	"context"
	"testing"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/iostreams"
)

func TestGetMachineName(t *testing.T) {
	tests := []struct {
		name                 string
		prebuildAvailability string
		wantStdout           string
	}{
		{
			name:                 "prebuild availability is pool",
			prebuildAvailability: "pool",
			wantStdout:           "something",
		},
		{
			name:                 "prebuild availability is blob",
			prebuildAvailability: "blob",
			wantStdout:           "something",
		},
		{
			name:                 "prebuild availability is none",
			prebuildAvailability: "none",
			wantStdout:           "something",
		},
		{
			name:                 "prebuild availability is empty",
			prebuildAvailability: "",
			wantStdout:           "something",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiMock := &apiClientMock{
				GetCodespacesMachinesFunc: func(ctx context.Context, repoID int, branch string, location string) ([]*api.Machine, error) {
					machines := []*api.Machine{
						{
							Name:                 "standardLinux32gb",
							DisplayName:          "4 cores, 8 GB RAM, 32 GB storage",
							PrebuildAvailability: tt.prebuildAvailability,
						},
					}
					return machines, nil
				},
			}

			io, _, stdout, _ := iostreams.Test()
			io.SetStdinTTY(true)
			io.SetStdoutTTY(true)
			app := NewApp(io, apiMock)
			ctx := context.TODO()

			getMachineName(ctx, app.io, app.apiClient, 123, "", "main", "EastUs")

			if out := stdout.String(); out != tt.wantStdout {
				t.Errorf("stdout = %q, want %q", out, tt.wantStdout)
			}
		})
	}
}
