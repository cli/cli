package list

import (
	"bytes"
	"context"
	"errors"
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
	jsonfieldstest.ExpectCommandToSupportJSONFields(t, NewCmdList, []string{
		"id",
		"resourceUrl",
		"eventTypes",
		"createdAt",
		"expiresAt",
		"url",
		"eventsUrl",
	})
}

func TestNewCmdList(t *testing.T) {
	tests := []struct {
		name       string
		args       string
		wantLimit  int
		wantFields []string
		wantErr    string
	}{
		{
			name:      "defaults",
			wantLimit: 30,
		},
		{
			name:      "limit",
			args:      "--limit 5",
			wantLimit: 5,
		},
		{
			name:    "zero limit",
			args:    "--limit 0",
			wantErr: "invalid limit: 0",
		},
		{
			name:       "json fields",
			args:       "--json id,resourceUrl",
			wantLimit:  30,
			wantFields: []string{"id", "resourceUrl"},
		},
		{
			name:    "unknown json field",
			args:    "--json nope",
			wantErr: "Unknown JSON field: \"nope\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			var gotOpts *ListOptions
			cmd := NewCmdList(&cmdutil.Factory{IOStreams: ios}, func(opts *ListOptions) error {
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
			assert.Equal(t, tt.wantLimit, gotOpts.Limit)
			if tt.wantFields == nil {
				assert.Nil(t, gotOpts.Exporter)
			} else {
				assert.Equal(t, tt.wantFields, gotOpts.Exporter.Fields())
			}
		})
	}
}

func TestListRun(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)
	subs := []client.Subscription{
		{
			ID:          "sub_2",
			ResourceURL: "https://github.com/OWNER/REPO/pull/2",
			EventTypes:  []string{"test.a", "test.b"},
			ExpiresAt:   now.Add(23 * time.Hour),
		},
		{
			ID:          "sub_1",
			ResourceURL: "https://github.com/OWNER/REPO/pull/1",
			EventTypes:  []string{"test.a"},
			ExpiresAt:   now.Add(30 * time.Minute),
		},
	}

	tests := []struct {
		name       string
		tty        bool
		jsonFields []string
		subs       []client.Subscription
		listErr    error
		wantOut    string
		wantErr    string
		wantNoRes  bool
	}{
		{
			name: "tty",
			tty:  true,
			subs: subs,
			wantOut: "\n" + heredoc.Doc(`
				Showing 2 event subscriptions

				ID     RESOURCE                              EVENTS          EXPIRES
				sub_2  https://github.com/OWNER/REPO/pull/2  test.a, test.b  in about 23 hours
				sub_1  https://github.com/OWNER/REPO/pull/1  test.a          in about 30 minutes
			`),
		},
		{
			name: "not tty",
			subs: subs,
			wantOut: heredoc.Doc(`
				sub_2	https://github.com/OWNER/REPO/pull/2	test.a,test.b	2026-01-03T02:00:00Z
				sub_1	https://github.com/OWNER/REPO/pull/1	test.a	2026-01-02T03:30:00Z
			`),
		},
		{
			name:      "no subscriptions",
			tty:       true,
			subs:      []client.Subscription{},
			wantNoRes: true,
		},
		{
			name:       "json",
			jsonFields: []string{"id", "eventTypes"},
			subs:       subs,
			wantOut:    `[{"eventTypes":["test.a","test.b"],"id":"sub_2"},{"eventTypes":["test.a"],"id":"sub_1"}]` + "\n",
		},
		{
			name:       "json without subscriptions",
			jsonFields: []string{"id"},
			subs:       []client.Subscription{},
			wantOut:    "[]\n",
		},
		{
			name:    "api error",
			listErr: client.ErrPreviewNotEnabled,
			wantErr: "failed to list subscriptions: event subscriptions are in preview",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			var gotHost string
			var gotLimit int
			opts := &ListOptions{
				IO:     ios,
				Config: func() (gh.Config, error) { return config.NewMockConfig(), nil },
				EventClient: func(host string) (client.EventClient, error) {
					gotHost = host
					return &client.EventClientMock{
						ListSubscriptionsFunc: func(_ context.Context, limit int) ([]client.Subscription, error) {
							gotLimit = limit
							return tt.subs, tt.listErr
						},
					}, nil
				},
				Now:   func() time.Time { return now },
				Limit: 7,
			}
			if tt.jsonFields != nil {
				exporter := cmdutil.NewJSONExporter()
				exporter.SetFields(tt.jsonFields)
				opts.Exporter = exporter
			}

			err := listRun(context.Background(), opts)

			assert.Equal(t, "github.com", gotHost)
			assert.Equal(t, 7, gotLimit)
			switch {
			case tt.wantErr != "":
				require.ErrorContains(t, err, tt.wantErr)
				require.ErrorIs(t, err, tt.listErr)
			case tt.wantNoRes:
				var noResults cmdutil.NoResultsError
				require.True(t, errors.As(err, &noResults))
				assert.Equal(t, "no event subscriptions", noResults.Error())
			default:
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantOut, stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}

func TestListHeader(t *testing.T) {
	assert.Equal(t, "Showing 1 event subscription", listHeader(1, 30))
	assert.Equal(t, "Showing the 30 most recent event subscriptions", listHeader(30, 30))
}

func TestListRunDefaultHost(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()
	cfg := config.NewMockConfig()
	authCfg := cfg.Authentication()
	authCfg.SetDefaultHost("example.com", "GH_HOST")
	cfg.AuthenticationFunc = func() gh.AuthConfig { return authCfg }
	clientErr := errors.New("event subscriptions are only supported on github.com")
	var gotHost string
	opts := &ListOptions{
		IO:     ios,
		Config: func() (gh.Config, error) { return cfg, nil },
		EventClient: func(host string) (client.EventClient, error) {
			gotHost = host
			return nil, clientErr
		},
		Limit: 30,
	}

	err := listRun(context.Background(), opts)

	require.ErrorIs(t, err, clientErr)
	assert.Equal(t, "example.com", gotHost)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}
