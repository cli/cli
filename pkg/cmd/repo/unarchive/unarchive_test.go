package unarchive

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdUnarchive(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		output  UnarchiveOptions
		errMsg  string
	}{
		{
			name:    "no arguments no tty",
			input:   "",
			errMsg:  "--yes required when not running interactively",
			wantErr: true,
		},
		{
			name:   "repo argument tty",
			input:  "OWNER/REPO --confirm",
			output: UnarchiveOptions{RepoArg: "OWNER/REPO", Confirmed: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}
			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)
			var gotOpts *UnarchiveOptions
			cmd := NewCmdUnarchive(f, func(opts *UnarchiveOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.output.RepoArg, gotOpts.RepoArg)
			assert.Equal(t, tt.output.Confirmed, gotOpts.Confirmed)
		})
	}
}

func Test_UnarchiveRun(t *testing.T) {
	queryResponse := `{ "data": { "repository": { "id": "THE-ID","isArchived": %s} } }`
	tests := []struct {
		name          string
		opts          UnarchiveOptions
		httpStubs     func(*httpmock.Registry)
		prompterStubs func(*prompter.PrompterMock)
		isTTY         bool
		wantStdout    string
		wantStderr    string
	}{
		{
			name:       "archived repo tty",
			wantStdout: "✓ Unarchived repository OWNER/REPO\n",
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.ConfirmFunc = func(p string, d bool) (bool, error) {
					if p == "Unarchive OWNER/REPO?" {
						return true, nil
					}
					return false, prompter.NoSuchPromptErr(p)
				}
			},
			isTTY: true,
			opts:  UnarchiveOptions{RepoArg: "OWNER/REPO"},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(fmt.Sprintf(queryResponse, "true")))
				reg.Register(
					httpmock.GraphQL(`mutation UnarchiveRepository\b`),
					httpmock.StringResponse(`{}`))
			},
		},
		{
			name:       "infer base repo",
			wantStdout: "✓ Unarchived repository OWNER/REPO\n",
			opts:       UnarchiveOptions{},
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.ConfirmFunc = func(p string, d bool) (bool, error) {
					if p == "Unarchive OWNER/REPO?" {
						return true, nil
					}
					return false, prompter.NoSuchPromptErr(p)
				}
			},
			isTTY: true,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(fmt.Sprintf(queryResponse, "true")))
				reg.Register(
					httpmock.GraphQL(`mutation UnarchiveRepository\b`),
					httpmock.StringResponse(`{}`))
			},
		},
		{
			name:       "unarchived repo tty",
			wantStderr: "! Repository OWNER/REPO is not archived\n",
			opts:       UnarchiveOptions{RepoArg: "OWNER/REPO"},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(fmt.Sprintf(queryResponse, "false")))
			},
		},
	}

	for _, tt := range tests {
		repo, _ := ghrepo.FromFullName("OWNER/REPO")
		reg := &httpmock.Registry{}
		if tt.httpStubs != nil {
			tt.httpStubs(reg)
		}

		tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
			return repo, nil
		}
		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}
		ios, _, stdout, stderr := iostreams.Test()
		tt.opts.IO = ios

		pm := &prompter.PrompterMock{}
		if tt.prompterStubs != nil {
			tt.prompterStubs(pm)
		}
		tt.opts.Prompter = pm

		t.Run(tt.name, func(t *testing.T) {
			defer reg.Verify(t)
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)

			err := unarchiveRun(&tt.opts)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
