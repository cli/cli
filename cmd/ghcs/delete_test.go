package ghcs

import (
	"testing"
	"time"

	"github.com/github/ghcs/internal/api"
)

func TestFilterCodespacesToDelete(t *testing.T) {
	type args struct {
		codespaces    []*api.Codespace
		thresholdDays int
	}
	tests := []struct {
		name    string
		now     time.Time
		args    args
		wantErr bool
		deleted []*api.Codespace
	}{
		{
			name: "no codespaces is to be deleted",

			args: args{
				codespaces: []*api.Codespace{
					{
						Name:       "testcodespace",
						CreatedAt:  "2021-08-09T10:10:24+02:00",
						LastUsedAt: "2021-08-09T13:10:24+02:00",
						Environment: api.CodespaceEnvironment{
							State: "Shutdown",
						},
					},
				},
				thresholdDays: 1,
			},
			now:     time.Date(2021, 8, 9, 20, 10, 24, 0, time.UTC),
			deleted: []*api.Codespace{},
		},
		{
			name: "one codespace is to be deleted",

			args: args{
				codespaces: []*api.Codespace{
					{
						Name:       "testcodespace",
						CreatedAt:  "2021-08-09T10:10:24+02:00",
						LastUsedAt: "2021-08-09T13:10:24+02:00",
						Environment: api.CodespaceEnvironment{
							State: "Shutdown",
						},
					},
				},
				thresholdDays: 1,
			},
			now: time.Date(2021, 8, 15, 20, 12, 24, 0, time.UTC),
			deleted: []*api.Codespace{
				{
					Name:       "testcodespace",
					CreatedAt:  "2021-08-09T10:10:24+02:00",
					LastUsedAt: "2021-08-09T13:10:24+02:00",
				},
			},
		},
		{
			name: "threshold is invalid",

			args: args{
				codespaces: []*api.Codespace{
					{
						Name:       "testcodespace",
						CreatedAt:  "2021-08-09T10:10:24+02:00",
						LastUsedAt: "2021-08-09T13:10:24+02:00",
						Environment: api.CodespaceEnvironment{
							State: "Shutdown",
						},
					},
				},
				thresholdDays: -1,
			},
			now:     time.Date(2021, 8, 15, 20, 12, 24, 0, time.UTC),
			wantErr: true,
			deleted: []*api.Codespace{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			now = func() time.Time {
				return tt.now
			}

			codespaces, err := filterCodespacesToDelete(tt.args.codespaces, tt.args.thresholdDays)
			if (err != nil) != tt.wantErr {
				t.Errorf("API.CleanupUnusedCodespaces() error = %v, wantErr %v", err, tt.wantErr)
			}

			if len(codespaces) != len(tt.deleted) {
				t.Errorf("expected %d deleted codespaces, got %d", len(tt.deleted), len(codespaces))
			}
		})
	}
}
