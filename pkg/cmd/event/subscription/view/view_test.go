package view

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmd/event/client"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/jsonfieldstest"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONFields(t *testing.T) {
	jsonfieldstest.ExpectCommandToSupportJSONFields(t, NewCmdView, []string{
		"id",
		"resourceUrl",
		"eventTypes",
		"createdAt",
		"expiresAt",
		"url",
		"eventsUrl",
	})
}

func TestNewCmdView(t *testing.T) {
	tests := []struct {
		name       string
		args       string
		wantID     string
		wantFields []string
		wantErr    string
	}{
		{
			name:   "id",
			args:   "sub_1",
			wantID: "sub_1",
		},
		{
			name:       "id with json fields",
			args:       "sub_1 --json id,expiresAt",
			wantID:     "sub_1",
			wantFields: []string{"id", "expiresAt"},
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
			var gotOpts *ViewOptions
			cmd := NewCmdView(&cmdutil.Factory{IOStreams: ios}, func(opts *ViewOptions) error {
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
			if tt.wantFields == nil {
				assert.Nil(t, gotOpts.Exporter)
			} else {
				assert.Equal(t, tt.wantFields, gotOpts.Exporter.Fields())
			}
		})
	}
}

func TestViewRun(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)
	sub := &client.Subscription{
		ID:          "sub_1",
		ResourceURL: "https://github.com/OWNER/REPO/pull/1",
		EventTypes:  []string{"com.github.pull_request.review.submitted.v1", "test.b"},
		CreatedAt:   now.Add(-2 * time.Hour),
		ExpiresAt:   now.Add(22 * time.Hour),
	}

	tests := []struct {
		name       string
		tty        bool
		jsonFields []string
		getErr     error
		wantOut    string
		wantErr    string
	}{
		{
			name: "tty",
			tty:  true,
			wantOut: heredoc.Doc(`
				sub_1
				Created about 2 hours ago • Expires in about 22 hours

				Resource
				https://github.com/OWNER/REPO/pull/1

				Events
				• com.github.pull_request.review.submitted.v1  a review was submitted
				• test.b
			`),
		},
		{
			name: "not tty",
			wantOut: heredoc.Doc(`
				id:	sub_1
				resource:	https://github.com/OWNER/REPO/pull/1
				events:	com.github.pull_request.review.submitted.v1,test.b
				created:	2026-01-02T01:00:00Z
				expires:	2026-01-03T01:00:00Z
			`),
		},
		{
			name:       "json",
			jsonFields: []string{"id", "expiresAt"},
			wantOut:    `{"expiresAt":"2026-01-03T01:00:00Z","id":"sub_1"}` + "\n",
		},
		{
			name:    "missing subscription",
			getErr:  client.ErrSubscriptionNotFound,
			wantErr: "subscription sub_1 was not found; it may have expired or been deleted, or you cannot access it",
		},
		{
			name:    "other error",
			getErr:  client.ErrPreviewNotEnabled,
			wantErr: "failed to get subscription: event subscriptions are in preview",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			var gotHost string
			opts := &ViewOptions{
				IO:     ios,
				Config: func() (gh.Config, error) { return config.NewMockConfig(), nil },
				EventClient: func(host string) (client.EventClient, error) {
					gotHost = host
					return &client.EventClientMock{
						GetSubscriptionFunc: func(_ context.Context, id string) (*client.Subscription, error) {
							assert.Equal(t, "sub_1", id)
							if tt.getErr != nil {
								return nil, tt.getErr
							}
							return sub, nil
						},
					}, nil
				},
				Now: func() time.Time { return now },
				ID:  "sub_1",
			}
			if tt.jsonFields != nil {
				exporter := cmdutil.NewJSONExporter()
				exporter.SetFields(tt.jsonFields)
				opts.Exporter = exporter
			}

			err := viewRun(context.Background(), opts)

			assert.Equal(t, "github.com", gotHost)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantOut, stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}
