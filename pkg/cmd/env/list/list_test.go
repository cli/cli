package list

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/env/shared"
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
		"name",
		"nodeId",
		"url",
		"htmlUrl",
		"createdAt",
		"updatedAt",
		"canAdminBypass",
		"protectionRules",
		"protectedBranches",
		"customBranchPolicies",
	})
}

func TestNewCmdList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wants    ListOptions
		wantsErr string
	}{
		{
			name:  "no arguments",
			input: "",
			wants: ListOptions{
				Limit: 30,
			},
		},
		{
			name:  "with limit",
			input: "--limit 100",
			wants: ListOptions{
				Limit: 100,
			},
		},
		{
			name:     "invalid limit",
			input:    "-L 0",
			wantsErr: "invalid limit: 0",
		},
		{
			name:  "with web",
			input: "--web",
			wants: ListOptions{
				Limit: 30,
				Web:   true,
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

			args, err := shlex.Split(tt.input)
			require.NoError(t, err)

			var gotOpts *ListOptions
			cmd := NewCmdList(f, func(opts *ListOptions) error {
				gotOpts = opts
				return nil
			})

			cmd.SetArgs(args)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantsErr != "" {
				require.EqualError(t, err, tt.wantsErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wants.Limit, gotOpts.Limit)
			}
		})
	}
}

type stubEnvironmentLister struct {
	environments []shared.Environment
	err          error
}

func (e stubEnvironmentLister) List(repo ghrepo.Interface, limit int, tty bool) ([]shared.Environment, error) {
	return e.environments, e.err
}

type testEnvironmentClientListError struct{}

func (e testEnvironmentClientListError) Error() string {
	return "environment client list error"
}

func TestListRun(t *testing.T) {
	tests := []struct {
		name        string
		opts        *ListOptions
		isTTY       bool
		stubLister  stubEnvironmentLister
		expectedErr error
		wantStdout  string
		wantStderr  string
	}{
		{
			name:  "client error",
			opts:  &ListOptions{},
			isTTY: true,
			stubLister: stubEnvironmentLister{
				environments: []shared.Environment{},
				err:          testEnvironmentClientListError{},
			},
			expectedErr: testEnvironmentClientListError{},
			wantStderr:  "",
		},
		{
			name:  "no results",
			opts:  &ListOptions{},
			isTTY: true,
			stubLister: stubEnvironmentLister{
				environments: []shared.Environment{},
			},
			expectedErr: cmdutil.NewNoResultsError("No environments found in OWNER/REPO"),
			wantStdout:  "",
			wantStderr:  "",
		},
		{
			name:  "list tty",
			opts:  &ListOptions{},
			isTTY: true,
			stubLister: stubEnvironmentLister{
				environments: []shared.Environment{
					{
						Id:   1,
						Name: "dev",
					},
					{
						Id:   1,
						Name: "prod",
					},
				},
			},
			wantStdout: heredoc.Doc(`

				Showing 2 of 2 environments in OWNER/REPO

				NAME  PROTECTION RULES  SECRETS  VARIABLES
				dev   0                 0        0
				prod  0                 0        0
			`),
			wantStderr: "",
		},
		{
			name:  "list non-tty",
			opts:  &ListOptions{},
			isTTY: false,
			stubLister: stubEnvironmentLister{
				environments: []shared.Environment{
					{
						Id:   1,
						Name: "dev",
					},
					{
						Id:   1,
						Name: "prod",
					},
				},
			},
			wantStdout: heredoc.Doc(`
				dev
				prod
			`),
			wantStderr: "",
		},
		{
			name: "list json non-tty",
			opts: &ListOptions{
				Exporter: func() cmdutil.Exporter {
					exporter := cmdutil.NewJSONExporter()
					exporter.SetFields([]string{"id", "name"})
					return exporter
				}(),
			},
			isTTY: false,
			stubLister: stubEnvironmentLister{
				environments: []shared.Environment{
					{
						Id:   1,
						Name: "dev",
					},
					{
						Id:   1,
						Name: "prod",
					},
				},
			},
			wantStdout: "[{\"id\":1,\"name\":\"dev\"},{\"id\":1,\"name\":\"prod\"}]\n",
			wantStderr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)

			opts := tt.opts
			opts.IO = ios
			opts.BaseRepo = func() (ghrepo.Interface, error) { return ghrepo.New("OWNER", "REPO"), nil }
			opts.EnvironmentListClient = &tt.stubLister

			err := listRun(opts)

			if tt.expectedErr != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.expectedErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantStdout, stdout.String())
			}

			if tt.wantStderr != "" {
				assert.Equal(t, tt.wantStderr, stderr.String())
			}
		})
	}
}
