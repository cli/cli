package create

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmd/event/client"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const resource = "https://github.com/OWNER/REPO/pull/1"

func TestNewCmdCreate(t *testing.T) {
	tests := []struct {
		name       string
		args       string
		wantEvents []string
		wantErr    string
	}{
		{
			name:       "resource and events",
			args:       "--resource " + resource + " --event test.a,test.b",
			wantEvents: []string{"test.a", "test.b"},
		},
		{
			name:    "missing resource",
			args:    "--event test.a",
			wantErr: "`--resource` is required",
		},
		{
			name:    "missing events",
			args:    "--resource " + resource,
			wantErr: "`--event` is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			var gotOpts *CreateOptions
			cmd := NewCmdCreate(&cmdutil.Factory{IOStreams: ios}, func(opts *CreateOptions) error {
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
				return
			}
			require.NoError(t, err)
			assert.Equal(t, resource, gotOpts.Resource)
			assert.Equal(t, tt.wantEvents, gotOpts.Events)
		})
	}
}

func TestCreateRun(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)
	sub := &client.Subscription{ID: "sub_1", ResourceURL: resource, EventTypes: []string{"test.a"}, ExpiresAt: now.Add(24 * time.Hour)}

	tests := []struct {
		name       string
		stdoutTTY  bool
		stderrTTY  bool
		jsonFields []string
		created    bool
		createErr  error
		wantOut    string
		wantErrOut string
		wantErr    string
	}{
		{
			name:       "created on a tty",
			stdoutTTY:  true,
			stderrTTY:  true,
			created:    true,
			wantOut:    "sub_1\n",
			wantErrOut: "✓ Created subscription sub_1, which expires in about 1 day\n",
		},
		{
			name:       "reused on a tty",
			stdoutTTY:  true,
			stderrTTY:  true,
			wantOut:    "sub_1\n",
			wantErrOut: "✓ Reused existing subscription sub_1, which expires in about 1 day\n",
		},
		{
			name:       "stdout captured while stderr is a tty",
			stderrTTY:  true,
			created:    true,
			wantOut:    "sub_1\n",
			wantErrOut: "✓ Created subscription sub_1, which expires in about 1 day\n",
		},
		{
			name:      "stderr redirected while stdout is a tty",
			stdoutTTY: true,
			created:   true,
			wantOut:   "sub_1\n",
		},
		{
			name:    "not tty prints only the ID",
			created: true,
			wantOut: "sub_1\n",
		},
		{
			name:       "json",
			jsonFields: []string{"id", "resourceUrl"},
			wantOut:    `{"id":"sub_1","resourceUrl":"https://github.com/OWNER/REPO/pull/1"}` + "\n",
		},
		{
			name:      "missing resource",
			createErr: client.ErrResourceNotFound,
			wantErr:   "could not subscribe to " + resource + "; the resource was not found, or you cannot access it",
		},
		{
			name:      "subscription limit",
			createErr: client.ErrSubscriptionLimit,
			wantErr:   "failed to create subscription: request rejected, or the subscription limit may have been reached\nTo free a slot, run `gh event subscription list`",
		},
		{
			name:      "other error",
			createErr: client.ErrPreviewNotEnabled,
			wantErr:   "failed to create subscription: event subscriptions are in preview",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.stdoutTTY)
			ios.SetStderrTTY(tt.stderrTTY)
			var gotHost, gotResource string
			var gotTypes []string
			opts := &CreateOptions{
				IO:     ios,
				Config: func() (gh.Config, error) { return config.NewMockConfig(), nil },
				EventClient: func(host string) (client.EventClient, error) {
					gotHost = host
					return &client.EventClientMock{
						CreateSubscriptionFunc: func(_ context.Context, resourceURL string, eventTypes []string) (*client.Subscription, bool, error) {
							gotResource, gotTypes = resourceURL, eventTypes
							if tt.createErr != nil {
								return nil, false, tt.createErr
							}
							return sub, tt.created, nil
						},
					}, nil
				},
				Now:      func() time.Time { return now },
				Resource: resource,
				Events:   []string{" test.a", "test.a", ""},
			}
			if tt.jsonFields != nil {
				exporter := cmdutil.NewJSONExporter()
				exporter.SetFields(tt.jsonFields)
				opts.Exporter = exporter
			}

			err := createRun(context.Background(), opts)

			assert.Equal(t, "github.com", gotHost)
			assert.Equal(t, resource, gotResource)
			assert.Equal(t, []string{"test.a"}, gotTypes)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.ErrorIs(t, err, tt.createErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantOut, stdout.String())
			assert.Equal(t, tt.wantErrOut, stderr.String())
		})
	}
}

func TestCreateRunResourceHost(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	var gotHost string
	opts := &CreateOptions{
		IO:     ios,
		Config: func() (gh.Config, error) { return config.NewMockConfig(), nil },
		EventClient: func(host string) (client.EventClient, error) {
			gotHost = host
			return &client.EventClientMock{
				CreateSubscriptionFunc: func(context.Context, string, []string) (*client.Subscription, bool, error) {
					return &client.Subscription{ID: "sub_1"}, true, nil
				},
			}, nil
		},
		Now:      time.Now,
		Resource: "https://ghe.example.com/OWNER/REPO/pull/1",
		Events:   []string{"test.a"},
	}

	require.NoError(t, createRun(context.Background(), opts))
	assert.Equal(t, "ghe.example.com", gotHost)
}
