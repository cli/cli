package view

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/repo/autolink/shared"
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
		"isAlphanumeric",
		"keyPrefix",
		"urlTemplate",
	})
}

func TestNewCmdView(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		output       viewOptions
		wantErr      bool
		wantExporter bool
		errMsg       string
	}{
		{
			name:    "no argument",
			input:   "",
			wantErr: true,
			errMsg:  "accepts 1 arg(s), received 0",
		},
		{
			name:   "id provided",
			input:  "123",
			output: viewOptions{ID: "123"},
		},
		{
			name:         "json flag",
			input:        "123 --json id",
			output:       viewOptions{},
			wantExporter: true,
		},
		{
			name:    "invalid json flag",
			input:   "123 --json invalid",
			output:  viewOptions{},
			wantErr: true,
			errMsg:  "Unknown JSON field: \"invalid\"\nAvailable fields:\n  id\n  isAlphanumeric\n  keyPrefix\n  urlTemplate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}
			f.HttpClient = func() (*http.Client, error) {
				return &http.Client{}, nil
			}

			argv, err := shlex.Split(tt.input)
			require.NoError(t, err)

			var gotOpts *viewOptions
			cmd := NewCmdView(f, func(opts *viewOptions) error {
				gotOpts = opts
				return nil
			})

			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantErr {
				require.EqualError(t, err, tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantExporter, gotOpts.Exporter != nil)
			}
		})
	}
}

type stubAutoLinkViewer struct {
	autolink *shared.Autolink
	err      error
}

func (g stubAutoLinkViewer) View(repo ghrepo.Interface, id string) (*shared.Autolink, error) {
	return g.autolink, g.err
}

type testAutolinkClientViewError struct{}

func (e testAutolinkClientViewError) Error() string {
	return "autolink client view error"
}

func TestViewRun(t *testing.T) {
	tests := []struct {
		name        string
		opts        *viewOptions
		stubViewer  stubAutoLinkViewer
		expectedErr error
		wantStdout  string
	}{
		{
			name: "view",
			opts: &viewOptions{
				ID: "1",
			},
			stubViewer: stubAutoLinkViewer{
				autolink: &shared.Autolink{
					ID:             1,
					KeyPrefix:      "TICKET-",
					URLTemplate:    "https://example.com/TICKET?query=<num>",
					IsAlphanumeric: true,
				},
			},
			wantStdout: heredoc.Doc(`
				Autolink in OWNER/REPO

				ID: 1
				Key Prefix: TICKET-
				URL Template: https://example.com/TICKET?query=<num>
				Alphanumeric: true
			`),
		},
		{
			name: "view json",
			opts: &viewOptions{
				Exporter: func() cmdutil.Exporter {
					exporter := cmdutil.NewJSONExporter()
					exporter.SetFields([]string{"id"})
					return exporter
				}(),
			},
			stubViewer: stubAutoLinkViewer{
				autolink: &shared.Autolink{
					ID:             1,
					KeyPrefix:      "TICKET-",
					URLTemplate:    "https://example.com/TICKET?query=<num>",
					IsAlphanumeric: true,
				},
			},
			wantStdout: "{\"id\":1}\n",
		},
		{
			name: "client error",
			opts: &viewOptions{},
			stubViewer: stubAutoLinkViewer{
				autolink: nil,
				err:      testAutolinkClientViewError{},
			},
			expectedErr: testAutolinkClientViewError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()

			opts := tt.opts
			opts.IO = ios
			opts.Browser = &browser.Stub{}

			opts.IO = ios
			opts.BaseRepo = func() (ghrepo.Interface, error) { return ghrepo.New("OWNER", "REPO"), nil }

			opts.AutolinkClient = &tt.stubViewer
			err := viewRun(opts)

			if tt.expectedErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantStdout, stdout.String())
			}
		})
	}
}
