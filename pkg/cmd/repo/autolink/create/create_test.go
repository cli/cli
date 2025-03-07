package create

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/repo/autolink/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdCreate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		output  createOptions
		wantErr bool
		errMsg  string
	}{
		{
			name:    "no argument",
			input:   "",
			wantErr: true,
			errMsg:  "Cannot create autolink: keyPrefix and urlTemplate arguments are both required",
		},
		{
			name:    "one argument",
			input:   "TEST-",
			wantErr: true,
			errMsg:  "Cannot create autolink: keyPrefix and urlTemplate arguments are both required",
		},
		{
			name:  "two argument",
			input: "TICKET- https://example.com/TICKET?query=<num>",
			output: createOptions{
				KeyPrefix:   "TICKET-",
				URLTemplate: "https://example.com/TICKET?query=<num>",
			},
		},
		{
			name:  "numeric flag",
			input: "TICKET- https://example.com/TICKET?query=<num> --numeric",
			output: createOptions{
				KeyPrefix:   "TICKET-",
				URLTemplate: "https://example.com/TICKET?query=<num>",
				Numeric:     true,
			},
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

			var gotOpts *createOptions
			cmd := NewCmdCreate(f, func(opts *createOptions) error {
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
				assert.Equal(t, tt.output.KeyPrefix, gotOpts.KeyPrefix)
				assert.Equal(t, tt.output.URLTemplate, gotOpts.URLTemplate)
				assert.Equal(t, tt.output.Numeric, gotOpts.Numeric)
			}
		})
	}
}

type stubAutolinkCreator struct {
	err error
}

func (g stubAutolinkCreator) Create(repo ghrepo.Interface, request AutolinkCreateRequest) (*shared.Autolink, error) {
	if g.err != nil {
		return nil, g.err
	}

	return &shared.Autolink{
		ID:             1,
		KeyPrefix:      request.KeyPrefix,
		URLTemplate:    request.URLTemplate,
		IsAlphanumeric: request.IsAlphanumeric,
	}, nil
}

type testAutolinkClientCreateError struct{}

func (e testAutolinkClientCreateError) Error() string {
	return "autolink client create error"
}

func TestCreateRun(t *testing.T) {
	tests := []struct {
		name        string
		opts        *createOptions
		stubCreator stubAutolinkCreator
		expectedErr error
		errMsg      string
		wantStdout  string
		wantStderr  string
	}{
		{
			name: "success, alphanumeric",
			opts: &createOptions{
				KeyPrefix:   "TICKET-",
				URLTemplate: "https://example.com/TICKET?query=<num>",
			},
			stubCreator: stubAutolinkCreator{},
			wantStdout:  "✓ Created repository autolink 1 on OWNER/REPO\n",
		},
		{
			name: "success, numeric",
			opts: &createOptions{
				KeyPrefix:   "TICKET-",
				URLTemplate: "https://example.com/TICKET?query=<num>",
				Numeric:     true,
			},
			stubCreator: stubAutolinkCreator{},
			wantStdout:  "✓ Created repository autolink 1 on OWNER/REPO\n",
		},
		{
			name: "client error",
			opts: &createOptions{
				KeyPrefix:   "TICKET-",
				URLTemplate: "https://example.com/TICKET?query=<num>",
			},
			stubCreator: stubAutolinkCreator{err: testAutolinkClientCreateError{}},
			expectedErr: testAutolinkClientCreateError{},
			errMsg:      fmt.Sprint("error creating autolink: ", testAutolinkClientCreateError{}.Error()),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()

			opts := tt.opts
			opts.IO = ios
			opts.Browser = &browser.Stub{}

			opts.IO = ios
			opts.BaseRepo = func() (ghrepo.Interface, error) { return ghrepo.New("OWNER", "REPO"), nil }

			opts.AutolinkClient = &tt.stubCreator
			err := createRun(opts)

			if tt.expectedErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedErr)
				assert.Equal(t, tt.errMsg, err.Error())
			} else {
				require.NoError(t, err)
			}

			if tt.wantStdout != "" {
				assert.Equal(t, tt.wantStdout, stdout.String())
			}

			if tt.wantStderr != "" {
				assert.Equal(t, tt.wantStderr, stderr.String())
			}
		})
	}
}
