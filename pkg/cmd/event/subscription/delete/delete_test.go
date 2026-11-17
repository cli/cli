package delete

import (
	"bytes"
	"context"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmd/event/client"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdDelete(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		wantID  string
		wantErr string
	}{
		{
			name:   "id",
			args:   "sub_1",
			wantID: "sub_1",
		},
		{
			name:    "no id",
			wantErr: "subscription ID required",
		},
		{
			name:    "more than one id",
			args:    "sub_1 sub_2",
			wantErr: "too many arguments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			var gotOpts *DeleteOptions
			cmd := NewCmdDelete(&cmdutil.Factory{IOStreams: ios}, func(opts *DeleteOptions) error {
				gotOpts = opts
				return nil
			})
			args, err := shlex.Split(tt.args)
			require.NoError(t, err)
			cmd.SetArgs(args)
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				assert.Nil(t, gotOpts)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, gotOpts.ID)
		})
	}
}

func TestDeleteRun(t *testing.T) {
	tests := []struct {
		name       string
		tty        bool
		deleteErr  error
		wantErrOut string
		wantErr    string
	}{
		{
			name:       "tty",
			tty:        true,
			wantErrOut: "✓ Deleted subscription sub_1\n",
		},
		{
			name: "not tty",
		},
		{
			name:      "missing subscription",
			tty:       true,
			deleteErr: client.ErrSubscriptionNotFound,
			wantErr:   "subscription sub_1 was not found; it may have expired or been deleted, or you cannot access it",
		},
		{
			name:      "other error",
			deleteErr: client.ErrPreviewNotEnabled,
			wantErr:   "failed to delete subscription: event subscriptions are in preview",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			ios.SetStderrTTY(tt.tty)
			var gotHost, gotID string
			opts := &DeleteOptions{
				IO:     ios,
				Config: func() (gh.Config, error) { return config.NewMockConfig(), nil },
				EventClient: func(host string) (client.EventClient, error) {
					gotHost = host
					return &client.EventClientMock{
						DeleteSubscriptionFunc: func(_ context.Context, id string) error {
							gotID = id
							return tt.deleteErr
						},
					}, nil
				},
				ID: "sub_1",
			}

			err := deleteRun(context.Background(), opts)

			assert.Equal(t, "github.com", gotHost)
			assert.Equal(t, "sub_1", gotID)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Empty(t, stdout.String())
			assert.Equal(t, tt.wantErrOut, stderr.String())
		})
	}
}
